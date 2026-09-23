// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// `Visible: [expr]` on a pluggable widget is what describe page emits for one
// (the PDS dropdown menu on UserGroups.GroupDetails), so it has to be
// accepted by check and carried by the builder. Before, check refused it as
// MDL-WIDGET01 "has no property `VisibleIf`", which made the describe ->
// replace round trip impossible without deleting the condition by hand.
func TestBuildPluggable_CarriesConditionalVisibility(t *testing.T) {
	e := multiSourceEngine(t)
	w := &ast.WidgetV3{
		Name: "cb",
		Type: "pluggablewidget",
		Properties: map[string]any{
			"WidgetType":                         "com.mendix.widget.web.combobox.Combobox",
			"optionsSourceAssociationDataSource": &ast.DataSourceV3{Type: "database", Reference: "Sales.Order"},
			"Attribute":                          "Number",
			"VisibleIf":                          "$currentObject/Number > 0",
		},
	}
	widget, err := e.pageBuilder.buildPluggable(multiSourceDef(), w)
	if err != nil {
		t.Fatalf("buildPluggable: %v", err)
	}
	cw, ok := widget.(*pages.CustomWidget)
	if !ok {
		t.Fatalf("got %T, want *pages.CustomWidget", widget)
	}
	if cw.ConditionalVisibility == nil {
		t.Fatal("ConditionalVisibility dropped by buildPluggable")
	}
	if cw.ConditionalVisibility.Expression != "$currentObject/Number > 0" {
		t.Errorf("Expression = %q", cw.ConditionalVisibility.Expression)
	}
}

func TestValidatePluggable_AcceptsVisibleIf(t *testing.T) {
	if !isBuiltinPropName("VisibleIf") {
		t.Error("VisibleIf is not a builtin property name, so check refuses `Visible: [expr]` on a pluggable widget (MDL-WIDGET01)")
	}
}
