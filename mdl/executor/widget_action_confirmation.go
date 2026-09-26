// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// applyActionConfirmation reads a widget's `Confirmation:` (and the optional
// `ConfirmationProceed:` / `ConfirmationCancel:` captions) onto its
// call-microflow or call-nanoflow action — Studio Pro's "Ask confirmation",
// stored as Forms$ConfirmationInfo. Captions default to Studio Pro's own
// "Proceed" / "Cancel".
//
// Mendix has confirmation on those two actions only: Delete, Save, Close, Show
// page and the rest carry no ConfirmationInfo in the model. So the property is
// refused on any other action, naming the remedy — call a microflow that does
// the work — rather than accepted and dropped, which is what MDL-WIDGET07 used
// to report it as.
func applyActionConfirmation(w *ast.WidgetV3, act pages.ClientAction) error {
	q, hasQ := lookupPropCI(w, "Confirmation")
	proceed, hasP := lookupPropCI(w, "ConfirmationProceed")
	cancel, hasC := lookupPropCI(w, "ConfirmationCancel")
	if !hasQ {
		if hasP || hasC {
			return mdlerrors.NewValidationf("widget %s: ConfirmationProceed / ConfirmationCancel need a Confirmation: 'question'", w.Name)
		}
		return nil
	}
	question := fmt.Sprint(q)
	if question == "" {
		return mdlerrors.NewValidationf("widget %s: Confirmation needs the question text, e.g. Confirmation: 'Delete this record?'", w.Name)
	}
	ci := &pages.ConfirmationInfo{Question: question, ProceedCaption: "Proceed", CancelCaption: "Cancel"}
	if hasP {
		ci.ProceedCaption = fmt.Sprint(proceed)
	}
	if hasC {
		ci.CancelCaption = fmt.Sprint(cancel)
	}
	switch a := act.(type) {
	case *pages.MicroflowClientAction:
		a.Confirmation = ci
	case *pages.NanoflowClientAction:
		a.Confirmation = ci
	case nil:
		return mdlerrors.NewValidationf("widget %s: Confirmation needs an action to confirm — Action: microflow Module.Flow(…)", w.Name)
	default:
		return mdlerrors.NewValidationf(
			"widget %s: Confirmation is only available on a call-microflow or call-nanoflow action (Mendix stores no confirmation on %T) — "+
				"call a microflow that performs the action, e.g. Action: microflow Module.ACT_Delete(Object: $currentObject), Confirmation: '…'",
			w.Name, act)
	}
	return nil
}

// extractActionConfirmation reads a client action's Forms$ConfirmationInfo —
// inside MicroflowSettings for a call-microflow action, on the action itself for
// a call-nanoflow one — as the MDL properties applyActionConfirmation reads.
// Captions equal to Studio Pro's defaults are left out.
func extractActionConfirmation(ctx *ExecContext, action map[string]any) []string {
	if action == nil {
		return nil
	}
	ci := asActionMap(action["ConfirmationInfo"])
	if ci == nil {
		if settings := asActionMap(action["MicroflowSettings"]); settings != nil {
			ci = asActionMap(settings["ConfirmationInfo"])
		}
	}
	if ci == nil {
		return nil
	}
	text := func(k string) string {
		if m := asActionMap(ci[k]); m != nil {
			return extractTextFromTemplate(ctx, m)
		}
		return ""
	}
	q := text("Question")
	if q == "" {
		return nil
	}
	props := []string{"Confirmation: " + mdlQuote(q)}
	if p := text("ProceedButtonCaption"); p != "" && p != "Proceed" {
		props = append(props, "ConfirmationProceed: "+mdlQuote(p))
	}
	if c := text("CancelButtonCaption"); c != "" && c != "Cancel" {
		props = append(props, "ConfirmationCancel: "+mdlQuote(c))
	}
	return props
}

// validateActionConfirmation (MDL-WIDGET35) reports at check time what the
// builder refuses at exec: a `Confirmation:` on a widget whose action is not a
// call-microflow / call-nanoflow action, the only two Mendix can confirm.
func validateActionConfirmation(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil {
		return nil
	}
	if _, ok := lookupPropCI(w, "Confirmation"); !ok {
		return nil
	}
	act := w.GetAction()
	if act != nil && (act.Type == "microflow" || act.Type == "nanoflow") {
		return nil
	}
	what := "no action"
	if act != nil {
		what = "a " + act.Type + " action"
	}
	return []linter.Violation{{
		RuleID:   "MDL-WIDGET35",
		Severity: linter.SeverityError,
		Message: fmt.Sprintf(
			"%s: widget `%s` has `Confirmation:` on %s — Mendix asks confirmation only before a call-microflow or call-nanoflow action; call a microflow that performs the action (e.g. `Action: microflow Module.ACT_Delete(Object: $currentObject)`) and confirm that",
			locationPrefix, w.Name, what),
	}}
}
