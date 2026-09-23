// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// extractButtonConfirmation reads the confirmation dialog of a button's action.
//
// Mendix keeps it on the action, not the button: under
// MicroflowSettings.ConfirmationInfo for Forms$MicroflowAction, and directly
// under ConfirmationInfo for Forms$CallNanoflowClientAction. The captions are
// returned empty when they equal the defaults Studio Pro stores ("Proceed",
// "Cancel"), so describe → exec → describe is stable: the writer fills the
// defaults back in. A dialog with an empty question is reported as absent —
// Mendix shows no dialog for it and the builder treats an empty question the
// same way.
func extractButtonConfirmation(ctx *ExecContext, w map[string]any) (question, proceed, cancel string) {
	action := actionMapForKey(w, "Action")
	if action == nil {
		return "", "", ""
	}
	var info map[string]any
	switch action["$Type"] {
	case "Forms$MicroflowAction", "Pages$MicroflowAction":
		info = asBsonMap(asBsonMap(action["MicroflowSettings"])["ConfirmationInfo"])
	case "Forms$CallNanoflowClientAction", "Pages$CallNanoflowClientAction":
		info = asBsonMap(action["ConfirmationInfo"])
	}
	if info == nil {
		return "", "", ""
	}
	lang := describeDefaultLanguage(ctx)
	text := func(key string) string {
		t := asBsonMap(info[key])
		if t == nil {
			return ""
		}
		return selectTranslationText(getBsonArrayElements(t["Items"]), lang)
	}
	question = text("Question")
	if question == "" {
		return "", "", ""
	}
	proceed = text("ProceedButtonCaption")
	if proceed == pages.DefaultConfirmProceedCaption {
		proceed = ""
	}
	cancel = text("CancelButtonCaption")
	if cancel == pages.DefaultConfirmCancelCaption {
		cancel = ""
	}
	return question, proceed, cancel
}

// appendConfirmationProps emits Confirmation / ConfirmProceed / ConfirmCancel.
func appendConfirmationProps(props []string, w rawWidget) []string {
	if w.Confirmation == "" {
		return props
	}
	props = append(props, fmt.Sprintf("Confirmation: %s", mdlQuote(w.Confirmation)))
	if w.ConfirmProceed != "" {
		props = append(props, fmt.Sprintf("ConfirmProceed: %s", mdlQuote(w.ConfirmProceed)))
	}
	if w.ConfirmCancel != "" {
		props = append(props, fmt.Sprintf("ConfirmCancel: %s", mdlQuote(w.ConfirmCancel)))
	}
	return props
}

// asBsonMap unwraps a BSON sub-document stored as map[string]any or primitive.M.
func asBsonMap(v any) map[string]any {
	switch m := v.(type) {
	case map[string]any:
		return m
	case primitive.M:
		return map[string]any(m)
	case primitive.D:
		return m.Map()
	}
	return nil
}
