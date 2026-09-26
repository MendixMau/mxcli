// SPDX-License-Identifier: Apache-2.0

// A text box's and a date picker's own Forms$FormattingInfo — Studio Pro's
// "Formatting" group (Decimal precision, Group digits) on a text box and the
// "Date format" on a date picker. The writer always emitted the default
// FormattingInfo, so an authored `GroupDigits: true` or `DateFormat: Custom`
// never reached the model.
package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func TestWriteTextBox_FormattingInfo(t *testing.T) {
	d := encodeWidget(t, &pages.TextBox{
		BaseWidget: pages.BaseWidget{BaseElement: model.BaseElement{ID: "tb"}, Name: "txtAmount"},
		FormattingInfo: &pages.FormattingInfo{
			DecimalPrecision: 2, GroupDigits: true, DateFormat: "Date", EnumFormat: "Text",
		},
	})
	fi := bsonnav.DGetDoc(d, "FormattingInfo")
	if fi == nil {
		t.Fatal("no FormattingInfo written")
	}
	if got := bsonnav.DGet(fi, "GroupDigits"); got != true {
		t.Errorf("GroupDigits = %v, want true", got)
	}
}

func TestWriteDatePicker_FormattingInfo(t *testing.T) {
	d := encodeWidget(t, &pages.DatePicker{
		BaseWidget: pages.BaseWidget{BaseElement: model.BaseElement{ID: "dp"}, Name: "dpFrom"},
		FormattingInfo: &pages.FormattingInfo{
			DecimalPrecision: 2, DateFormat: "Custom", CustomDateFormat: "d MMM yyyy", EnumFormat: "Text",
		},
	})
	fi := bsonnav.DGetDoc(d, "FormattingInfo")
	if fi == nil {
		t.Fatal("no FormattingInfo written")
	}
	if got := bsonnav.DGetString(fi, "DateFormat"); got != "Custom" {
		t.Errorf("DateFormat = %q, want Custom", got)
	}
	if got := bsonnav.DGetString(fi, "CustomDateFormat"); got != "d MMM yyyy" {
		t.Errorf("CustomDateFormat = %q, want the authored pattern", got)
	}
}

// Control: unformatted widgets keep the default Studio Pro stores on 200 of 200
// text boxes in a stock project (DecimalPrecision 2, GroupDigits false, Date).
func TestWriteTextBox_NoFormattingKeepsDefaults(t *testing.T) {
	d := encodeWidget(t, &pages.TextBox{
		BaseWidget: pages.BaseWidget{BaseElement: model.BaseElement{ID: "tb"}, Name: "txtPlain"},
	})
	fi := bsonnav.DGetDoc(d, "FormattingInfo")
	if fi == nil {
		t.Fatal("no FormattingInfo written")
	}
	if got := bsonnav.DGet(fi, "GroupDigits"); got != false {
		t.Errorf("GroupDigits = %v, want false", got)
	}
	if got := bsonnav.DGetString(fi, "DateFormat"); got != "Date" {
		t.Errorf("DateFormat = %q, want Date", got)
	}
}
