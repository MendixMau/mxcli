// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// confirmTextOf returns the Text of the first Texts$Translation under a
// Texts$Text stored at key, and its Items marker.
func confirmTextOf(t *testing.T, info bsonv1.D, key string) (text string, marker int32) {
	t.Helper()
	td, ok := docGet(info, key).(bsonv1.D)
	if !ok {
		t.Fatalf("%s is not a document: %#v", key, docGet(info, key))
	}
	if got := docGet(td, "$Type"); got != "Texts$Text" {
		t.Fatalf("%s $Type = %v, want Texts$Text", key, got)
	}
	arr, ok := docGet(td, "Items").(bsonv1.A)
	if !ok || len(arr) < 2 {
		t.Fatalf("%s Items not a non-empty typed array: %#v", key, docGet(td, "Items"))
	}
	marker, _ = arr[0].(int32)
	tr := arr[1].(bsonv1.D)
	if got := docGet(tr, "$Type"); got != "Texts$Translation" {
		t.Fatalf("%s item $Type = %v", key, got)
	}
	s, _ := docGet(tr, "Text").(string)
	return s, marker
}

func txt(s string) *model.Text {
	return &model.Text{Translations: map[string]string{"en_US": s}}
}

// A microflow action's confirmation dialog is stored on its MicroflowSettings
// as Forms$ConfirmationInfo — the shape measured on Studio Pro pages
// (AppStore.Framework_Overview): three Texts$Text parts with marker-3 Items and
// the default captions written out.
func TestMicroflowAction_ConfirmationInfo(t *testing.T) {
	doc := encodeAction(t, &pages.MicroflowClientAction{
		MicroflowName: "M.ACT_Delete",
		Confirmation:  &pages.ConfirmationInfo{Question: txt("Are you sure?")},
	})
	settings, ok := docGet(doc, "MicroflowSettings").(bsonv1.D)
	if !ok {
		t.Fatalf("MicroflowSettings missing")
	}
	info, ok := docGet(settings, "ConfirmationInfo").(bsonv1.D)
	if !ok {
		t.Fatalf("ConfirmationInfo = %#v, want a Forms$ConfirmationInfo document", docGet(settings, "ConfirmationInfo"))
	}
	if got := docGet(info, "$Type"); got != "Forms$ConfirmationInfo" {
		t.Errorf("$Type = %v", got)
	}
	for key, want := range map[string]string{
		"Question":             "Are you sure?",
		"ProceedButtonCaption": "Proceed",
		"CancelButtonCaption":  "Cancel",
	} {
		got, marker := confirmTextOf(t, info, key)
		if got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
		if marker != 3 {
			t.Errorf("%s Items marker = %d, want 3 (Studio Pro)", key, marker)
		}
	}
}

func TestNanoflowAction_ConfirmationInfo(t *testing.T) {
	doc := encodeAction(t, &pages.NanoflowClientAction{
		NanoflowName: "M.NAV_Go",
		Confirmation: &pages.ConfirmationInfo{
			Question:       txt("Continue?"),
			ProceedCaption: txt("Yes"),
			CancelCaption:  txt("No"),
		},
	})
	info, ok := docGet(doc, "ConfirmationInfo").(bsonv1.D)
	if !ok {
		t.Fatalf("ConfirmationInfo = %#v, want a document", docGet(doc, "ConfirmationInfo"))
	}
	for key, want := range map[string]string{"Question": "Continue?", "ProceedButtonCaption": "Yes", "CancelButtonCaption": "No"} {
		if got, _ := confirmTextOf(t, info, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

// Without a dialog the slot stays null, as Studio Pro stores it.
func TestMicroflowAction_NoConfirmationIsNull(t *testing.T) {
	doc := encodeAction(t, &pages.MicroflowClientAction{MicroflowName: "M.ACT_X"})
	settings := docGet(doc, "MicroflowSettings").(bsonv1.D)
	found := false
	for _, e := range settings {
		if e.Key == "ConfirmationInfo" {
			found = true
			if e.Value != nil {
				t.Errorf("ConfirmationInfo = %#v, want null", e.Value)
			}
		}
	}
	if !found {
		t.Errorf("ConfirmationInfo key missing — Studio Pro stores it as null")
	}
}
