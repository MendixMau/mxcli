// SPDX-License-Identifier: Apache-2.0

// Studio Pro's "Ask confirmation" on a call-microflow / call-nanoflow action —
// Forms$ConfirmationInfo (question, proceed caption, cancel caption) on the
// microflow settings or the nanoflow action. MDL had no spelling for it:
// `Confirmation:` on a button was MDL-WIDGET07 "silently dropped", and the
// writer always stored ConfirmationInfo null. A Delete button therefore could
// not ask before deleting without Studio Pro.
//
// A Delete / Save / Close action has NO confirmation in Mendix — the model
// types carry none — so the property is refused there with the remedy (call a
// microflow that deletes) rather than dropped.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func TestApplyActionConfirmation_Microflow(t *testing.T) {
	act := pages.ClientAction(&pages.MicroflowClientAction{MicroflowName: "M.ACT_Delete"})
	w := &ast.WidgetV3{Type: "ACTIONBUTTON", Name: "btnDelete", Properties: map[string]any{
		"Confirmation": "Delete this department?", "ConfirmationProceed": "Delete",
	}}
	if err := applyActionConfirmation(w, act); err != nil {
		t.Fatal(err)
	}
	c := act.(*pages.MicroflowClientAction).Confirmation
	if c == nil || c.Question != "Delete this department?" || c.ProceedCaption != "Delete" || c.CancelCaption != "Cancel" {
		t.Errorf("Confirmation = %+v, want question + Delete / Cancel (Studio Pro's default cancel caption)", c)
	}
}

func TestApplyActionConfirmation_Nanoflow(t *testing.T) {
	act := pages.ClientAction(&pages.NanoflowClientAction{NanoflowName: "M.NF"})
	w := &ast.WidgetV3{Type: "ACTIONBUTTON", Name: "b", Properties: map[string]any{"Confirmation": "Sure?"}}
	if err := applyActionConfirmation(w, act); err != nil {
		t.Fatal(err)
	}
	c := act.(*pages.NanoflowClientAction).Confirmation
	if c == nil || c.ProceedCaption != "Proceed" || c.CancelCaption != "Cancel" {
		t.Errorf("Confirmation = %+v, want Studio Pro's defaults Proceed / Cancel", c)
	}
}

func TestApplyActionConfirmation_RefusedOnDelete(t *testing.T) {
	w := &ast.WidgetV3{Type: "ACTIONBUTTON", Name: "b", Properties: map[string]any{"Confirmation": "Sure?"}}
	if err := applyActionConfirmation(w, &pages.DeleteClientAction{}); err == nil {
		t.Error("a Delete action has no ConfirmationInfo in Mendix; the property must be refused, not dropped")
	}
	if err := applyActionConfirmation(w, nil); err == nil {
		t.Error("Confirmation with no action at all must be refused")
	}
}

// Control: no key, nothing changes.
func TestApplyActionConfirmation_Absent(t *testing.T) {
	act := &pages.MicroflowClientAction{MicroflowName: "M.X"}
	if err := applyActionConfirmation(&ast.WidgetV3{Name: "b", Properties: map[string]any{}}, act); err != nil {
		t.Fatal(err)
	}
	if act.Confirmation != nil {
		t.Errorf("got %+v, want nil", act.Confirmation)
	}
}

func TestValidateActionConfirmation(t *testing.T) {
	bad := &ast.WidgetV3{Type: "ACTIONBUTTON", Name: "b", Properties: map[string]any{
		"Confirmation": "Sure?", "Action": &ast.ActionV3{Type: "delete"},
	}}
	if vs := validateActionConfirmation(bad, "page P"); len(vs) != 1 || vs[0].RuleID != "MDL-WIDGET35" {
		t.Errorf("Confirmation on a delete action: got %v, want one MDL-WIDGET35", vs)
	}
	good := &ast.WidgetV3{Type: "ACTIONBUTTON", Name: "b", Properties: map[string]any{
		"Confirmation": "Sure?", "Action": &ast.ActionV3{Type: "microflow", Target: "M.X"},
	}}
	if vs := validateActionConfirmation(good, "page P"); len(vs) != 0 {
		t.Errorf("Confirmation on a microflow action: got %v, want none", vs)
	}
}
