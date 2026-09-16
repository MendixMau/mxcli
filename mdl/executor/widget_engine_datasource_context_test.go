// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	mwidgets "github.com/mendixlabs/mxcli/modelsdk/widgets"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// dropdownFilterPropertyTypes loads the REAL DatagridDropdownFilter template
// metadata. Hand-writing the DataSourceProperty links would make the test agree
// with itself rather than with the widget: the whole point of the change is that
// the link comes from the widget's own package.
func dropdownFilterPropertyTypes(t *testing.T) map[string]pages.PropertyTypeIDEntry {
	t.Helper()
	_, _, ids, _, _, err := mwidgets.GetTemplateFullBSON(
		"com.mendix.widget.web.datagriddropdownfilter.DatagridDropdownFilter", mmpr.GenerateID, "")
	if err != nil {
		t.Fatalf("load DatagridDropdownFilter template: %v", err)
	}
	if ids["attr"].DataSourceProperty != "linkedDs" || ids["refCaption"].DataSourceProperty != "refOptions" {
		t.Fatalf("template does not carry the datasource links this test is about: "+
			"attr=%q refCaption=%q", ids["attr"].DataSourceProperty, ids["refCaption"].DataSourceProperty)
	}
	return ids
}

func newDataSourceContextEngine(t *testing.T, ids map[string]pages.PropertyTypeIDEntry) *PluggableWidgetEngine {
	t.Helper()
	return &PluggableWidgetEngine{
		pageBuilder: &pageBuilder{
			// The LAST datasource applied wins the shared context. Making it the
			// grid's list means a dependent of refOptions that ignores its own
			// link lands on Sales.Order — a wrong, and plausible, answer.
			entityContext:    "Sales.Order",
			paramEntityNames: map[string]string{},
			widgetScope:      map[string]model.ID{},
		},
		currentPropertyTypeIDs: ids,
		dataSourceEntities: map[string]string{
			"linkedDs":   "Sales.Order",
			"refOptions": "Sales.Customer",
		},
	}
}

// A dependent property binds against the datasource the WIDGET says it belongs
// to, not against whichever datasource mapping happened to run last.
//
// DatagridDropdownFilter is the case: `linkedDs` (the grid's rows) and
// `refOptions` (the association target's option list) are populated at the same
// time, and `attr` belongs to the first while `refCaption` and `refSearchAttr`
// belong to the second.
func TestResolveMapping_AttributeBindsToItsOwnDataSource(t *testing.T) {
	ids := dropdownFilterPropertyTypes(t)
	e := newDataSourceContextEngine(t, ids)

	tests := []struct {
		propertyKey string
		mdlValue    string
		want        string
	}{
		{"attr", "Number", "Sales.Order.Number"},
		{"refCaption", "Name", "Sales.Customer.Name"},
		{"refSearchAttr", "Name", "Sales.Customer.Name"},
	}

	for _, tc := range tests {
		t.Run(tc.propertyKey, func(t *testing.T) {
			mapping := PropertyMapping{PropertyKey: tc.propertyKey, Source: "Attribute", Operation: "attribute"}
			w := &ast.WidgetV3{Name: "filter", Properties: map[string]any{tc.propertyKey: tc.mdlValue}}

			ctx, err := e.resolveMapping(mapping, w)
			if err != nil {
				t.Fatalf("resolveMapping: %v", err)
			}
			if ctx.AttributePath != tc.want {
				t.Errorf("AttributePath = %q, want %q", ctx.AttributePath, tc.want)
			}
		})
	}
}

// The control for the test above, and the reason it is worth having.
//
// With no template metadata — which is exactly the state the engine was in
// before PropertyTypeIDEntry carried DataSourceProperty — every property falls
// back to the one shared entityContext, so `refCaption` binds to the GRID's
// entity. That is a reference to an attribute of the wrong entity: valid-looking
// BSON, and CE1613 at build time.
func TestResolveMapping_WithoutTemplateLinksBindsToSharedContext(t *testing.T) {
	e := newDataSourceContextEngine(t, nil) // no DataSourceProperty links

	mapping := PropertyMapping{PropertyKey: "refCaption", Source: "Attribute", Operation: "attribute"}
	w := &ast.WidgetV3{Name: "filter", Properties: map[string]any{"refCaption": "Name"}}

	ctx, err := e.resolveMapping(mapping, w)
	if err != nil {
		t.Fatalf("resolveMapping: %v", err)
	}
	if ctx.AttributePath != "Sales.Order.Name" {
		t.Fatalf("control did not reproduce the old behaviour: AttributePath = %q, want %q "+
			"(if this fails, the test above is no longer proving anything)",
			ctx.AttributePath, "Sales.Order.Name")
	}
}

// A property with no DataSourceProperty — every property of every
// single-datasource widget — keeps using the shared entity context. This is what
// makes the change inert for every widget shipped today.
func TestEntityContextFor_FallsBackToSharedContext(t *testing.T) {
	ids := dropdownFilterPropertyTypes(t)
	e := newDataSourceContextEngine(t, ids)

	// `valueAttribute` is declared with no dataSource in the widget package.
	if got := ids["valueAttribute"].DataSourceProperty; got != "" {
		t.Fatalf("precondition: valueAttribute.DataSourceProperty = %q, want empty", got)
	}
	if got := e.entityContextFor("valueAttribute"); got != "Sales.Order" {
		t.Errorf("entityContextFor(valueAttribute) = %q, want the shared context Sales.Order", got)
	}
	// A key the template does not declare at all must not panic or divert.
	if got := e.entityContextFor("noSuchProperty"); got != "Sales.Order" {
		t.Errorf("entityContextFor(unknown) = %q, want the shared context Sales.Order", got)
	}
}

// A datasource whose value did not resolve to an entity must not shadow the
// shared context with an empty string.
func TestEntityContextFor_UnresolvedDataSourceFallsBack(t *testing.T) {
	ids := dropdownFilterPropertyTypes(t)
	e := newDataSourceContextEngine(t, ids)
	delete(e.dataSourceEntities, "refOptions")

	if got := e.entityContextFor("refCaption"); got != "Sales.Order" {
		t.Errorf("entityContextFor(refCaption) = %q, want the shared context Sales.Order", got)
	}
}
