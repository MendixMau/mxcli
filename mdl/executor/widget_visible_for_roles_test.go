// SPDX-License-Identifier: Apache-2.0

// Studio Pro's "Visible: based on module roles" — ConditionalVisibilitySettings.
// ModuleRoles, a by-name list of Module.Role. MDL had no way to write it: a
// `VisibleForRoles:` key was MDL-WIDGET07 "silently dropped", and the settings
// element was always written with an empty role list, so a project gating a
// Delete button or a menu to Admin could only do it in Studio Pro.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func TestApplyVisibleForRoles(t *testing.T) {
	btn := &pages.ActionButton{}
	w := &ast.WidgetV3{Type: "ACTIONBUTTON", Name: "btnDelete", Properties: map[string]any{
		"VisibleForRoles": []string{"Procurement.Admin", "Procurement.ProcurementOfficer"},
	}}
	if err := applyVisibleForRoles(btn, w); err != nil {
		t.Fatal(err)
	}
	cv := btn.ConditionalVisibility
	if cv == nil {
		t.Fatal("no ConditionalVisibilitySettings — the roles have nowhere to live")
	}
	want := []model.ID{"Procurement.Admin", "Procurement.ProcurementOfficer"}
	if len(cv.ModuleRoles) != 2 || cv.ModuleRoles[0] != want[0] || cv.ModuleRoles[1] != want[1] {
		t.Errorf("ModuleRoles = %v, want %v", cv.ModuleRoles, want)
	}
	if cv.Expression != "" {
		t.Errorf("Expression = %q, want empty (roles only)", cv.Expression)
	}
}

// Roles combine with an expression, as in Studio Pro (both must hold).
func TestApplyVisibleForRoles_KeepsExpression(t *testing.T) {
	btn := &pages.ActionButton{}
	btn.ConditionalVisibility = &pages.ConditionalVisibilitySettings{Expression: "$currentObject/Active"}
	w := &ast.WidgetV3{Type: "ACTIONBUTTON", Name: "b", Properties: map[string]any{
		"VisibleForRoles": []string{"M.Admin"},
	}}
	if err := applyVisibleForRoles(btn, w); err != nil {
		t.Fatal(err)
	}
	if btn.ConditionalVisibility.Expression != "$currentObject/Active" || len(btn.ConditionalVisibility.ModuleRoles) != 1 {
		t.Errorf("got %+v", btn.ConditionalVisibility)
	}
}

func TestApplyVisibleForRoles_RefusesUnqualified(t *testing.T) {
	for _, v := range []any{[]string{"Admin"}, []string{}, "M.Admin"} {
		w := &ast.WidgetV3{Type: "ACTIONBUTTON", Name: "b", Properties: map[string]any{"VisibleForRoles": v}}
		if err := applyVisibleForRoles(&pages.ActionButton{}, w); err == nil {
			t.Errorf("%#v accepted, want an error naming Module.Role", v)
		}
	}
}

// Control: no key, no settings element — the null Studio Pro stores.
func TestApplyVisibleForRoles_AbsentLeavesNil(t *testing.T) {
	btn := &pages.ActionButton{}
	if err := applyVisibleForRoles(btn, &ast.WidgetV3{Type: "ACTIONBUTTON", Name: "b", Properties: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	if btn.ConditionalVisibility != nil {
		t.Errorf("got %+v, want nil", btn.ConditionalVisibility)
	}
}

func TestVisibleForRolesIsKnownStaticProp(t *testing.T) {
	vs := validateStaticWidgetUnknownProps(&ast.WidgetV3{Type: "ACTIONBUTTON", Name: "b",
		Properties: map[string]any{"VisibleForRoles": []string{"M.Admin"}}}, "page P")
	if len(vs) != 0 {
		t.Errorf("got %v, want no MDL-WIDGET07", vs)
	}
}
