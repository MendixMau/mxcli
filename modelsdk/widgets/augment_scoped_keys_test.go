// SPDX-License-Identifier: Apache-2.0

package widgets

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// A property key used both at the top level and inside an object-list must be
// reconciled against its OWN definition. PDSDropdownMenu declares `caption`
// twice — top-level in "General::Dropdown Menu", and per dropdown item in
// "General::Dropdown Items::Actions" — and the reconcile passes used one index
// flattened across both levels, so the nested definition overwrote the top-level
// PropertyType's Category. Every freshly built instance then differed from the
// installed package and mxbuild reported CE0463 "The definition of this widget
// has changed" on a page that `alter page … replace` had just rewritten.
func TestGenerateFromMPK_SameKeyAtTwoLevelsKeepsOwnDefinition(t *testing.T) {
	def := &mpk.WidgetDefinition{
		ID: "example.dropdown.Dropdown",
		Properties: []mpk.PropertyDef{
			{Key: "caption", Type: "textTemplate", Caption: "Caption", Category: "General::Dropdown Menu"},
			{Key: "items", Type: "object", IsList: true, Required: true, Caption: "Items", Category: "General::Dropdown Items",
				Children: []mpk.PropertyDef{
					{Key: "itemType", Type: "enumeration", Caption: "Item type", Category: "General::Dropdown Items::Actions",
						DefaultValue: "button", Required: true,
						EnumValues: []mpk.EnumValue{{Key: "button", Caption: "Button"}, {Key: "divider", Caption: "Divider"}}},
					{Key: "caption", Type: "textTemplate", Caption: "Item caption", Category: "General::Dropdown Items::Actions"},
				}},
			{Key: "itemType", Type: "enumeration", Caption: "Top type", Category: "General::Dropdown Menu",
				DefaultValue: "a", Required: true,
				EnumValues: []mpk.EnumValue{{Key: "a", Caption: "A"}}},
		},
	}
	def.AllTopLevel = def.Properties

	tmpl := GenerateFromMPK(def)

	top := propertyTypesOf(t, tmpl.Type["ObjectType"])
	nested := propertyTypesOf(t, top["items"]["ValueType"].(map[string]any)["ObjectType"])

	if got := top["caption"]["Category"]; got != "General::Dropdown Menu" {
		t.Errorf("top-level caption Category = %v, want General::Dropdown Menu (nested definition leaked up)", got)
	}
	if got := top["caption"]["Caption"]; got != "Caption" {
		t.Errorf("top-level caption Caption = %v, want Caption", got)
	}
	if got := nested["caption"]["Category"]; got != "General::Dropdown Items::Actions" {
		t.Errorf("nested caption Category = %v, want General::Dropdown Items::Actions", got)
	}
	// Enumerations too: each level keeps its own option set and default.
	if got := enumKeys(top["itemType"]); len(got) != 1 || got[0] != "a" {
		t.Errorf("top-level itemType options = %v, want [a]", got)
	}
	if got := enumKeys(nested["itemType"]); len(got) != 2 {
		t.Errorf("nested itemType options = %v, want [button divider]", got)
	}
	if got := top["itemType"]["ValueType"].(map[string]any)["DefaultValue"]; got != "a" {
		t.Errorf("top-level itemType default = %v, want a", got)
	}
}

func propertyTypesOf(t *testing.T, objType any) map[string]map[string]any {
	t.Helper()
	ot, ok := objType.(map[string]any)
	if !ok {
		t.Fatalf("ObjectType missing: %T", objType)
	}
	out := map[string]map[string]any{}
	pts, _ := ot["PropertyTypes"].([]any)
	for _, pt := range pts {
		if m, ok := pt.(map[string]any); ok {
			if k, _ := m["PropertyKey"].(string); k != "" {
				out[k] = m
			}
		}
	}
	return out
}

func enumKeys(pt map[string]any) []string {
	vt, _ := pt["ValueType"].(map[string]any)
	vals, _ := vt["EnumerationValues"].([]any)
	var out []string
	for _, v := range vals {
		if m, ok := v.(map[string]any); ok {
			out = append(out, m["_Key"].(string))
		}
	}
	return out
}
