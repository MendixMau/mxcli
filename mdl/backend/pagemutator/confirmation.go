// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// setWidgetConfirmationMut edits the confirmation dialog of a button's action:
// `set Confirmation = '…' on btn` and its two captions.
//
// The dialog (Forms$ConfirmationInfo) belongs to the action, not the button —
// under MicroflowSettings.ConfirmationInfo on a Forms$MicroflowAction, directly
// under ConfirmationInfo on a Forms$CallNanoflowClientAction — and those are
// the only client actions with the slot. Anything else is refused: writing the
// key where the type has none builds clean and does not open in Studio Pro.
//
//   - confirmation, non-empty: sets the question, creating the dialog (with the
//     "Proceed"/"Cancel" captions Studio Pro stores) when there is none.
//   - confirmation, empty: removes the dialog (the slot goes back to null).
//   - confirmproceed / confirmcancel: edit a caption of an existing dialog; with
//     no dialog they are refused, since a caption alone is not a dialog.
//
// Editing a text keeps its other translations and replaces only the one in the
// authoring language, appending that one when it is missing.
func setWidgetConfirmationMut(widget bson.D, prop string, value any) error {
	text, ok := value.(string)
	if !ok {
		return mdlerrors.NewValidationf("%s value must be a string", prop)
	}
	holder, err := confirmationHolder(widget)
	if err != nil {
		return err
	}
	info := bsonnav.DGetDoc(holder, "ConfirmationInfo")

	switch prop {
	case "confirmation":
		if text == "" {
			bsonnav.DSet(holder, "ConfirmationInfo", nil)
			return nil
		}
		if info == nil {
			if !bsonnav.DSet(holder, "ConfirmationInfo", newConfirmationInfoDoc(text)) {
				return fmt.Errorf("action has no ConfirmationInfo slot")
			}
			return nil
		}
		return setConfirmationText(info, "Question", text)
	case "confirmproceed", "confirmcancel":
		if info == nil {
			return mdlerrors.NewValidationf(
				"%s edits the confirmation dialog, and this button's action has none — set Confirmation first", prop)
		}
		key := "ProceedButtonCaption"
		def := pages.DefaultConfirmProceedCaption
		if prop == "confirmcancel" {
			key, def = "CancelButtonCaption", pages.DefaultConfirmCancelCaption
		}
		if text == "" {
			text = def
		}
		return setConfirmationText(info, key, text)
	}
	return fmt.Errorf("unknown confirmation property %q", prop)
}

// confirmationHolder returns the document that carries the ConfirmationInfo
// slot for the widget's action.
func confirmationHolder(widget bson.D) (bson.D, error) {
	action := bsonnav.DGetDoc(widget, "Action")
	if action == nil {
		return nil, mdlerrors.NewValidationf(
			"widget %q (%s) has no action — Confirmation applies to a button calling a microflow or nanoflow",
			bsonnav.DGetString(widget, "Name"), widgetTypeName(widget))
	}
	switch t := bsonnav.DGetString(action, "$Type"); t {
	case "Forms$MicroflowAction":
		if settings := bsonnav.DGetDoc(action, "MicroflowSettings"); settings != nil {
			return settings, nil
		}
		return nil, fmt.Errorf("microflow action has no MicroflowSettings")
	case "Forms$CallNanoflowClientAction":
		return action, nil
	default:
		return nil, mdlerrors.NewValidationf(
			"widget %q: Confirmation requires a microflow or nanoflow action, and its action is %s — Mendix stores the confirmation dialog on those two only",
			bsonnav.DGetString(widget, "Name"), t)
	}
}

// newConfirmationInfoDoc builds a Forms$ConfirmationInfo in the shape Studio
// Pro stores: three Texts$Text parts, marker-3 Items, default captions written
// out.
func newConfirmationInfoDoc(question string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Forms$ConfirmationInfo"},
		{Key: "CancelButtonCaption", Value: newTextDoc(pages.DefaultConfirmCancelCaption)},
		{Key: "ProceedButtonCaption", Value: newTextDoc(pages.DefaultConfirmProceedCaption)},
		{Key: "Question", Value: newTextDoc(question)},
	}
}

func newTextDoc(text string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Texts$Text"},
		{Key: "Items", Value: bson.A{int32(3), newTranslationDoc(text)}},
	}
}

func newTranslationDoc(text string) bson.D {
	return bson.D{
		{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
		{Key: "$Type", Value: "Texts$Translation"},
		{Key: "LanguageCode", Value: model.AuthoringLanguage()},
		{Key: "Text", Value: text},
	}
}

// setConfirmationText replaces the authoring-language translation of one of
// the dialog's texts, keeping the others.
func setConfirmationText(info bson.D, key, text string) error {
	textDoc := bsonnav.DGetDoc(info, key)
	if textDoc == nil {
		if !bsonnav.DSet(info, key, newTextDoc(text)) {
			return fmt.Errorf("ConfirmationInfo has no %s", key)
		}
		return nil
	}
	lang := model.AuthoringLanguage()
	items := bsonnav.DGetArrayElements(bsonnav.DGet(textDoc, "Items"))
	for _, item := range items {
		tr, ok := item.(bson.D)
		if ok && bsonnav.DGetString(tr, "LanguageCode") == lang {
			bsonnav.DSet(tr, "Text", text)
			return nil
		}
	}
	arr := bson.A{int32(3)}
	arr = append(arr, items...)
	arr = append(arr, newTranslationDoc(text))
	if !bsonnav.DSet(textDoc, "Items", arr) {
		return fmt.Errorf("%s has no Items", key)
	}
	return nil
}
