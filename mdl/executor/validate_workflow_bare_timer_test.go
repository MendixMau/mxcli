// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// MDL-WF07. A bare `boundary event timer` writes Workflows$TimerBoundaryEvent,
// which no Mendix 11 runtime has. Measured on 11.14.0: check and mxbuild pass,
// and the runtime refuses to start the application — "Class
// 'Workflows$TimerBoundaryEvent' could not be found". It was the example in
// `mxcli syntax workflow boundary-event`.

func pvAt(major, minor int, product string) func() *types.ProjectVersion {
	return func() *types.ProjectVersion {
		return &types.ProjectVersion{MajorVersion: major, MinorVersion: minor, ProductVersion: product}
	}
}

func hasWfRule(errs []string, rule string) bool {
	for _, e := range errs {
		if strings.Contains(e, rule) {
			return true
		}
	}
	return false
}

func createWfRefErrors(t *testing.T, ctx *ExecContext, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateWorkflowStmt)
	if !ok {
		t.Fatalf("not a CREATE WORKFLOW: %T", prog.Statements[0])
	}
	return validateWorkflowStatementRefs(ctx, stmt, nil)
}

const wfBoundary = `create workflow Sales.WF2
  parameter $Context: Sales.Order
begin
  user task Review 'Review'
    page Sales.ReviewPage
    outcomes 'Done' { }
    boundary event %KIND%timer 'addHours([%CurrentDateTime%], 1)' {
      call microflow Sales.ACT_Escalate;
    };
end workflow;`

func TestMDLWF07_CreateRefusesABareTimerOn11(t *testing.T) {
	ctx, mb := wfKindFixture(t)
	mb.ProjectVersionFunc = pvAt(11, 14, "11.14.0")

	if errs := createWfRefErrors(t, ctx, strings.Replace(wfBoundary, "%KIND%", "", 1)); !hasWfRule(errs, "MDL-WF07") {
		t.Errorf("a bare timer on 11.14 must be refused with MDL-WF07, got: %v", errs)
	}
	for _, kind := range []string{"interrupting ", "non interrupting "} {
		if errs := createWfRefErrors(t, ctx, strings.Replace(wfBoundary, "%KIND%", kind, 1)); hasWfRule(errs, "MDL-WF07") {
			t.Errorf("`%stimer` names its kind and must not be refused: %v", kind, errs)
		}
	}
}

// The gate is the measurement: no 11.x runtime has the type, and whether a 10.x
// one did is unknown here, so 10.x is left alone rather than guessed at.
func TestMDLWF07_NotAppliedBelow11(t *testing.T) {
	ctx, mb := wfKindFixture(t)
	mb.ProjectVersionFunc = pvAt(10, 24, "10.24.0")
	if errs := createWfRefErrors(t, ctx, strings.Replace(wfBoundary, "%KIND%", "", 1)); hasWfRule(errs, "MDL-WF07") {
		t.Errorf("MDL-WF07 must not fire on 10.24: %v", errs)
	}
}

// INSERT BOUNDARY EVENT writes the same type, and so does a boundary event
// nested in an inserted activity — either would put the refusal one keyword away.
func TestMDLWF07_AlterRefusesABareTimerOn11(t *testing.T) {
	ctx, mb := wfKindFixture(t)
	mb.ProjectVersionFunc = pvAt(11, 14, "11.14.0")

	cases := map[string]struct {
		src  string
		want bool
	}{
		"insert boundary event, bare": {
			`alter workflow Sales.WF insert boundary event on task1 timer 'addHours([%CurrentDateTime%], 1)' { call microflow Sales.ACT_Escalate; };`, true},
		"insert boundary event, interrupting": {
			`alter workflow Sales.WF insert boundary event on task1 interrupting timer 'addHours([%CurrentDateTime%], 1)' { call microflow Sales.ACT_Escalate; };`, false},
		"bare timer nested in an inserted user task": {
			`alter workflow Sales.WF insert after task1 user task Second 'Second' page Sales.ReviewPage outcomes 'Done' { } boundary event timer 'addHours([%CurrentDateTime%], 1)' { call microflow Sales.ACT_Escalate; };`, true},
	}
	for name, tc := range cases {
		if got := hasWfRule(alterWfRefErrors(t, ctx, tc.src), "MDL-WF07"); got != tc.want {
			t.Errorf("%s: MDL-WF07 reported = %v, want %v", name, got, tc.want)
		}
	}
}
