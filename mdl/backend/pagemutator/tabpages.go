// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// Tab pages are addressable widgets: DESCRIBE PAGE prints `tabpage tabPage2
// (Caption: …)`, and ALTER PAGE finds them by that name. They are not ordinary
// widgets, though, and the helpers here keep the two kinds apart:
//
//   - a Forms$TabControl's TabPages list holds tab pages and nothing else, and a
//     tab page lives nowhere but there — either mix is a document Studio Pro
//     cannot load;
//   - the TabControl's DefaultPagePointer is a binary $ID pointing at one of its
//     tab pages, so dropping or replacing that page must repoint it (a dangling
//     pointer is an unopenable project);
//   - a tab page has no Appearance, so Class/Style/DynamicClasses cannot be set.

const (
	tabPageType    = "Forms$TabPage"
	tabControlType = "Forms$TabControl"
)

func isTabPage(w bson.D) bool    { return bsonnav.DGetString(w, "$Type") == tabPageType }
func isTabControl(w bson.D) bool { return bsonnav.DGetString(w, "$Type") == tabControlType }

func countTabPages(arr []any) int {
	n := 0
	for _, e := range arr {
		if d, ok := e.(bson.D); ok && isTabPage(d) {
			n++
		}
	}
	return n
}

func firstTabPage(arr []any) bson.D {
	for _, e := range arr {
		if d, ok := e.(bson.D); ok && isTabPage(d) {
			return d
		}
	}
	return nil
}

// checkSiblingKinds refuses new siblings of the wrong kind: next to a tab page
// only tab pages, next to anything else no tab page.
func checkSiblingKinds(target *bsonWidgetResult, newWidgets []any, widgetRef string) error {
	if isTabPage(target.widget) {
		return requireTabPages(newWidgets, widgetRef)
	}
	return refuseTabPagesOutsideTabControl(newWidgets, widgetRef)
}

// requireTabPages refuses any widget that is not a tab page.
func requireTabPages(newWidgets []any, widgetRef string) error {
	for _, e := range newWidgets {
		d, ok := e.(bson.D)
		if !ok || isTabPage(d) {
			continue
		}
		return fmt.Errorf("cannot place %s %q beside or into tab container content at %q: a tab container holds only tab pages — "+
			"put the widget inside a tab page (INSERT INTO <tabpage>) instead",
			bsonnav.DGetString(d, "$Type"), bsonnav.DGetString(d, "Name"), widgetRef)
	}
	return nil
}

// refuseTabPagesOutsideTabControl refuses a tab page anywhere but a tab container.
func refuseTabPagesOutsideTabControl(newWidgets []any, widgetRef string) error {
	for _, e := range newWidgets {
		if d, ok := e.(bson.D); ok && isTabPage(d) {
			return fmt.Errorf("cannot place tab page %q at %q: a tab page can only sit in a tab container — "+
				"INSERT BEFORE/AFTER an existing tab page, or INSERT INTO the tab container",
				bsonnav.DGetString(d, "Name"), widgetRef)
		}
	}
	return nil
}

// appendTabPages appends tab pages to a TabControl's TabPages list, keeping the
// list marker, and returns the (possibly reallocated) TabControl doc.
func appendTabPages(tabControl bson.D, newPages []any) bson.D {
	existing := bsonnav.DGetArrayElements(bsonnav.DGet(tabControl, "TabPages"))
	if bsonnav.DGet(tabControl, "TabPages") == nil {
		arr := bson.A{int32(3)}
		arr = append(arr, newPages...)
		if bsonnav.DSet(tabControl, "TabPages", arr) {
			return tabControl
		}
		return append(tabControl, bson.E{Key: "TabPages", Value: arr})
	}
	all := make([]any, 0, len(existing)+len(newPages))
	all = append(all, existing...)
	all = append(all, newPages...)
	bsonnav.DSetArray(tabControl, "TabPages", all)
	return tabControl
}

// repointDefaultTabPage moves a TabControl's DefaultPagePointer off a tab page
// that is being removed, onto its successor. A pointer to any other page is
// left alone.
func repointDefaultTabPage(tabControl, removed, successor bson.D) {
	if !isTabControl(tabControl) || successor == nil {
		return
	}
	current := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(tabControl, "DefaultPagePointer"))
	removedID := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(removed, "$ID"))
	if current == "" || current != removedID {
		return
	}
	bsonnav.DSet(tabControl, "DefaultPagePointer", bsonnav.DGet(successor, "$ID"))
}

// refuseTabPageAppearance refuses the Appearance properties on a tab page. It
// has no Appearance, and the generic setter treats a missing Appearance as a
// silent no-op — so `set Class = 'hidden' on tabPage2` would report success and
// change nothing.
func refuseTabPageAppearance(widget bson.D, widgetRef, prop string) error {
	if !isTabPage(widget) {
		return nil
	}
	switch strings.ToLower(prop) {
	case "class", "style", "dynamicclasses":
		return fmt.Errorf("cannot set %s on tab page %q: a tab page has no Appearance (no class or style) — "+
			"use Visible to hide it, or set the class on the tab container", prop, widgetRef)
	}
	return nil
}
