// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Entries of a pluggable widget's object list (a PDS dropdown menu's
// `dropdownItems`, an accordion's groups) are nameless CustomWidgets$WidgetObject
// documents in the owner's Object.Properties[].Value.Objects. DESCRIBE names them
// positionally — `<keyword><N>`, 1-based per list — so ALTER PAGE addresses them
// the same way: bare (`dropdownitem2`) when only one widget on the page has such
// an entry, qualified (`menu.dropdownitem2`) otherwise.
//
// New entries are never hand-built here. The executor builds a throwaway
// "donor" widget of the same widget id with the new entries as its children,
// through the ordinary pluggable-widget builder, and the entries are lifted out
// of its serialized form. Every TypePointer in them names the DONOR's Type, so
// each is repointed at the matching element of the owner's stored Type (paired
// by PropertyKey); a pointer with no counterpart refuses the whole operation
// before anything is written. Splicing the donor's entries in unchanged is what
// produces CE0463 / a document Studio Pro cannot load.

// objectListLoc is one object-list property of one pluggable widget.
type objectListLoc struct {
	owner     bson.D
	ownerName string
	widgetID  string
	listKey   string
	keyword   string // lower case, e.g. dropdownitem
	propDoc   bson.D // the CustomWidgets$WidgetProperty holding the list
}

// items returns the list's entries (marker stripped).
func (l objectListLoc) items() []any {
	return bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(l.propDoc, "Value"), "Objects"))
}

// setItems writes the entries back, preserving the list's typed-array marker.
func (l objectListLoc) setItems(items []any) bool {
	return bsonnav.DSetArrayIn(l.propDoc, "Value", "Objects", items, 2)
}

// objectListsOf returns the object-list properties of a CustomWidget document,
// in the order its Type declares them.
func objectListsOf(w bson.D) []objectListLoc {
	typ := bsonnav.DGetDoc(w, "Type")
	objType := bsonnav.DGetDoc(typ, "ObjectType")
	obj := bsonnav.DGetDoc(w, "Object")
	if objType == nil || obj == nil {
		return nil
	}
	props := bsonnav.DGetArrayElements(bsonnav.DGet(obj, "Properties"))
	var out []objectListLoc
	for _, pt := range bsonnav.DGetArrayElements(bsonnav.DGet(objType, "PropertyTypes")) {
		ptDoc, ok := pt.(bson.D)
		if !ok {
			continue
		}
		vt := bsonnav.DGetDoc(ptDoc, "ValueType")
		if bsonnav.DGetDoc(vt, "ObjectType") == nil {
			continue
		}
		if isList, _ := bsonnav.DGet(vt, "IsList").(bool); !isList {
			continue
		}
		ptID := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(ptDoc, "$ID"))
		for _, p := range props {
			pDoc, ok := p.(bson.D)
			if !ok || bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(pDoc, "TypePointer")) != ptID {
				continue
			}
			key := bsonnav.DGetString(ptDoc, "PropertyKey")
			out = append(out, objectListLoc{
				owner:     w,
				ownerName: bsonnav.DGetString(w, "Name"),
				widgetID:  bsonnav.DGetString(typ, "WidgetId"),
				listKey:   key,
				keyword:   strings.ToLower(types.ObjectListKeyword(key)),
				propDoc:   pDoc,
			})
			break
		}
	}
	return out
}

// collectCustomWidgets returns every CustomWidgets$CustomWidget document under v.
func collectCustomWidgets(v any, out *[]bson.D) {
	switch x := v.(type) {
	case bson.D:
		if bsonnav.DGetString(x, "$Type") == "CustomWidgets$CustomWidget" {
			*out = append(*out, x)
		}
		for _, e := range x {
			collectCustomWidgets(e.Value, out)
		}
	case bson.A:
		for _, el := range x {
			collectCustomWidgets(el, out)
		}
	}
}

// isAddressableList reports whether ALTER PAGE addresses a list's entries by
// position. DataGrid2 columns are excluded: they have their own path
// (`grid.column`, by column name) and must not be shadowed by `columnN`.
func isAddressableList(l objectListLoc) bool {
	return l.listKey != "columns"
}

var positionalItemRE = regexp.MustCompile(`^([a-z]+)([1-9][0-9]*)$`)

// ResolveObjectListItem resolves an object-list entry reference. With itemRef
// empty, widgetRef is a bare entry name (`dropdownitem2`); otherwise widgetRef
// names the owner and itemRef the entry. ok is false when the reference is not
// an entry at all — including when a real widget carries that name, which wins.
func (m *Mutator) ResolveObjectListItem(widgetRef, itemRef string) (backend.ObjectListItemRef, bool, error) {
	var zero backend.ObjectListItemRef
	owner, name := widgetRef, itemRef
	if itemRef == "" {
		owner, name = "", widgetRef
		if m.widgetFinder(m.rawData, widgetRef) != nil {
			return zero, false, nil
		}
	}
	match := positionalItemRE.FindStringSubmatch(strings.ToLower(name))
	if match == nil {
		return zero, false, nil
	}
	keyword := match[1]
	n, _ := strconv.Atoi(match[2])

	var widgets []bson.D
	if owner != "" {
		res := m.widgetFinder(m.rawData, owner)
		if res == nil {
			return zero, false, nil
		}
		widgets = []bson.D{res.widget}
	} else {
		collectCustomWidgets(m.rawData, &widgets)
	}

	var candidates []objectListLoc
	for _, w := range widgets {
		for _, l := range objectListsOf(w) {
			if l.keyword == keyword && isAddressableList(l) {
				candidates = append(candidates, l)
			}
		}
	}
	if len(candidates) == 0 {
		return zero, false, nil
	}
	if len(candidates) > 1 {
		// A bare name is ambiguous only among lists that actually have entry N.
		var have []objectListLoc
		for _, c := range candidates {
			if n <= len(c.items()) {
				have = append(have, c)
			}
		}
		if len(have) == 1 {
			candidates = have
		} else {
			var forms []string
			for _, c := range candidates {
				forms = append(forms, "`"+c.ownerName+"."+keyword+strconv.Itoa(n)+"`")
			}
			return zero, false, fmt.Errorf("%q names an entry of more than one widget's list; qualify it as one of %s",
				name, strings.Join(forms, ", "))
		}
	}
	c := candidates[0]
	if count := len(c.items()); n > count {
		return zero, false, fmt.Errorf("%q: widget %q has %d %s entr%s, so there is no entry %d",
			name, c.ownerName, count, keyword, plural(count, "y", "ies"), n)
	}
	return backend.ObjectListItemRef{
		Owner:    c.ownerName,
		WidgetID: c.widgetID,
		ListKey:  c.listKey,
		Keyword:  c.keyword,
		Index:    n - 1,
	}, true, nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// PluggableWidgetID returns the widget id of the named pluggable widget, or "".
func (m *Mutator) PluggableWidgetID(widgetRef string) string {
	res := m.widgetFinder(m.rawData, widgetRef)
	if res == nil || bsonnav.DGetString(res.widget, "$Type") != "CustomWidgets$CustomWidget" {
		return ""
	}
	return bsonnav.DGetString(bsonnav.DGetDoc(res.widget, "Type"), "WidgetId")
}

// ownerList finds the owner's list of the given key.
func (m *Mutator) ownerList(ownerRef, listKey string) (objectListLoc, error) {
	res := m.widgetFinder(m.rawData, ownerRef)
	if res == nil {
		return objectListLoc{}, m.widgetNotFoundError(ownerRef)
	}
	for _, l := range objectListsOf(res.widget) {
		if l.listKey == listKey {
			return l, nil
		}
	}
	return objectListLoc{}, fmt.Errorf("widget %q has no object list %q", ownerRef, listKey)
}

// InsertObjectListItems splices the donor's entries of listKey into the owner's
// list at index (0..len; -1 appends).
func (m *Mutator) InsertObjectListItems(ownerRef, listKey string, index int, donor pages.Widget) error {
	target, err := m.ownerList(ownerRef, listKey)
	if err != nil {
		return err
	}
	existing := target.items()
	if index < 0 {
		index = len(existing)
	}
	if index > len(existing) {
		return fmt.Errorf("widget %q: cannot insert at position %d of a list with %d entries", ownerRef, index+1, len(existing))
	}

	donorDoc := m.deps.SerializeWidget(donor)
	if donorDoc == nil {
		return fmt.Errorf("widget %q: could not serialize the new %s entries", ownerRef, target.keyword)
	}
	var source *objectListLoc
	for _, l := range objectListsOf(donorDoc) {
		if l.listKey == listKey {
			l := l
			source = &l
			break
		}
	}
	if source == nil {
		return fmt.Errorf("widget %q: the new entries did not produce a %q list", ownerRef, listKey)
	}
	newItems := source.items()
	if len(newItems) == 0 {
		return fmt.Errorf("widget %q: no %s entries to insert", ownerRef, target.keyword)
	}

	idMap := map[string]any{}
	pairTypeIDs(bsonnav.DGetDoc(bsonnav.DGetDoc(donorDoc, "Type"), "ObjectType"),
		bsonnav.DGetDoc(bsonnav.DGetDoc(target.owner, "Type"), "ObjectType"), idMap)
	var unmapped []string
	for _, it := range newItems {
		repointTypePointers(it, idMap, &unmapped)
	}
	if len(unmapped) > 0 {
		return fmt.Errorf("widget %q: %d TypePointer(s) of the new %s entries have no counterpart in the widget's "+
			"stored type (is the stored widget from another version of its package?); nothing was written",
			ownerRef, len(unmapped), target.keyword)
	}

	out := make([]any, 0, len(existing)+len(newItems))
	out = append(out, existing[:index]...)
	out = append(out, newItems...)
	out = append(out, existing[index:]...)
	if !target.setItems(out) {
		return fmt.Errorf("widget %q: could not write list %q", ownerRef, listKey)
	}
	return nil
}

// DropObjectListItems removes the entries at the given 0-based indexes.
func (m *Mutator) DropObjectListItems(ownerRef, listKey string, indexes []int) error {
	target, err := m.ownerList(ownerRef, listKey)
	if err != nil {
		return err
	}
	existing := target.items()
	drop := map[int]bool{}
	for _, i := range indexes {
		if i < 0 || i >= len(existing) {
			return fmt.Errorf("widget %q has %d %s entries, so there is no entry %d", ownerRef, len(existing), target.keyword, i+1)
		}
		drop[i] = true
	}
	out := make([]any, 0, len(existing))
	for i, it := range existing {
		if !drop[i] {
			out = append(out, it)
		}
	}
	if !target.setItems(out) {
		return fmt.Errorf("widget %q: could not write list %q", ownerRef, listKey)
	}
	return nil
}

// pairTypeIDs maps every id of the donor's WidgetObjectType tree (object types,
// property types, value types) onto the stored tree's id of the element with
// the same PropertyKey path.
func pairTypeIDs(donorOT, storedOT bson.D, idMap map[string]any) {
	if donorOT == nil || storedOT == nil {
		return
	}
	mapID(donorOT, storedOT, idMap)
	stored := map[string]bson.D{}
	for _, pt := range bsonnav.DGetArrayElements(bsonnav.DGet(storedOT, "PropertyTypes")) {
		if d, ok := pt.(bson.D); ok {
			stored[bsonnav.DGetString(d, "PropertyKey")] = d
		}
	}
	for _, pt := range bsonnav.DGetArrayElements(bsonnav.DGet(donorOT, "PropertyTypes")) {
		d, ok := pt.(bson.D)
		if !ok {
			continue
		}
		s := stored[bsonnav.DGetString(d, "PropertyKey")]
		if s == nil {
			continue
		}
		mapID(d, s, idMap)
		dVT, sVT := bsonnav.DGetDoc(d, "ValueType"), bsonnav.DGetDoc(s, "ValueType")
		if dVT == nil || sVT == nil {
			continue
		}
		mapID(dVT, sVT, idMap)
		pairTypeIDs(bsonnav.DGetDoc(dVT, "ObjectType"), bsonnav.DGetDoc(sVT, "ObjectType"), idMap)
	}
}

func mapID(donor, stored bson.D, idMap map[string]any) {
	d := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(donor, "$ID"))
	if d == "" {
		return
	}
	if s := bsonnav.DGet(stored, "$ID"); s != nil {
		idMap[d] = s
	}
}

// repointTypePointers rewrites every TypePointer under v through idMap,
// recording the ones with no mapping. A nested CustomWidget carries its own
// Type, so its pointers are already consistent and are left alone.
func repointTypePointers(v any, idMap map[string]any, unmapped *[]string) {
	switch x := v.(type) {
	case bson.D:
		if bsonnav.DGetString(x, "$Type") == "CustomWidgets$CustomWidget" {
			return
		}
		for i := range x {
			if x[i].Key == "TypePointer" {
				id := bsonnav.ExtractBinaryIDFromDoc(x[i].Value)
				if id == "" {
					continue
				}
				if s, ok := idMap[id]; ok {
					x[i].Value = s
				} else {
					*unmapped = append(*unmapped, id)
				}
				continue
			}
			repointTypePointers(x[i].Value, idMap, unmapped)
		}
	case bson.A:
		for _, el := range x {
			repointTypePointers(el, idMap, unmapped)
		}
	}
}
