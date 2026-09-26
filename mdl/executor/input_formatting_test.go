// SPDX-License-Identifier: Apache-2.0

// `GroupDigits:` / `DecimalPrecision:` on a TEXTBOX and `DateFormat:` /
// `CustomDateFormat:` on a DATEPICKER were accepted by check with no diagnostic —
// the keys are the dynamic-text format keys, which MDL-WIDGET07 exempts — and
// dropped on write. The builder now reads them into the widget's FormattingInfo.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestInputFormattingFromWidget(t *testing.T) {
	w := &ast.WidgetV3{Type: "TEXTBOX", Name: "t", Properties: map[string]any{
		"GroupDigits": true, "DecimalPrecision": 0,
	}}
	fi, err := inputFormattingFromWidget(w)
	if err != nil || fi == nil {
		t.Fatalf("fi=%v err=%v", fi, err)
	}
	if !fi.GroupDigits || fi.DecimalPrecision != 0 || fi.DateFormat != "Date" || fi.EnumFormat != "Text" {
		t.Errorf("got %+v", fi)
	}

	w = &ast.WidgetV3{Type: "DATEPICKER", Name: "d", Properties: map[string]any{
		"DateFormat": "custom", "CustomDateFormat": "d MMM yyyy",
	}}
	fi, err = inputFormattingFromWidget(w)
	if err != nil || fi == nil {
		t.Fatalf("fi=%v err=%v", fi, err)
	}
	if fi.DateFormat != "Custom" || fi.CustomDateFormat != "d MMM yyyy" || fi.DecimalPrecision != 2 {
		t.Errorf("got %+v", fi)
	}

	// A pattern alone selects the Custom format — it has no other meaning.
	w = &ast.WidgetV3{Type: "DATEPICKER", Name: "d", Properties: map[string]any{"CustomDateFormat": "yyyy"}}
	if fi, _ = inputFormattingFromWidget(w); fi == nil || fi.DateFormat != "Custom" {
		t.Errorf("CustomDateFormat alone: got %+v, want DateFormat Custom", fi)
	}

	// Control: nothing authored, nothing built — the writer keeps its default.
	w = &ast.WidgetV3{Type: "TEXTBOX", Name: "t", Properties: map[string]any{}}
	if fi, _ = inputFormattingFromWidget(w); fi != nil {
		t.Errorf("no format keys: got %+v, want nil", fi)
	}

	for _, bad := range []map[string]any{
		{"DateFormat": "Weekly"},
		{"DecimalPrecision": "two"},
		{"GroupDigits": "maybe"},
		{"DateFormat": "Custom"}, // Custom with no pattern
	} {
		if _, err := inputFormattingFromWidget(&ast.WidgetV3{Type: "TEXTBOX", Name: "t", Properties: bad}); err == nil {
			t.Errorf("%v: accepted, want an error", bad)
		}
	}
}

// A format key on a built-in widget that has no FormattingInfo is dropped on
// write, so MDL-WIDGET07 must say so; on TEXTBOX / DATEPICKER it is consumed.
func TestFormatKeysOnInputWidgets_WIDGET07(t *testing.T) {
	onBox := validateStaticWidgetUnknownProps(&ast.WidgetV3{Type: "TEXTBOX", Name: "t",
		Properties: map[string]any{"GroupDigits": true}}, "page P")
	if len(onBox) != 0 {
		t.Errorf("TEXTBOX GroupDigits: got %v, want no warning", onBox)
	}
	onPicker := validateStaticWidgetUnknownProps(&ast.WidgetV3{Type: "DATEPICKER", Name: "d",
		Properties: map[string]any{"DateFormat": "DateTime"}}, "page P")
	if len(onPicker) != 0 {
		t.Errorf("DATEPICKER DateFormat: got %v, want no warning", onPicker)
	}
	onArea := validateStaticWidgetUnknownProps(&ast.WidgetV3{Type: "TEXTAREA", Name: "a",
		Properties: map[string]any{"GroupDigits": true}}, "page P")
	if len(onArea) != 1 {
		t.Errorf("TEXTAREA GroupDigits: got %v, want one MDL-WIDGET07 (a text area has no FormattingInfo)", onArea)
	}
}
