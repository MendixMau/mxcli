// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
)

// multiSourceDef is a ComboBox definition that maps BOTH of the widget's
// datasources in one mode, which no shipped definition does: mxcli models each
// widget as single-source-per-mode, and the ComboBox's two are mode-exclusive
// (association vs database). The widget PACKAGE declares both as authorable, so
// this is a definition the registry would accept from a project.
//
// ComboBox rather than the DatagridDropdownFilter that looks like the obvious
// multi-source widget: the filter's second datasource is `isLinked`, owned by
// the containing grid and refused by refuseLinkedDataSourceMapping. Measured
// across every widget package in testdata/expr-checker, it is the only one of
// the eight multi-datasource widgets with a linked source, and so the only one
// that is NOT multi-source from MDL's side.
func multiSourceDef() *WidgetDefinition {
	return &WidgetDefinition{
		WidgetID:     "com.mendix.widget.web.combobox.Combobox",
		MDLName:      "COMBOBOX",
		TemplateFile: "combobox.json",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "optionsSourceAssociationDataSource", Source: "DataSource", Operation: "datasource",
				MdlAliases: []string{"AssociationSource"}},
			{PropertyKey: "optionsSourceAssociationCaptionAttribute", Source: "Attribute", Operation: "attribute"},
			{PropertyKey: "optionsSourceDatabaseDataSource", Source: "DataSource", Operation: "datasource",
				MdlAliases: []string{"DatabaseSource"}},
			{PropertyKey: "optionsSourceDatabaseCaptionAttribute", Source: "CaptionAttribute", Operation: "attribute"},
		},
	}
}

func multiSourceEngine(t *testing.T) *PluggableWidgetEngine {
	t.Helper()
	real := &modelsdkbackend.Backend{}
	mod := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-sales")}, Name: "Sales"}
	b := &mock.MockBackend{
		LoadWidgetTemplateFunc: real.LoadWidgetTemplate,
		ListModulesFunc:        func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{{
				BaseElement: model.BaseElement{ID: model.ID("dm-sales")},
				ContainerID: mod.ID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: model.ID("e-order")}, Name: "Order",
						Attributes: []*domainmodel.Attribute{{Name: "Number"}}},
					{BaseElement: model.BaseElement{ID: model.ID("e-customer")}, Name: "Customer",
						Attributes: []*domainmodel.Attribute{{Name: "Name"}}},
				},
			}}, nil
		},
	}
	pb := &pageBuilder{
		backend:          b,
		paramEntityNames: map[string]string{},
		widgetScope:      map[string]model.ID{},
	}
	e := NewPluggableWidgetEngine(b, pb)
	pb.pluggableEngine = e
	return e
}

// The capability #1109 asks for: two datasources populated at once, each
// addressed by its own schema key, with each dependent property bound to ITS
// datasource's entity rather than to whichever one was applied last.
func TestBuild_TwoNamedDataSources(t *testing.T) {
	e := multiSourceEngine(t)
	w := &ast.WidgetV3{
		Name: "cb",
		Type: "pluggablewidget",
		Properties: map[string]any{
			"WidgetType":                         "com.mendix.widget.web.combobox.Combobox",
			"optionsSourceAssociationDataSource": &ast.DataSourceV3{Type: "database", Reference: "Sales.Order"},
			"Attribute":                          "Number",
			// The alias resolves the same as the key would.
			"DatabaseSource":   &ast.DataSourceV3{Type: "database", Reference: "Sales.Customer"},
			"CaptionAttribute": "Name",
		},
	}

	widget, err := e.Build(multiSourceDef(), w)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := renderedWidgetStrings(t, widget)

	// Both datasources survive. Before, one generic `datasource:` slot meant one
	// of them was always unset, which mxbuild reports as CE0642 against a
	// property the author never mentioned.
	for _, want := range []string{"Sales.Order", "Sales.Customer"} {
		if !strings.Contains(got, want) {
			t.Errorf("built widget does not carry datasource entity %q", want)
		}
	}
	// And each dependent bound to its OWN datasource's entity: the association
	// caption declares optionsSourceAssociationDataSource, the database caption
	// declares optionsSourceDatabaseDataSource (widget.xml `dataSource=`).
	for _, want := range []string{"Sales.Order.Number", "Sales.Customer.Name"} {
		if !strings.Contains(got, want) {
			t.Errorf("built widget does not carry attribute path %q", want)
		}
	}
	// The failure this replaces: both dependents against the same entity.
	if strings.Contains(got, "Sales.Order.Name") || strings.Contains(got, "Sales.Customer.Number") {
		t.Errorf("a dependent bound to the WRONG datasource's entity")
	}
}

// The generic `datasource:` clause says "the datasource" where the widget has
// several. Refused, naming the keys, rather than guessed at.
func TestBuild_GenericClauseRefusedOnMultiSourceWidget(t *testing.T) {
	e := multiSourceEngine(t)
	w := &ast.WidgetV3{
		Name: "cb",
		Type: "pluggablewidget",
		Properties: map[string]any{
			"WidgetType": "com.mendix.widget.web.combobox.Combobox",
			"DataSource": &ast.DataSourceV3{Type: "database", Reference: "Sales.Customer"},
		},
	}

	_, err := e.Build(multiSourceDef(), w)
	if err == nil {
		t.Fatal("expected a refusal for an ambiguous generic datasource clause")
	}
	for _, want := range []string{"ambiguous", "optionsSourceAssociationDataSource", "optionsSourceDatabaseDataSource"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal should mention %q, got: %v", want, err)
		}
	}
}

// A single-datasource widget keeps the generic clause it has always taken. This
// is the compatibility half, and it is why the change is invisible to every
// definition shipped today.
func TestBuild_GenericClauseStillWorksOnSingleSourceWidget(t *testing.T) {
	def := multiSourceDef()
	// Drop one datasource mapping, leaving the widget single-source.
	var kept []PropertyMapping
	for _, m := range def.PropertyMappings {
		if m.PropertyKey != "optionsSourceAssociationDataSource" {
			kept = append(kept, m)
		}
	}
	def.PropertyMappings = kept

	e := multiSourceEngine(t)
	w := &ast.WidgetV3{
		Name: "cb",
		Type: "pluggablewidget",
		Properties: map[string]any{
			"WidgetType":       "com.mendix.widget.web.combobox.Combobox",
			"DataSource":       &ast.DataSourceV3{Type: "database", Reference: "Sales.Customer"},
			"CaptionAttribute": "Name",
		},
	}

	widget, err := e.Build(def, w)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	got := renderedWidgetStrings(t, widget)
	if !strings.Contains(got, "Sales.Customer") {
		t.Error("generic datasource clause was not applied on a single-source widget")
	}
	if !strings.Contains(got, "Sales.Customer.Name") {
		t.Error("dependent attribute did not bind to the generic clause's entity")
	}
}

// A definition that maps a LINKED datasource is refused. The platform fills one
// from the containing widget, so a value written there is not the author's to
// set — and does not even satisfy the property: measured on Mendix 11.6.6, a
// drop-down filter written WITH linkedDs still fails CE0642 "Property
// 'Datasource to Filter' is required", while the same filter written without it
// (inside a Data Grid 2 column, where the grid supplies it) passes at 0 errors.
func TestBuild_LinkedDataSourceMappingRefused(t *testing.T) {
	e := multiSourceEngine(t)
	def := &WidgetDefinition{
		WidgetID:     "com.mendix.widget.web.datagriddropdownfilter.DatagridDropdownFilter",
		MDLName:      "DROPDOWNFILTER",
		TemplateFile: "datagrid-dropdown-filter.json",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "baseType", Value: "ref", Operation: "primitive"},
			{PropertyKey: "linkedDs", Source: "DataSource", Operation: "datasource"},
			{PropertyKey: "refOptions", Source: "DataSource", Operation: "datasource"},
		},
	}
	w := &ast.WidgetV3{Name: "f", Type: "pluggablewidget", Properties: map[string]any{
		"WidgetType": "com.mendix.widget.web.datagriddropdownfilter.DatagridDropdownFilter",
		"linkedDs":   &ast.DataSourceV3{Type: "database", Reference: "Sales.Order"},
		"refOptions": &ast.DataSourceV3{Type: "database", Reference: "Sales.Customer"},
	}}

	_, err := e.Build(def, w)
	if err == nil {
		t.Fatal("expected a refusal for a definition mapping a linked datasource")
	}
	for _, want := range []string{"linkedDs", "LINKED"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal should mention %q, got: %v", want, err)
		}
	}
}

// The control: the SAME definition without the linked mapping builds. So the
// refusal is about that one property, not about the widget or about having two
// datasource mappings.
func TestBuild_DropdownFilterWithoutLinkedMappingBuilds(t *testing.T) {
	e := multiSourceEngine(t)
	def := &WidgetDefinition{
		WidgetID:     "com.mendix.widget.web.datagriddropdownfilter.DatagridDropdownFilter",
		MDLName:      "DROPDOWNFILTER",
		TemplateFile: "datagrid-dropdown-filter.json",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "baseType", Value: "ref", Operation: "primitive"},
			{PropertyKey: "refOptions", Source: "DataSource", Operation: "datasource"},
		},
	}
	w := &ast.WidgetV3{Name: "f", Type: "pluggablewidget", Properties: map[string]any{
		"WidgetType": "com.mendix.widget.web.datagriddropdownfilter.DatagridDropdownFilter",
		"refOptions": &ast.DataSourceV3{Type: "database", Reference: "Sales.Customer"},
	}}

	if _, err := e.Build(def, w); err != nil {
		t.Fatalf("the shipped shape must still build: %v", err)
	}
}

// Every widget package in testdata carries this fact, and the shipped
// definitions depend on it: `linkedDs` is the ONLY linked datasource among the
// multi-datasource widgets, which is why DatagridDropdownFilter is
// single-source from MDL's side while ComboBox and the charts are not.
func TestDropdownFilterLinkedDataSourceIsMarkedInTheTemplate(t *testing.T) {
	e := multiSourceEngine(t)
	wb, err := e.backend.LoadWidgetTemplate("com.mendix.widget.web.datagriddropdownfilter.DatagridDropdownFilter", "")
	if err != nil || wb == nil {
		t.Fatalf("load template: %v", err)
	}
	ids := wb.PropertyTypeIDs()
	if !ids["linkedDs"].IsLinked {
		t.Error("linkedDs must be marked IsLinked — without it the guard is inert")
	}
	if ids["refOptions"].IsLinked {
		t.Error("refOptions is the author's to set and must NOT be marked IsLinked")
	}
}

// namedDataSourceValue resolves under the schema key or any registered alias,
// case-insensitively, and ignores a value that is not a datasource — a scalar
// there is MDL-WIDGET05's business.
func TestNamedDataSourceValue(t *testing.T) {
	m := PropertyMapping{PropertyKey: "optionsSourceDatabaseDataSource", MdlAliases: []string{"DatabaseSource"}}
	ds := &ast.DataSourceV3{Type: "database", Reference: "Sales.Customer"}

	for _, spelling := range []string{
		"optionsSourceDatabaseDataSource", "optionssourcedatabasedatasource", "DatabaseSource", "databasesource",
	} {
		w := &ast.WidgetV3{Properties: map[string]any{spelling: ds}}
		if got := namedDataSourceValue(m, w); got != ds {
			t.Errorf("spelling %q did not resolve", spelling)
		}
	}
	scalar := &ast.WidgetV3{Properties: map[string]any{"optionsSourceDatabaseDataSource": "Sales.Customer"}}
	if got := namedDataSourceValue(m, scalar); got != nil {
		t.Errorf("a scalar must not resolve as a datasource, got %v", got)
	}
}

// A mode gated on a datasource must be selected by a datasource given under its
// own key, not only by the generic clause — otherwise allowing the named form
// would pick the wrong mode and drop the value anyway.
func TestSelectMappings_NamedDataSourceSelectsMode(t *testing.T) {
	def := &WidgetDefinition{
		MDLName: "COMBOBOX",
		Modes: []WidgetMode{
			{
				Name:      "association",
				Condition: "hasDataSource",
				PropertyMappings: []PropertyMapping{
					{PropertyKey: "optionsSourceAssociationDataSource", Source: "DataSource", Operation: "datasource"},
				},
			},
			{Name: "default", PropertyMappings: []PropertyMapping{{PropertyKey: "attributeEnumeration", Source: "Attribute", Operation: "attribute"}}},
		},
	}
	e := &PluggableWidgetEngine{}

	named := &ast.WidgetV3{Properties: map[string]any{
		"optionsSourceAssociationDataSource": &ast.DataSourceV3{Type: "database", Reference: "Sales.Customer"},
	}}
	mappings, _, err := e.selectMappings(def, named)
	if err != nil {
		t.Fatalf("selectMappings: %v", err)
	}
	if len(mappings) != 1 || mappings[0].PropertyKey != "optionsSourceAssociationDataSource" {
		t.Errorf("named datasource did not select the association mode, got %+v", mappings)
	}

	// An ACTION given by name must not select it. A microflow action and a
	// microflow datasource parse to the same AST shape, so a condition that
	// looked at every datasource-shaped property would flip the mode on an
	// `OnChange:`.
	action := &ast.WidgetV3{Properties: map[string]any{
		"onChangeEvent": &ast.DataSourceV3{Type: "microflow", Reference: "Sales.DoThing"},
	}}
	mappings, _, err = e.selectMappings(def, action)
	if err != nil {
		t.Fatalf("selectMappings: %v", err)
	}
	if len(mappings) != 1 || mappings[0].PropertyKey != "attributeEnumeration" {
		t.Errorf("a named action selected a datasource mode, got %+v", mappings)
	}
}

// hasDataSource:<key> names WHICH datasource selects the mode, for a widget
// whose modes are mutually exclusive by that (a ComboBox's association vs
// database). Without it the two modes are indistinguishable and mode ORDER
// decides.
func TestSelectMappings_HasDataSourceByKey(t *testing.T) {
	def := &WidgetDefinition{
		MDLName: "COMBOBOX",
		Modes: []WidgetMode{
			{
				Name:      "association",
				Condition: "hasDataSource:optionsSourceAssociationDataSource",
				PropertyMappings: []PropertyMapping{
					{PropertyKey: "optionsSourceAssociationDataSource", Source: "DataSource", Operation: "datasource"},
				},
			},
			{
				Name:      "database",
				Condition: "hasDataSource:optionsSourceDatabaseDataSource",
				PropertyMappings: []PropertyMapping{
					{PropertyKey: "optionsSourceDatabaseDataSource", Source: "DataSource", Operation: "datasource"},
				},
			},
			{Name: "default", PropertyMappings: []PropertyMapping{{PropertyKey: "attributeEnumeration", Source: "Attribute", Operation: "attribute"}}},
		},
	}
	e := &PluggableWidgetEngine{}

	// The SECOND mode is selected, though the first is listed earlier — order is
	// no longer what decides.
	w := &ast.WidgetV3{Properties: map[string]any{
		"optionsSourceDatabaseDataSource": &ast.DataSourceV3{Type: "database", Reference: "Sales.Customer"},
	}}
	mappings, _, err := e.selectMappings(def, w)
	if err != nil {
		t.Fatalf("selectMappings: %v", err)
	}
	if len(mappings) != 1 || mappings[0].PropertyKey != "optionsSourceDatabaseDataSource" {
		t.Errorf("hasDataSource:<key> selected the wrong mode, got %+v", mappings)
	}
}

// #643's guarantee, restated in terms of the VALUE rather than the key: a real
// datasource under a datasource-typed key is accepted, a scalar under the same
// key (or under one of its aliases) is still the silent drop that rule exists to
// catch.
func TestMDLWIDGET05_JudgesTheValueNotTheKey(t *testing.T) {
	reg := LoadWidgetRegistry("")
	if reg == nil {
		t.Fatal("built-in widget registry not available")
	}
	const key = "optionsSourceAssociationDataSource"

	accepted := combo(map[string]any{
		"optionsSourceType": "association",
		key:                 &ast.DataSourceV3{Type: "database", Reference: "Sales.Customer"},
	})
	for _, v := range validatePluggableWidgetProperties(accepted, reg, "page P") {
		if v.RuleID == "MDL-WIDGET05" {
			t.Errorf("a real datasource under %s must be accepted: %s", key, v.Message)
		}
	}

	rejected := combo(map[string]any{"optionsSourceType": "association", key: "Sales.Customer"})
	if _, ok := ruleIDs(validatePluggableWidgetProperties(rejected, reg, "page P"))["MDL-WIDGET05"]; !ok {
		t.Errorf("a scalar under %s must still be MDL-WIDGET05", key)
	}
}

// An alias is an authorable spelling of the same property, so a scalar under it
// is the same defect reached by the other name. Listing only the schema key let
// it through to the generic property handling, which is the silent drop.
func TestDatasourceTypedKeys_IncludesAliases(t *testing.T) {
	def := &WidgetDefinition{
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "optionsSourceDatabaseDataSource", Source: "DataSource", Operation: "datasource", MdlAliases: []string{"DatabaseSource"}},
		},
	}
	keys := datasourceTypedKeys(def)
	for _, want := range []string{"optionssourcedatabasedatasource", "databasesource"} {
		if !keys[want] {
			t.Errorf("datasourceTypedKeys missing %q — a scalar written there skips MDL-WIDGET05", want)
		}
	}
}

// MDL-WIDGET16 and MDL-WIDGET05 must agree about what counts as an options
// datasource. While WIDGET16 read only the generic clause, a ComboBox whose
// option list was given by name got both "this datasource is fine" (WIDGET05
// accepted it) and "there is no datasource" (WIDGET16) on the same widget, and
// because exec refuses a script with errors, the page could not be written at
// all.
func TestMDLWIDGET16_AcceptsANamedOptionsDataSource(t *testing.T) {
	w := &ast.WidgetV3{
		Name: "cb",
		Type: "combobox",
		Properties: map[string]any{
			"Association":                        "Order_Customer",
			"optionsSourceAssociationDataSource": &ast.DataSourceV3{Type: "database", Reference: "Sales.Customer"},
			"CaptionAttribute":                   "Name",
		},
	}
	if vs := validateComboBoxAssociation(w, "page P"); len(vs) != 0 {
		t.Errorf("named options datasource must satisfy MDL-WIDGET16, got: %s", vs[0].Message)
	}

	// The rule still fires when there is genuinely no option list.
	bare := &ast.WidgetV3{Name: "cb", Type: "combobox",
		Properties: map[string]any{"Association": "Order_Customer"}}
	if vs := validateComboBoxAssociation(bare, "page P"); len(vs) != 1 {
		t.Errorf("an association-mode combobox with no datasource must still be MDL-WIDGET16, got %d violations", len(vs))
	}
}

// renderedWidgetStrings flattens every string in the built widget's BSON, which
// is what the assertions above match against: the point is that a value reached
// the document at all, not which property path it sits under.
func renderedWidgetStrings(t *testing.T, w pages.Widget) string {
	t.Helper()
	cw, ok := w.(*pages.CustomWidget)
	if !ok {
		t.Fatalf("Build returned %T, want *pages.CustomWidget", w)
	}
	var sb strings.Builder
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			for _, e := range x {
				sb.WriteString(e.Key)
				sb.WriteByte(' ')
				walk(e.Value)
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		case string:
			sb.WriteString(x)
			sb.WriteByte('\n')
		}
	}
	walk(cw.RawObject)
	return sb.String()
}
