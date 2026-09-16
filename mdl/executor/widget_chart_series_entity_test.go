// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// lineSeriesMapping is a LineChart `lines` object list carrying both of a
// series' datasources and the dependents the widget links to each. The links are
// the ones GenerateDefJSON already emits from the .mpk — `staticXAttribute`
// declares `staticDataSource`, `dynamicXAttribute` declares `dynamicDataSource`
// (widget.xml `dataSource="…"`), so this is the shipped definition's shape, not
// an invented one.
func lineSeriesMapping() *ObjectListMapping {
	return &ObjectListMapping{
		PropertyKey:  "lines",
		MDLContainer: "LINE",
		ItemProperties: []ItemPropertyMapping{
			{PropertyKey: "dataSet", Operation: "primitive"},
			{PropertyKey: "staticDataSource", Source: "DataSource", Operation: "datasource"},
			{PropertyKey: "dynamicDataSource", Source: "DataSource", Operation: "datasource"},
			{PropertyKey: "staticXAttribute", Source: "Attribute", Operation: "attribute", DataSource: "staticDataSource"},
			{PropertyKey: "dynamicXAttribute", Source: "Attribute", Operation: "attribute", DataSource: "dynamicDataSource"},
		},
	}
}

// A series configuring BOTH datasources binds each dependent against ITS OWN.
//
// The item builder pre-resolves every datasource the item sets and used to drop
// each entity into the one shared pageBuilder.entityContext, so the LAST one
// won: measured on Mendix 11.6.6, a line chart series given a static source over
// SalesByRegion and a dynamic one over Forecast wrote its static x/y attributes
// as Sales.Customer.Number and Sales.Customer.Total — attributes that entity does not
// have — and mxbuild reported CE1613 "The selected attribute … no longer
// exists." twice.
func TestBuildObjectListItem_AttributeBindsToItsOwnSeriesDataSource(t *testing.T) {
	e := multiSourceEngine(t)
	child := &ast.WidgetV3{
		Name: "mixed",
		Properties: map[string]any{
			"dataSet":           "static",
			"staticDataSource":  &ast.DataSourceV3{Type: "database", Reference: "Sales.Order"},
			"staticXAttribute":  "Number",
			"dynamicDataSource": &ast.DataSourceV3{Type: "database", Reference: "Sales.Customer"},
			"dynamicXAttribute": "Name",
		},
	}

	spec, err := e.buildObjectListItem(lineSeriesMapping(), child)
	if err != nil {
		t.Fatalf("buildObjectListItem: %v", err)
	}

	got := map[string]string{}
	for _, p := range spec.Properties {
		if p.Operation == "attribute" {
			got[p.PropertyKey] = p.AttributePath
		}
	}
	want := map[string]string{
		"staticXAttribute":  "Sales.Order.Number",
		"dynamicXAttribute": "Sales.Customer.Name",
	}
	for key, wantPath := range want {
		if got[key] != wantPath {
			t.Errorf("%s = %q, want %q", key, got[key], wantPath)
		}
	}
}

// The control, and the failure it replaces: with no declared link, every
// dependent falls back to the shared context — which is the LAST datasource the
// item configured, so the static attribute lands on the dynamic entity. This is
// what the code did for every property before the link was read, and it is what
// mxbuild reported as CE1613.
func TestBuildObjectListItem_WithoutTheLinkTheLastDataSourceWins(t *testing.T) {
	e := multiSourceEngine(t)
	m := lineSeriesMapping()
	for i := range m.ItemProperties {
		m.ItemProperties[i].DataSource = "" // as if the widget declared no link
	}
	child := &ast.WidgetV3{
		Name: "mixed",
		Properties: map[string]any{
			"dataSet":           "static",
			"staticDataSource":  &ast.DataSourceV3{Type: "database", Reference: "Sales.Order"},
			"staticXAttribute":  "Number",
			"dynamicDataSource": &ast.DataSourceV3{Type: "database", Reference: "Sales.Customer"},
		},
	}

	spec, err := e.buildObjectListItem(m, child)
	if err != nil {
		t.Fatalf("buildObjectListItem: %v", err)
	}
	for _, p := range spec.Properties {
		if p.PropertyKey == "staticXAttribute" && p.AttributePath != "Sales.Customer.Number" {
			t.Fatalf("control did not reproduce the old behaviour: staticXAttribute = %q, want the "+
				"WRONG entity Sales.Customer.Number (if this fails, the test above proves nothing)",
				p.AttributePath)
		}
	}
}

// A series configuring ONE datasource — every series in every real chart, since
// `dataSet` selects static or dynamic — resolves exactly as before, through the
// shared context. This is why the change is invisible to existing charts.
func TestBuildObjectListItem_SingleSeriesDataSourceUnchanged(t *testing.T) {
	e := multiSourceEngine(t)
	child := &ast.WidgetV3{
		Name: "onlyStatic",
		Properties: map[string]any{
			"dataSet":          "static",
			"staticDataSource": &ast.DataSourceV3{Type: "database", Reference: "Sales.Order"},
			"staticXAttribute": "Number",
		},
	}

	spec, err := e.buildObjectListItem(lineSeriesMapping(), child)
	if err != nil {
		t.Fatalf("buildObjectListItem: %v", err)
	}
	for _, p := range spec.Properties {
		if p.PropertyKey == "staticXAttribute" && p.AttributePath != "Sales.Order.Number" {
			t.Errorf("staticXAttribute = %q, want Sales.Order.Number", p.AttributePath)
		}
	}
}
