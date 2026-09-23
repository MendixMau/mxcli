// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// The fixtures below are a pluggable widget with one object-list property,
// shaped like the PDS dropdown menu: Type.ObjectType.PropertyTypes holds
// `dropdownItems` (an object list) whose ValueType.ObjectType declares the
// entry's own properties; Object.Properties holds the entries, each pointing
// back into the Type by TypePointer. Every call mints fresh ids — exactly how
// a freshly built donor differs from the widget stored on the page.

const menuWidgetID = "mendix.pdsdropdownmenu.PDSDropdownMenu"

type menuTypeIDs struct {
	objectType, listPT, listVT, itemOT, itemTypePT, itemTypeVT any
}

func newMenuTypeIDs() menuTypeIDs {
	return menuTypeIDs{
		objectType: bsonutil.NewIDBsonBinary(),
		listPT:     bsonutil.NewIDBsonBinary(),
		listVT:     bsonutil.NewIDBsonBinary(),
		itemOT:     bsonutil.NewIDBsonBinary(),
		itemTypePT: bsonutil.NewIDBsonBinary(),
		itemTypeVT: bsonutil.NewIDBsonBinary(),
	}
}

func menuItem(ids menuTypeIDs, itemType string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
		{Key: "Properties", Value: bson.A{int32(2), bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
			{Key: "TypePointer", Value: ids.itemTypePT},
			{Key: "Value", Value: bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
				{Key: "PrimitiveValue", Value: itemType},
				{Key: "TypePointer", Value: ids.itemTypeVT},
			}},
		}}},
		{Key: "TypePointer", Value: ids.itemOT},
	}
}

func menuWidget(name string, ids menuTypeIDs, itemTypes ...string) bson.D {
	items := bson.A{int32(2)}
	for _, it := range itemTypes {
		items = append(items, menuItem(ids, it))
	}
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: name},
		{Key: "Object", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "CustomWidgets$WidgetObject"},
			{Key: "Properties", Value: bson.A{int32(2), bson.D{
				{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
				{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
				{Key: "TypePointer", Value: ids.listPT},
				{Key: "Value", Value: bson.D{
					{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
					{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
					{Key: "Objects", Value: items},
					{Key: "TypePointer", Value: ids.listVT},
				}},
			}}},
			{Key: "TypePointer", Value: ids.objectType},
		}},
		{Key: "Type", Value: bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "CustomWidgets$CustomWidgetType"},
			{Key: "WidgetId", Value: menuWidgetID},
			{Key: "ObjectType", Value: bson.D{
				{Key: "$ID", Value: ids.objectType},
				{Key: "$Type", Value: "CustomWidgets$WidgetObjectType"},
				{Key: "PropertyTypes", Value: bson.A{int32(2), bson.D{
					{Key: "$ID", Value: ids.listPT},
					{Key: "$Type", Value: "CustomWidgets$WidgetPropertyType"},
					{Key: "PropertyKey", Value: "dropdownItems"},
					{Key: "ValueType", Value: bson.D{
						{Key: "$ID", Value: ids.listVT},
						{Key: "$Type", Value: "CustomWidgets$WidgetValueType"},
						{Key: "IsList", Value: true},
						{Key: "Type", Value: "Object"},
						{Key: "ObjectType", Value: bson.D{
							{Key: "$ID", Value: ids.itemOT},
							{Key: "$Type", Value: "CustomWidgets$WidgetObjectType"},
							{Key: "PropertyTypes", Value: bson.A{int32(2), bson.D{
								{Key: "$ID", Value: ids.itemTypePT},
								{Key: "$Type", Value: "CustomWidgets$WidgetPropertyType"},
								{Key: "PropertyKey", Value: "itemType"},
								{Key: "ValueType", Value: bson.D{
									{Key: "$ID", Value: ids.itemTypeVT},
									{Key: "$Type", Value: "CustomWidgets$WidgetValueType"},
									{Key: "Type", Value: "Enumeration"},
								}},
							}}},
						}},
					}},
				}}},
			}},
		}},
	}
}

// donorDeps serializes every widget to one prebuilt donor document.
type donorDeps struct {
	Deps
	donor bson.D
}

func (d *donorDeps) SerializeWidget(pages.Widget) bson.D { return d.donor }

func storedItemTypes(t *testing.T, m *Mutator, owner string) []string {
	t.Helper()
	res := m.widgetFinder(m.rawData, owner)
	if res == nil {
		t.Fatalf("%s not found", owner)
	}
	var out []string
	for _, loc := range objectListsOf(res.widget) {
		for _, it := range loc.items() {
			props := bsonnav.DGetArrayElements(bsonnav.DGet(it.(bson.D), "Properties"))
			v := bsonnav.DGetDoc(props[0].(bson.D), "Value")
			out = append(out, bsonnav.DGetString(v, "PrimitiveValue"))
		}
	}
	return out
}

// collectTypePointers returns every TypePointer id under v.
func collectTypePointers(v any, out *[]string) {
	switch x := v.(type) {
	case bson.D:
		for _, e := range x {
			if e.Key == "TypePointer" {
				*out = append(*out, bsonnav.ExtractBinaryIDFromDoc(e.Value))
				continue
			}
			collectTypePointers(e.Value, out)
		}
	case bson.A:
		for _, el := range x {
			collectTypePointers(el, out)
		}
	}
}

func idOf(v any) string { return bsonnav.ExtractBinaryIDFromDoc(v) }

// TestInsertObjectListItems_RepointsDonorEntriesAtStoredType is the load-bearing
// assertion. The donor is built fresh, so its entries point at the DONOR's
// Type ids; spliced in unchanged they would point at nothing on the page, which
// Studio Pro reports as CE0463 (or cannot load at all). Every TypePointer in the
// inserted entry must name an id of the owner's stored Type.
func TestInsertObjectListItems_RepointsDonorEntriesAtStoredType(t *testing.T) {
	stored := newMenuTypeIDs()
	fresh := newMenuTypeIDs()
	m := New(makeRawPage(menuWidget("menu1", stored, "button", "divider")), model.ID("u"),
		&donorDeps{donor: menuWidget("donor", fresh, "link")})

	if err := m.InsertObjectListItems("menu1", "dropdownItems", 1, &pages.CustomWidget{}); err != nil {
		t.Fatalf("InsertObjectListItems: %v", err)
	}
	if got := strings.Join(storedItemTypes(t, m, "menu1"), ","); got != "button,link,divider" {
		t.Fatalf("entries = %s, want button,link,divider", got)
	}

	res := m.widgetFinder(m.rawData, "menu1")
	inserted := objectListsOf(res.widget)[0].items()[1]
	var ptrs []string
	collectTypePointers(inserted, &ptrs)
	want := map[string]bool{idOf(stored.itemOT): true, idOf(stored.itemTypePT): true, idOf(stored.itemTypeVT): true}
	if len(ptrs) != 3 {
		t.Fatalf("inserted entry has %d TypePointers, want 3", len(ptrs))
	}
	for _, p := range ptrs {
		if !want[p] {
			t.Errorf("TypePointer %s does not name the stored Type (a donor id leaked through)", p)
		}
	}
}

func TestInsertObjectListItems_AppendAndIntoEmptyList(t *testing.T) {
	stored := newMenuTypeIDs()
	m := New(makeRawPage(menuWidget("menu1", stored)), model.ID("u"),
		&donorDeps{donor: menuWidget("donor", newMenuTypeIDs(), "button", "divider")})
	if err := m.InsertObjectListItems("menu1", "dropdownItems", -1, &pages.CustomWidget{}); err != nil {
		t.Fatalf("InsertObjectListItems: %v", err)
	}
	if got := strings.Join(storedItemTypes(t, m, "menu1"), ","); got != "button,divider" {
		t.Fatalf("entries = %s, want button,divider", got)
	}
}

// A donor pointer with no counterpart in the stored Type (the stored widget is
// an older version of the package, say) must refuse with nothing written:
// guessing a target is what produces a document Studio Pro cannot open.
func TestInsertObjectListItems_UnmappedPointerRefusesAndWritesNothing(t *testing.T) {
	stored := newMenuTypeIDs()
	fresh := newMenuTypeIDs()
	donor := menuWidget("donor", fresh, "link")
	// Point the donor entry's value at an id its own Type does not declare.
	item := objectListsOf(donor)[0].items()[0].(bson.D)
	prop := bsonnav.DGetArrayElements(bsonnav.DGet(item, "Properties"))[0].(bson.D)
	bsonnav.DSet(bsonnav.DGetDoc(prop, "Value"), "TypePointer", bsonutil.NewIDBsonBinary())

	m := New(makeRawPage(menuWidget("menu1", stored, "button")), model.ID("u"), &donorDeps{donor: donor})
	err := m.InsertObjectListItems("menu1", "dropdownItems", 1, &pages.CustomWidget{})
	if err == nil {
		t.Fatal("expected a refusal for an unmapped TypePointer")
	}
	if got := strings.Join(storedItemTypes(t, m, "menu1"), ","); got != "button" {
		t.Errorf("entries = %s after a refusal, want the list untouched", got)
	}
}

func TestResolveObjectListItem(t *testing.T) {
	m := New(makeRawPage(
		menuWidget("menu1", newMenuTypeIDs(), "button", "divider"),
		menuWidget("menu2", newMenuTypeIDs(), "button"),
		makeWidget("dropdownitem9", "Forms$TextBox"),
	), model.ID("u"), &stubWidgetDeps{})

	ref, ok, err := m.ResolveObjectListItem("dropdownitem2", "")
	if err != nil || !ok || ref.Owner != "menu1" || ref.Index != 1 || ref.ListKey != "dropdownItems" ||
		ref.WidgetID != menuWidgetID || ref.Keyword != "dropdownitem" {
		t.Errorf("bare dropdownitem2 = %+v ok=%v err=%v, want menu1 index 1", ref, ok, err)
	}

	ref, ok, err = m.ResolveObjectListItem("menu2", "dropdownitem1")
	if err != nil || !ok || ref.Owner != "menu2" || ref.Index != 0 {
		t.Errorf("menu2.dropdownitem1 = %+v ok=%v err=%v", ref, ok, err)
	}

	// Both menus have a first entry: a bare name must not pick one.
	if _, _, err = m.ResolveObjectListItem("dropdownitem1", ""); err == nil ||
		!strings.Contains(err.Error(), "menu1.dropdownitem1") || !strings.Contains(err.Error(), "menu2.dropdownitem1") {
		t.Errorf("ambiguous bare name: err = %v, want both qualified forms named", err)
	}

	// Past the end of the list: an error naming how many there are.
	if _, _, err = m.ResolveObjectListItem("menu1", "dropdownitem5"); err == nil || !strings.Contains(err.Error(), "2") {
		t.Errorf("out of range: err = %v", err)
	}

	// A real widget of that name wins, and a non-entry name is not an entry.
	if _, ok, err = m.ResolveObjectListItem("dropdownitem9", ""); ok || err != nil {
		t.Errorf("a widget named like an entry resolved as one: ok=%v err=%v", ok, err)
	}
	if _, ok, err = m.ResolveObjectListItem("somethingElse", ""); ok || err != nil {
		t.Errorf("unrelated name: ok=%v err=%v", ok, err)
	}

	if got := m.PluggableWidgetID("menu1"); got != menuWidgetID {
		t.Errorf("PluggableWidgetID(menu1) = %q", got)
	}
	if got := m.PluggableWidgetID("dropdownitem9"); got != "" {
		t.Errorf("PluggableWidgetID(textbox) = %q, want empty", got)
	}
}

func TestDropObjectListItems(t *testing.T) {
	m := New(makeRawPage(menuWidget("menu1", newMenuTypeIDs(), "button", "divider", "link")), model.ID("u"), &stubWidgetDeps{})
	if err := m.DropObjectListItems("menu1", "dropdownItems", []int{0, 2}); err != nil {
		t.Fatalf("DropObjectListItems: %v", err)
	}
	if got := strings.Join(storedItemTypes(t, m, "menu1"), ","); got != "divider" {
		t.Errorf("entries = %s, want divider", got)
	}
	if err := m.DropObjectListItems("menu1", "dropdownItems", []int{4}); err == nil {
		t.Error("expected an error for an index past the end")
	}
}
