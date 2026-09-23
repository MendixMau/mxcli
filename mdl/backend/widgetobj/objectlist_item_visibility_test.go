// SPDX-License-Identifier: Apache-2.0

// Replacing a PDS Dropdown Menu with its five items failed CE0463: the divider
// item's unset, optional, VISIBLE `caption` was written null where Studio Pro
// stores an empty ClientTemplate. A TextTemplate the editor HIDES on an item is
// still null. Both directions are decided per item by the widget's nested
// visibility rules, so a list whose widget ships none is left alone.
package widgetobj

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
)

const pdsID = "mendix.pdsdropdownmenu.PDSDropdownMenu"

func pdsEntries() map[string]pages.PropertyTypeIDEntry {
	return map[string]pages.PropertyTypeIDEntry{
		"dropdownItems": {
			PropertyTypeID: "10000000000000000000000000000001",
			ObjectTypeID:   "10000000000000000000000000000002",
			NestedKeyOrder: []string{"itemType", "caption", "url"},
			NestedPropertyIDs: map[string]pages.PropertyTypeIDEntry{
				"itemType": {PropertyTypeID: "10000000000000000000000000000003", ValueType: "Enumeration", DefaultValue: "button"},
				"caption":  {PropertyTypeID: "10000000000000000000000000000004", ValueType: "TextTemplate"},
				"url":      {PropertyTypeID: "10000000000000000000000000000005", ValueType: "TextTemplate"},
			},
		},
	}
}

var pdsItemRules = []types.WidgetVisibilityRule{{
	PropertyKey: "url", ListPropertyKey: "dropdownItems",
	HiddenWhen: &types.WidgetVisibilityCondition{PropertyKey: "itemType", Operator: "ne", Value: "link", Scope: types.ConditionScopeItem},
}}

// itemTextTemplates returns, per item, whether each TextTemplate sub-property is null.
func itemTextTemplates(t *testing.T, obj bson.D, entries map[string]pages.PropertyTypeIDEntry) []map[string]bool {
	t.Helper()
	list := entries["dropdownItems"]
	byID := map[string]string{}
	for k, e := range list.NestedPropertyIDs {
		byID[e.PropertyTypeID] = k
	}
	var out []map[string]bool
	for _, p := range obj[0].Value.(bson.A)[1:] {
		val := widgetValueOfProperty(p.(bson.D))
		for _, e := range val {
			if e.Key != "Objects" {
				continue
			}
			for _, it := range e.Value.(bson.A)[1:] {
				m := map[string]bool{}
				for _, ip := range fieldOf(it.(bson.D), "Properties").(bson.A)[1:] {
					k := byID[propertyTypePointerID(ip.(bson.D))]
					if list.NestedPropertyIDs[k].ValueType == "TextTemplate" {
						m[k] = bsonFieldIsNil(widgetValueOfProperty(ip.(bson.D)), "TextTemplate")
					}
				}
				out = append(out, m)
			}
		}
	}
	return out
}

func fieldOf(d bson.D, k string) any {
	for _, e := range d {
		if e.Key == k {
			return e.Value
		}
	}
	return nil
}

func pdsBuilder(entries map[string]pages.PropertyTypeIDEntry, widgetID string) *Builder {
	obj := bson.D{{Key: "Properties", Value: bson.A{int32(2), bson.D{
		{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
		{Key: "TypePointer", Value: types.UUIDToBlob(entries["dropdownItems"].PropertyTypeID)},
		{Key: "Value", Value: bson.D{{Key: "Objects", Value: bson.A{int32(2)}}}},
	}}}}
	return New(widgetID, nil, obj, entries, "", nil)
}

func TestObjectListItem_VisibleUnsetTextTemplateIsEmptyNotNull(t *testing.T) {
	entries := pdsEntries()
	b := pdsBuilder(entries, pdsID)
	b.SetObjectList("dropdownItems", []backend.ObjectListItemSpec{
		{Properties: []backend.ObjectListItemProperty{{PropertyKey: "itemType", Operation: "primitive", PrimitiveVal: "divider"}}},
		{Properties: []backend.ObjectListItemProperty{{PropertyKey: "itemType", Operation: "primitive", PrimitiveVal: "link"}}},
	})
	b.ApplyPropertyVisibility(pdsItemRules)
	got := itemTextTemplates(t, b.object, entries)
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2", len(got))
	}
	// divider: caption visible (empty CT), url hidden (null)
	if got[0]["caption"] || !got[0]["url"] {
		t.Errorf("divider: caption null=%v url null=%v, want false/true", got[0]["caption"], got[0]["url"])
	}
	// link: both visible
	if got[1]["caption"] || got[1]["url"] {
		t.Errorf("link: caption null=%v url null=%v, want false/false", got[1]["caption"], got[1]["url"])
	}
}

// Without nested rules the per-item logic is unknown, so nothing is filled —
// the convention the builder already applies stands.
func TestObjectListItem_NoNestedRulesLeavesItemsAlone(t *testing.T) {
	entries := pdsEntries()
	b := pdsBuilder(entries, pdsID)
	b.SetObjectList("dropdownItems", []backend.ObjectListItemSpec{
		{Properties: []backend.ObjectListItemProperty{{PropertyKey: "itemType", Operation: "primitive", PrimitiveVal: "divider"}}},
	})
	b.ApplyPropertyVisibility([]types.WidgetVisibilityRule{{
		PropertyKey: "other", HiddenWhen: &types.WidgetVisibilityCondition{Operator: types.OperatorAlways},
	}})
	got := itemTextTemplates(t, b.object, entries)
	if !got[0]["caption"] || !got[0]["url"] {
		t.Errorf("items changed without nested rules: %v", got[0])
	}
}
