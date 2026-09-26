// SPDX-License-Identifier: Apache-2.0

// Studio Pro's input-widget Validation (None / Required / Custom — "Required"
// is a preset that stores the expression `$value != empty`) lives in the same
// Forms$WidgetValidation on a text area and a date picker as on a text box.
// Only the text box writer carried it (ako/mxcli#550); the other two always
// wrote the empty element, so `Validation:` there was dropped with no warning.
package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func TestWriteDatePickerAndTextArea_Validation(t *testing.T) {
	for name, w := range map[string]pages.Widget{
		"datepicker": &pages.DatePicker{
			BaseWidget:           pages.BaseWidget{BaseElement: model.BaseElement{ID: "dp"}, Name: "dpFrom"},
			ValidationExpression: "$value != empty", ValidationMessage: "Pick a date.",
		},
		"textarea": &pages.TextArea{
			BaseWidget:           pages.BaseWidget{BaseElement: model.BaseElement{ID: "ta"}, Name: "taNote"},
			ValidationExpression: "$value != empty", ValidationMessage: "Enter a note.",
		},
	} {
		d := encodeWidget(t, w)
		v := bsonnav.DGetDoc(d, "Validation")
		if v == nil {
			t.Fatalf("%s: no Forms$WidgetValidation", name)
		}
		if got := bsonnav.DGetString(v, "Expression"); got != "$value != empty" {
			t.Errorf("%s: Validation.Expression = %q, want the authored expression", name, got)
		}
	}
}
