// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#489: DESCRIBE PAGE drops a DataGrid2 column's filter widget when the
// column also carries custom content.
//
// A column's `content` and `filter` are two independent Widgets-typed slots, and
// the write path fills both (measured on 11.6.6: showContentAs=customContent,
// content=[cbActive], filter=[ddfActive], mx check 0 errors, both working in the
// browser). The reader took the FIRST widgets-typed property it met instead of
// keying on the resolved property key, and the writer emits column properties
// alphabetically — so `content` won and the filter was dropped. A filter-only
// column survived by accident: with `content` empty the filter landed in the
// content list and was re-emitted in the column body, where the builder routes
// it back to the filter slot by widget type.
//
// The round trip is what makes this more than cosmetic: describing such a page
// and re-executing the output deletes the filter, with mxcli check, exec and
// mx check clean at both ends.
package executor

import (
	"bytes"
	"strings"
	"testing"
)

// buildDataGridWithContentAndFilterColumn mirrors the shape Mendix stores for a
// DataGrid2 whose single column holds BOTH a custom-content widget and a filter
// widget. Property keys resolve through the widget's own PropertyTypes, keyed by
// the WidgetProperty's TypePointer (the PropertyType $ID — the inner WidgetValue
// points at the ValueType $ID instead, which is a different node).
func buildDataGridWithContentAndFilterColumn() map[string]any {
	const (
		idColumns       = "type-id-columns"
		idHeader        = "type-id-header"
		idShowContentAs = "type-id-showcontentas"
		idContent       = "type-id-content"
		idFilter        = "type-id-filter"
	)

	colProp := func(typePointer string, value map[string]any) map[string]any {
		return map[string]any{"TypePointer": typePointer, "Value": value}
	}

	return map[string]any{
		"Name": "dgTest",
		"Type": map[string]any{
			"WidgetId": "com.mendix.widget.web.datagrid.Datagrid",
			"ObjectType": map[string]any{
				"PropertyTypes": []any{
					map[string]any{
						"$ID": idColumns, "PropertyKey": "columns",
						"ValueType": map[string]any{
							"ObjectType": map[string]any{
								"PropertyTypes": []any{
									map[string]any{"$ID": idHeader, "PropertyKey": "header",
										"ValueType": map[string]any{"Type": "TextTemplate"}},
									map[string]any{"$ID": idShowContentAs, "PropertyKey": "showContentAs",
										"ValueType": map[string]any{"Type": "Enumeration"}},
									map[string]any{"$ID": idContent, "PropertyKey": "content",
										"ValueType": map[string]any{"Type": "Widgets"}},
									map[string]any{"$ID": idFilter, "PropertyKey": "filter",
										"ValueType": map[string]any{"Type": "Widgets"}},
								},
							},
						},
					},
				},
			},
		},
		"Object": map[string]any{
			"Properties": []any{
				map[string]any{
					"TypePointer": idColumns,
					"Value": map[string]any{
						"Objects": []any{
							map[string]any{
								// Alphabetical, as the writer emits them: content before filter.
								"Properties": []any{
									colProp(idContent, map[string]any{
										"Widgets": []any{
											map[string]any{
												"$Type": "Forms$CheckBox",
												"Name":  "cbActive",
											},
										},
									}),
									colProp(idFilter, map[string]any{
										"Widgets": []any{
											map[string]any{
												"$Type": "CustomWidgets$CustomWidget",
												"Name":  "ddfActive",
												"Type": map[string]any{
													"WidgetId": "com.mendix.widget.web.datagriddropdownfilter.DatagridDropdownFilter",
												},
											},
										},
									}),
									colProp(idHeader, map[string]any{
										"TextTemplate": map[string]any{
											"Template": map[string]any{
												"Items": []any{
													map[string]any{"Text": "Active"},
												},
											},
										},
									}),
									colProp(idShowContentAs, map[string]any{
										"PrimitiveValue": "customContent",
									}),
								},
							},
						},
					},
				},
			},
		},
	}
}

// The read half: both slots must survive extraction, in their own lists.
func TestDataGrid2Column_KeepsContentAndFilterWidgets(t *testing.T) {
	cols := extractDataGrid2Columns(nil, buildDataGridWithContentAndFilterColumn())
	if len(cols) != 1 {
		t.Fatalf("expected 1 column, got %d", len(cols))
	}
	col := cols[0]
	if len(col.ContentWidgets) != 1 || col.ContentWidgets[0].Name != "cbActive" {
		t.Errorf("content widgets = %+v, want one widget named cbActive", col.ContentWidgets)
	}
	if len(col.FilterWidgets) != 1 || col.FilterWidgets[0].Name != "ddfActive" {
		t.Fatalf("filter widgets = %+v, want one widget named ddfActive — the column's "+
			"filter was dropped, so describe→exec deletes it (ako/mxcli#489)", col.FilterWidgets)
	}
}

// A filter-only column keeps working: its filter belongs in FilterWidgets now
// rather than riding along in the content list, and DESCRIBE must still emit it.
func TestDataGrid2Column_FilterOnlyColumnStillRoundTrips(t *testing.T) {
	w := buildDataGridWithContentAndFilterColumn()
	// Drop the content property, leaving filter + header + showContentAs.
	cols := w["Object"].(map[string]any)["Properties"].([]any)[0].(map[string]any)
	objects := cols["Value"].(map[string]any)["Objects"].([]any)
	colProps := objects[0].(map[string]any)["Properties"].([]any)
	objects[0].(map[string]any)["Properties"] = colProps[1:]

	got := extractDataGrid2Columns(nil, w)
	if len(got) != 1 {
		t.Fatalf("expected 1 column, got %d", len(got))
	}
	if len(got[0].ContentWidgets) != 0 {
		t.Errorf("content widgets = %+v, want none", got[0].ContentWidgets)
	}
	if len(got[0].FilterWidgets) != 1 || got[0].FilterWidgets[0].Name != "ddfActive" {
		t.Errorf("filter widgets = %+v, want one widget named ddfActive", got[0].FilterWidgets)
	}
}

// The emit half: both widgets must appear in the column body. Testing through
// outputWidgetMDLV3 rather than the column helper means removing the emit change
// fails this test — asserting on the helper alone would prove the helper works
// and nothing about the wiring.
func TestDataGrid2Column_EmitsContentAndFilterWidgets(t *testing.T) {
	cols := extractDataGrid2Columns(nil, buildDataGridWithContentAndFilterColumn())
	if len(cols) == 0 {
		t.Fatal("fixture produced no columns")
	}

	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}
	outputWidgetMDLV3(ctx, rawWidget{
		Type:            "CustomWidgets$CustomWidget",
		RenderMode:      "datagrid2",
		Name:            "dgTest",
		WidgetID:        "com.mendix.widget.web.datagrid.Datagrid",
		DataGridColumns: cols,
	}, 0)

	out := buf.String()
	if !strings.Contains(out, "cbActive") {
		t.Errorf("custom-content widget missing from DESCRIBE output:\n%s", out)
	}
	if !strings.Contains(out, "ddfActive") {
		t.Errorf("filter widget missing from DESCRIBE output — re-executing this output "+
			"deletes the filter (ako/mxcli#489):\n%s", out)
	}
	// A body, not a bare column line, or the output cannot re-parse.
	if !strings.Contains(out, "column Active") || !strings.Contains(out, "{") {
		t.Errorf("column should be emitted with a body:\n%s", out)
	}
}
