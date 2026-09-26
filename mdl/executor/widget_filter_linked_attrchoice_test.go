// SPDX-License-Identifier: Apache-2.0

// A data-grid filter widget given an explicit `Attributes: [...]` list is written
// with attrChoice "linked" ("Custom" in Studio Pro) — widgetobj.Builder's
// SetAttributeObjects forces it, because "auto" beside a populated list is
// definition drift. The checker must read the same value the writer stores.
//
// It did not: the hand-written filter definitions map attrChoice to the fixed
// value "auto", so MDL-WIDGET10 evaluated editorConfig's "attributes is hidden
// when attrChoice is auto" and told the author the list "will be ignored" — on a
// widget whose stored BSON carries attrChoice "linked" and the attribute list,
// the exact shape of Studio Pro's WorkflowCommons.MyInitiatedWorkflows gallery
// filters. The false warning is what sent a real project to Studio Pro for a
// Gallery text/date filter mxcli had been writing correctly all along.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestWidgetValueMap_FilterAttributesListSelectsLinked(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	for _, name := range []string{"TEXTFILTER", "DATEFILTER", "NUMBERFILTER"} {
		def, ok := reg.Get(name)
		if !ok {
			t.Fatalf("%s: no definition", name)
		}
		w := &ast.WidgetV3{Type: name, Name: "f", Properties: map[string]any{
			"Attributes": []string{"Name"},
		}}
		values, _ := widgetValueMap(w, def)
		if got := values["attrchoice"]; got != "linked" {
			t.Errorf("%s with an Attributes list: attrChoice resolved to %q; the writer stores \"linked\", so MDL-WIDGET10 judges a configuration that is never written", name, got)
		}

		// Control: without a list the writer leaves the definition's value alone.
		bare := &ast.WidgetV3{Type: name, Name: "f", Properties: map[string]any{}}
		values, _ = widgetValueMap(bare, def)
		if got := values["attrchoice"]; got == "linked" {
			t.Errorf("%s without an Attributes list: attrChoice resolved to \"linked\"; only a populated list forces it", name)
		}
	}
}
