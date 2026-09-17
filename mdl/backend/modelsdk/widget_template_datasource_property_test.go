// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"reflect"
	"sort"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// pages.PropertyTypeIDEntry must BE mdl/types.PropertyTypeIDEntry, not a second
// struct of the same shape.
//
// While they were two types, every hop between them was a hand-written field
// copy, and a hand-written copy can only lose fields. That is exactly what
// happened to DataSourceProperty: the template loader read it, both copies
// (convertPropTypeIDs here, and the inline one in BuildFilterWidget) omitted it,
// and nothing failed — the widget engine simply never learned which datasource a
// dependent property binds against.
//
// reflect.TypeOf rather than a compile-time assignment: `var _ pages.X =
// types.X{}` still compiles for two identical-but-distinct structs, so it would
// not catch the re-declaration this guards against.
func TestPropertyTypeIDEntryIsCanonicalType(t *testing.T) {
	got := reflect.TypeOf(pages.PropertyTypeIDEntry{})
	want := reflect.TypeOf(types.PropertyTypeIDEntry{})
	if got != want {
		t.Fatalf("pages.PropertyTypeIDEntry is %s, not an alias of %s — "+
			"re-declaring it re-introduces the field-copy that dropped DataSourceProperty", got, want)
	}
	if got := reflect.TypeOf(pages.PropertyTranslation{}); got != reflect.TypeOf(types.PropertyTranslation{}) {
		t.Fatalf("pages.PropertyTranslation is %s, not an alias of the mdl/types type", got)
	}
}

// A widget's template states, per property, which of its datasources that
// property binds against (widget.xml's `dataSource="…"`, stored as the
// ValueType's DataSourceProperty). For a multi-datasource widget that is the
// only authoritative statement of the link — mapping ORDER in a .def.json is a
// reconstruction of it, and a wrong one the moment a definition is re-generated.
//
// This asserts the link survives the whole path the widget engine uses:
// GetTemplateFullBSON -> LoadWidgetTemplate -> WidgetObjectBuilder.PropertyTypeIDs().
// Before the entry type was unified it did not: every DataSourceProperty here
// arrived empty, which is the control for this test.
func TestLoadWidgetTemplatePreservesDataSourceProperty(t *testing.T) {
	tests := []struct {
		name        string
		widgetID    string
		dataSources []string          // DataSource-typed keys, sorted
		links       map[string]string // dependent property -> its datasource
	}{
		{
			// Two datasources populated AT THE SAME TIME: the grid's own list
			// and the association target's option list. This is the shape a
			// single generic `datasource:` clause cannot express.
			name:        "DatagridDropdownFilter",
			widgetID:    "com.mendix.widget.web.datagriddropdownfilter.DatagridDropdownFilter",
			dataSources: []string{"linkedDs", "refOptions"},
			links: map[string]string{
				"attr":          "linkedDs",
				"refEntity":     "linkedDs",
				"refCaption":    "refOptions",
				"refCaptionExp": "refOptions",
				"refSearchAttr": "refOptions",
			},
		},
		{
			// Two datasources that are mutually exclusive by mode
			// (optionsSourceType). Each mode's dependents name their own.
			name:        "Combobox",
			widgetID:    "com.mendix.widget.web.combobox.Combobox",
			dataSources: []string{"optionsSourceAssociationDataSource", "optionsSourceDatabaseDataSource"},
			links: map[string]string{
				"optionsSourceAssociationCaptionAttribute": "optionsSourceAssociationDataSource",
				"optionsSourceDatabaseCaptionAttribute":    "optionsSourceDatabaseDataSource",
				"optionsSourceDatabaseValueAttribute":      "optionsSourceDatabaseDataSource",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := &Backend{}
			wb, err := b.LoadWidgetTemplate(tc.widgetID, "")
			if err != nil {
				t.Fatalf("LoadWidgetTemplate(%s): %v", tc.widgetID, err)
			}
			if wb == nil {
				t.Fatalf("no embedded template for %s", tc.widgetID)
			}
			ids := wb.PropertyTypeIDs()

			var gotSources []string
			for key, entry := range ids {
				if entry.ValueType == "DataSource" {
					gotSources = append(gotSources, key)
				}
			}
			sort.Strings(gotSources)
			if !reflect.DeepEqual(gotSources, tc.dataSources) {
				t.Errorf("DataSource-typed keys = %v, want %v", gotSources, tc.dataSources)
			}

			for prop, wantSource := range tc.links {
				entry, ok := ids[prop]
				if !ok {
					t.Errorf("template has no property %q", prop)
					continue
				}
				if entry.DataSourceProperty != wantSource {
					t.Errorf("%s.DataSourceProperty = %q, want %q",
						prop, entry.DataSourceProperty, wantSource)
				}
			}

			// A datasource does not itself bind to one.
			for _, key := range tc.dataSources {
				if got := ids[key].DataSourceProperty; got != "" {
					t.Errorf("%s.DataSourceProperty = %q, want empty", key, got)
				}
			}
		})
	}
}
