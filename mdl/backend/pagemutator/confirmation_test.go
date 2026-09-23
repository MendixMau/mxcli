// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

func microflowButton(info any) bson.D {
	return bson.D{
		{Key: "$Type", Value: "Forms$ActionButton"},
		{Key: "Name", Value: "btnDelete"},
		{Key: "Action", Value: bson.D{
			{Key: "$Type", Value: "Forms$MicroflowAction"},
			{Key: "MicroflowSettings", Value: bson.D{
				{Key: "$Type", Value: "Forms$MicroflowSettings"},
				{Key: "ConfirmationInfo", Value: info},
				{Key: "Microflow", Value: "M.ACT_Delete"},
			}},
		}},
	}
}

func confirmInfoOf(t *testing.T, raw bson.D) bson.D {
	t.Helper()
	w := findBsonWidget(raw, "btnDelete")
	if w == nil {
		t.Fatal("btnDelete not found")
	}
	settings := bsonnav.DGetDoc(bsonnav.DGetDoc(w.widget, "Action"), "MicroflowSettings")
	return bsonnav.DGetDoc(settings, "ConfirmationInfo")
}

// translations returns LanguageCode → Text for the Texts$Text at key.
func translations(info bson.D, key string) map[string]string {
	out := map[string]string{}
	for _, it := range bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(info, key), "Items")) {
		tr := it.(bson.D)
		out[bsonnav.DGetString(tr, "LanguageCode")] = bsonnav.DGetString(tr, "Text")
	}
	return out
}

// `set Confirmation = '…' on btn` on a button with no dialog creates one in
// the Studio Pro shape, default captions included; the captions then edit it.
func TestSetConfirmation_CreatesThenEditsDialog(t *testing.T) {
	raw := makeRawPage(microflowButton(nil))
	m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}

	if err := m.SetWidgetProperty("btnDelete", "Confirmation", "Are you sure?"); err != nil {
		t.Fatalf("set Confirmation: %v", err)
	}
	info := confirmInfoOf(t, raw)
	if info == nil {
		t.Fatal("ConfirmationInfo still null")
	}
	if got := bsonnav.DGetString(info, "$Type"); got != "Forms$ConfirmationInfo" {
		t.Errorf("$Type = %q", got)
	}
	if got := translations(info, "Question")["en_US"]; got != "Are you sure?" {
		t.Errorf("Question = %q", got)
	}
	if got := translations(info, "ProceedButtonCaption")["en_US"]; got != "Proceed" {
		t.Errorf("Proceed = %q, want the Studio Pro default", got)
	}

	if err := m.SetWidgetProperty("btnDelete", "confirmcancel", "No"); err != nil {
		t.Fatalf("set ConfirmCancel: %v", err)
	}
	if got := translations(confirmInfoOf(t, raw), "CancelButtonCaption")["en_US"]; got != "No" {
		t.Errorf("Cancel = %q, want No", got)
	}

	// Empty question removes the dialog.
	if err := m.SetWidgetProperty("btnDelete", "Confirmation", ""); err != nil {
		t.Fatalf("clear Confirmation: %v", err)
	}
	if info := confirmInfoOf(t, raw); info != nil {
		t.Errorf("ConfirmationInfo = %v, want null after clearing", info)
	}
}

// Editing keeps the other languages a Studio Pro project stores.
func TestSetConfirmation_KeepsOtherTranslations(t *testing.T) {
	text := func(en, nl string) bson.D {
		return bson.D{{Key: "$Type", Value: "Texts$Text"}, {Key: "Items", Value: bson.A{int32(3),
			bson.D{{Key: "$Type", Value: "Texts$Translation"}, {Key: "LanguageCode", Value: "en_US"}, {Key: "Text", Value: en}},
			bson.D{{Key: "$Type", Value: "Texts$Translation"}, {Key: "LanguageCode", Value: "nl_NL"}, {Key: "Text", Value: nl}},
		}}}
	}
	info := bson.D{
		{Key: "$Type", Value: "Forms$ConfirmationInfo"},
		{Key: "CancelButtonCaption", Value: text("Cancel", "Annuleren")},
		{Key: "ProceedButtonCaption", Value: text("Proceed", "Doorgaan")},
		{Key: "Question", Value: text("Sure?", "Zeker?")},
	}
	raw := makeRawPage(microflowButton(info))
	m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("btnDelete", "Confirmation", "Really?"); err != nil {
		t.Fatal(err)
	}
	got := translations(confirmInfoOf(t, raw), "Question")
	if got["en_US"] != "Really?" || got["nl_NL"] != "Zeker?" {
		t.Errorf("Question = %v, want en_US replaced and nl_NL kept", got)
	}
}

// Refused, not dropped: a caption without a dialog, and a dialog on an action
// that has no slot for one.
func TestSetConfirmation_Refusals(t *testing.T) {
	raw := makeRawPage(microflowButton(nil))
	m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("btnDelete", "ConfirmProceed", "Yes"); err == nil {
		t.Error("ConfirmProceed accepted on an action with no dialog")
	}

	save := bson.D{
		{Key: "$Type", Value: "Forms$ActionButton"},
		{Key: "Name", Value: "btnSave"},
		{Key: "Action", Value: bson.D{{Key: "$Type", Value: "Forms$SaveChangesClientAction"}}},
	}
	raw2 := makeRawPage(save)
	m2 := &Mutator{rawData: raw2, widgetFinder: findBsonWidget}
	if err := m2.SetWidgetProperty("btnSave", "Confirmation", "Q?"); err == nil {
		t.Error("Confirmation accepted on a save_changes action")
	}
	if bsonnav.DGet(bsonnav.DGetDoc(findBsonWidget(raw2, "btnSave").widget, "Action"), "ConfirmationInfo") != nil {
		t.Error("a ConfirmationInfo key was invented on a save_changes action")
	}
}
