// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Button property names for the confirmation dialog of a call-microflow or
// call-nanoflow action (Forms$ConfirmationInfo).
const (
	propConfirmation   = "Confirmation"
	propConfirmProceed = "ConfirmProceed"
	propConfirmCancel  = "ConfirmCancel"
)

// confirmationPropsProblem reports why a widget's Confirmation / ConfirmProceed /
// ConfirmCancel properties cannot be written, or "" when they can (or are
// absent). It is shared by `mxcli check` (MDL-WIDGET32) and the page builder so
// the two cannot disagree.
//
// Mendix stores the dialog on the action — MicroflowSettings.ConfirmationInfo
// for a microflow call, ConfirmationInfo directly on a nanoflow call — and no
// other action or widget has one. So the properties are refused, never dropped,
// where nothing can carry them, and the captions are refused without a
// question: an unused caption is a script that says more than the model holds.
func confirmationPropsProblem(w *ast.WidgetV3) string {
	question := w.GetStringProp(propConfirmation)
	proceed := w.GetStringProp(propConfirmProceed)
	cancel := w.GetStringProp(propConfirmCancel)
	if question == "" && proceed == "" && cancel == "" {
		return ""
	}
	if !isButtonKeyword(w.Type) {
		return fmt.Sprintf("widget %q (%s): Confirmation/ConfirmProceed/ConfirmCancel apply to actionbutton and linkbutton only", w.Name, w.Type)
	}
	if question == "" {
		return fmt.Sprintf("button %q: ConfirmProceed/ConfirmCancel need a Confirmation question — they are the captions of the confirmation dialog", w.Name)
	}
	action := w.GetAction()
	if action == nil || (action.Type != "microflow" && action.Type != "nanoflow") {
		return fmt.Sprintf("button %q: Confirmation requires Action: MICROFLOW or NANOFLOW — Mendix stores the confirmation dialog on a microflow or nanoflow call only", w.Name)
	}
	return ""
}

// applyConfirmationV3 attaches a button's confirmation dialog to its action,
// or refuses the properties (see confirmationPropsProblem).
func (pb *pageBuilder) applyConfirmationV3(w *ast.WidgetV3, action pages.ClientAction) error {
	if problem := confirmationPropsProblem(w); problem != "" {
		return mdlerrors.NewValidation(problem)
	}
	question := w.GetStringProp(propConfirmation)
	if question == "" {
		return nil
	}
	ci := pb.newConfirmationInfo(question, w.GetStringProp(propConfirmProceed), w.GetStringProp(propConfirmCancel))
	switch a := action.(type) {
	case *pages.MicroflowClientAction:
		a.Confirmation = ci
	case *pages.NanoflowClientAction:
		a.Confirmation = ci
	default:
		return mdlerrors.NewValidationf("button %q: Confirmation requires Action: MICROFLOW or NANOFLOW, got %T", w.Name, action)
	}
	return nil
}

// newConfirmationInfo builds the semantic dialog; empty captions become the
// defaults Studio Pro stores ("Proceed", "Cancel").
func (pb *pageBuilder) newConfirmationInfo(question, proceed, cancel string) *pages.ConfirmationInfo {
	if proceed == "" {
		proceed = pages.DefaultConfirmProceedCaption
	}
	if cancel == "" {
		cancel = pages.DefaultConfirmCancelCaption
	}
	lang := pb.textLang()
	text := func(s string) *model.Text {
		return &model.Text{
			BaseElement:  model.BaseElement{ID: model.ID(types.GenerateID()), TypeName: "Texts$Text"},
			Translations: map[string]string{lang: s},
		}
	}
	return &pages.ConfirmationInfo{
		BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID()), TypeName: "Forms$ConfirmationInfo"},
		Question:       text(question),
		ProceedCaption: text(proceed),
		CancelCaption:  text(cancel),
	}
}
