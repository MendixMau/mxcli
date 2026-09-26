// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// inputFormattingWidgets are the built-in widgets that carry their own
// Forms$FormattingInfo, so the dynamic-text format keys (decimalPrecision,
// groupDigits, dateFormat, customDateFormat) mean something on them: Studio
// Pro's "Formatting" group on a text box and "Date format" on a date picker.
// On any other built-in widget the keys have nowhere to go.
var inputFormattingWidgets = map[string]bool{"TEXTBOX": true, "DATEPICKER": true}

// canonicalDateFormats maps the accepted spellings onto the stored
// Forms$FormattingInfo.DateFormat enum values.
var canonicalDateFormats = map[string]string{
	"date": "Date", "datetime": "DateTime", "time": "Time", "custom": "Custom",
}

// inputFormattingFromWidget reads a TEXTBOX / DATEPICKER's format keys into a
// FormattingInfo. It returns nil when the widget names none of them, so the
// writer keeps the default Studio Pro stores on an unformatted widget
// (DecimalPrecision 2, GroupDigits false, DateFormat Date).
//
// Before, these keys passed check silently — they are the dynamic-text format
// keys, which MDL-WIDGET07 exempts — and the writer always emitted the default,
// so `GroupDigits: true` on a text box and `DateFormat: Custom` on a date
// picker never reached the model.
func inputFormattingFromWidget(w *ast.WidgetV3) (*pages.FormattingInfo, error) {
	fi := &pages.FormattingInfo{DecimalPrecision: 2, DateFormat: "Date", EnumFormat: "Text"}
	authored := false
	if raw, ok := lookupPropCI(w, "DecimalPrecision"); ok {
		n, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(raw)))
		if err != nil || n < 0 {
			return nil, mdlerrors.NewValidationf("%s %s: DecimalPrecision must be a non-negative integer, got %v", strings.ToLower(w.Type), w.Name, raw)
		}
		fi.DecimalPrecision, authored = n, true
	}
	if raw, ok := lookupPropCI(w, "GroupDigits"); ok {
		v, err := propBool(raw)
		if err != nil {
			return nil, mdlerrors.NewValidationf("%s %s: GroupDigits: %v", strings.ToLower(w.Type), w.Name, err)
		}
		fi.GroupDigits, authored = v, true
	}
	if raw, ok := lookupPropCI(w, "CustomDateFormat"); ok {
		fi.CustomDateFormat, authored = fmt.Sprint(raw), true
		// A pattern has no meaning under any other format.
		fi.DateFormat = "Custom"
	}
	if raw, ok := lookupPropCI(w, "DateFormat"); ok {
		v, known := canonicalDateFormats[strings.ToLower(strings.TrimSpace(fmt.Sprint(raw)))]
		if !known {
			return nil, mdlerrors.NewValidationf("%s %s: DateFormat must be one of Date, DateTime, Time, Custom, got %v", strings.ToLower(w.Type), w.Name, raw)
		}
		fi.DateFormat, authored = v, true
	}
	if !authored {
		return nil, nil
	}
	if fi.DateFormat == "Custom" && fi.CustomDateFormat == "" {
		return nil, mdlerrors.NewValidationf("%s %s: DateFormat: Custom needs a CustomDateFormat pattern, e.g. CustomDateFormat: 'd MMM yyyy'", strings.ToLower(w.Type), w.Name)
	}
	fi.BaseElement = model.BaseElement{ID: model.ID(types.GenerateID()), TypeName: "Forms$FormattingInfo"}
	return fi, nil
}

// extractInputFormatting renders a text box's / date picker's own
// Forms$FormattingInfo as the MDL properties that differ from the default Studio
// Pro stores on an unformatted widget (DecimalPrecision 2, GroupDigits false,
// DateFormat Date, no pattern). Only deviations are emitted, so an unformatted
// widget describes exactly as before. A CustomDateFormat is emitted whenever one
// is stored — Studio Pro keeps the pattern after the format is switched away
// from Custom, and the builder keeps it too, so the round trip is exact.
func extractInputFormatting(w map[string]any) []string {
	fi, ok := w["FormattingInfo"].(map[string]any)
	if !ok || fi == nil {
		return nil
	}
	var props []string
	if raw, ok := fi["DecimalPrecision"]; ok && toInt(raw) != 2 {
		n := toInt(raw)
		props = append(props, fmt.Sprintf("DecimalPrecision: %d", n))
	}
	if g, _ := fi["GroupDigits"].(bool); g {
		props = append(props, "GroupDigits: true")
	}
	if df, _ := fi["DateFormat"].(string); df != "" && df != "Date" {
		props = append(props, "DateFormat: "+df)
	}
	if c, _ := fi["CustomDateFormat"].(string); c != "" {
		props = append(props, "CustomDateFormat: "+mdlQuote(c))
	}
	return props
}
