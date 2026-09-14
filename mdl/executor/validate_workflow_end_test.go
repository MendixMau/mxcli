// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// An `end workflow` is named after its caption, "End" by default — the name of
// the End mxcli appends to the main flow. Measured before the fix: two nested
// Ends plus the main one built as CE0495 "Duplicate name 'End'". The implicit
// Start and End are reserved, so nothing in the user activities takes them.
func TestDeduplicateActivityNames_ReservesImplicitStartAndEnd(t *testing.T) {
	end := func() *workflows.EndWorkflowActivity {
		e := &workflows.EndWorkflowActivity{}
		e.Name = "End"
		return e
	}
	a, b := end(), end()
	deduplicateActivityNames([]workflows.WorkflowActivity{a, b}, "Start", "End")
	if a.Name == "End" || b.Name == "End" || a.Name == b.Name {
		t.Errorf("nested Ends must not take the main End's name or each other's: got %q, %q", a.Name, b.Name)
	}

	// Control: without the reservation the first one takes "End".
	c := end()
	deduplicateActivityNames([]workflows.WorkflowActivity{c})
	if c.Name != "End" {
		t.Fatalf("control: expected the unreserved list to keep %q, got %q", "End", c.Name)
	}
}

// Only the main flow's End is implicit. A nested one is emitted (see
// TestDescribeWorkflow_NestedEndIsEmitted); this one must not be, or every
// describe would print a stray `end workflow;` before the closer.
func TestDescribeWorkflow_MainFlowEndStaysImplicit(t *testing.T) {
	start := &workflows.StartWorkflowActivity{}
	end := &workflows.EndWorkflowActivity{}
	end.Name, end.Caption = "End", "End"
	out := strings.Join(formatMainFlowActivities(&workflows.Flow{Activities: []workflows.WorkflowActivity{start, end}}, "  "), "\n")
	if strings.Contains(out, "end workflow") {
		t.Errorf("the main flow's End must stay implicit, got:\n%s", out)
	}
}

// exec builds nothing from a `return;`, so without the guard `exec --no-check`
// would drop it and the branch would fall through.
func TestRefuseWorkflowReturn(t *testing.T) {
	nested := []ast.WorkflowActivityNode{&ast.WorkflowUserTaskNode{
		Outcomes: []ast.WorkflowUserTaskOutcomeNode{{Caption: "Reject", Activities: []ast.WorkflowActivityNode{&ast.WorkflowReturnNode{}}}},
	}}
	err := refuseWorkflowReturn(nested)
	if err == nil || !strings.Contains(err.Error(), "end workflow") || !strings.Contains(err.Error(), "MDL-WF11") {
		t.Errorf("a nested `return` must be refused with a pointer to `end workflow` [MDL-WF11], got %v", err)
	}
	if err := refuseWorkflowReturn([]ast.WorkflowActivityNode{&ast.WorkflowEndNode{}}); err != nil {
		t.Errorf("`end workflow` must not be refused as a return: %v", err)
	}
}

var endRules = []string{"MDL-WF08", "MDL-WF09", "MDL-WF10", "MDL-WF11"}

// firedEndRules returns which of the End rules fired, in endRules order.
func firedEndRules(vs [][2]string) []string {
	var out []string
	for _, id := range endRules {
		if hasRule(vs, id) {
			out = append(out, id)
		}
	}
	return out
}

func endWF(body string) string {
	return wfPreamble + "create workflow WF.W parameter $Ctx: WF.Ctx\nbegin\n" + body + "\nend workflow;"
}

const endNextTask = `user task Next 'Next' page WF.TaskPage outcomes 'ok' { } 'no' { };`

// Every row is a placement measured against mxbuild (11.13.0) with the real
// syntax — want is the one End rule that predicts its CE code, "" where the
// build was clean. The clean rows are what keep the rules from refusing valid
// workflows, and each case checks that ONLY its rule fires.
func TestValidateWorkflow_EndPlacement(t *testing.T) {
	cases := []struct{ name, body, want string }{
		// Measured clean.
		{"outcome ends", `user task A 'a' page WF.TaskPage outcomes 'Reject' { end workflow; } 'Approve' { };` + endNextTask, ""},
		{"decision branch ends", `decision '$Ctx/Total > 1000' outcomes true -> { end workflow; } false -> { };` + endNextTask, ""},
		{"call microflow outcome ends", `call microflow WF.ACT outcomes true -> { end workflow; } false -> { };` + endNextTask, ""},
		{"interrupting boundary path ends", `user task A 'a' page WF.TaskPage outcomes 'x' { } 'y' { }
  boundary event interrupting timer 'addDays([%CurrentDateTime%], 3)' { end workflow; };`, ""},
		{"deep inside a decision branch", `decision '$Ctx/Total > 1000' outcomes
  true -> { user task N 'n' page WF.TaskPage outcomes 'Reject' { end workflow; } 'Approve' { }; }
  false -> { };` + endNextTask, ""},
		{"nested under an interrupting boundary", `user task A 'a' page WF.TaskPage outcomes 'x' { } 'y' { }
  boundary event interrupting timer 'addDays([%CurrentDateTime%], 3)' {
    user task N 'n' page WF.TaskPage outcomes 'Reject' { end workflow; } 'Approve' { };
  };`, ""},
		{"two ends, one branch continues", `user task A 'a' page WF.TaskPage outcomes 'Reject' { end workflow; } 'Cancel' { end workflow; } 'Approve' { };` + endNextTask, ""},

		// CE1844.
		{"parallel path", `parallel split
  path 1 { user task P1 'p1' page WF.TaskPage outcomes 'x' { } 'y' { }; end workflow; }
  path 2 { user task P2 'p2' page WF.TaskPage outcomes 'x' { } 'y' { }; };`, "MDL-WF08"},
		{"outcome nested in a parallel path", `parallel split
  path 1 { user task P1 'p1' page WF.TaskPage outcomes 'Reject' { end workflow; } 'Approve' { }; }
  path 2 { user task P2 'p2' page WF.TaskPage outcomes 'x' { } 'y' { }; };`, "MDL-WF08"},
		{"non-interrupting boundary path", `user task A 'a' page WF.TaskPage outcomes 'x' { } 'y' { }
  boundary event non interrupting timer 'addDays([%CurrentDateTime%], 3)' { end workflow; };`, "MDL-WF08"},
		{"outcome nested under a non-interrupting boundary", `user task A 'a' page WF.TaskPage outcomes 'x' { } 'y' { }
  boundary event non interrupting timer 'addDays([%CurrentDateTime%], 3)' {
    user task N 'n' page WF.TaskPage outcomes 'Reject' { end workflow; } 'Approve' { };
  };`, "MDL-WF08"},

		// CE6671.
		{"activity after end workflow", `user task A 'a' page WF.TaskPage outcomes 'Reject' { end workflow; call microflow WF.ACT; } 'Approve' { };` + endNextTask, "MDL-WF09"},

		// CE6689.
		{"both outcomes end, then an activity", `user task A 'a' page WF.TaskPage outcomes 'x' { end workflow; } 'y' { end workflow; };` + endNextTask, "MDL-WF10"},
		{"both outcomes end, last in the main flow", `user task A 'a' page WF.TaskPage outcomes 'x' { end workflow; } 'y' { end workflow; };`, "MDL-WF10"},
		{"decision branches both end, then an activity", `decision '$Ctx/Total > 1000' outcomes true -> { end workflow; } false -> { end workflow; };` + endNextTask, "MDL-WF10"},
		{"every outcome jumps, then an activity", `user task A 'a' page WF.TaskPage outcomes 'x' { } 'y' { };
user task B 'b' page WF.TaskPage outcomes 'x' { jump to A; } 'y' { jump to A; };` + endNextTask, "MDL-WF10"},
		{"one ends and one jumps, then an activity", `user task A 'a' page WF.TaskPage outcomes 'x' { } 'y' { };
user task B 'b' page WF.TaskPage outcomes 'x' { end workflow; } 'y' { jump to A; };` + endNextTask, "MDL-WF10"},
		{"termination through a nested decision", `user task A 'a' page WF.TaskPage outcomes
  'x' { decision '$Ctx/Total > 1000' outcomes true -> { end workflow; } false -> { end workflow; }; }
  'y' { end workflow; };` + endNextTask, "MDL-WF10"},
		{"main flow ends in a jump", `user task A 'a' page WF.TaskPage outcomes 'x' { } 'y' { };
user task B 'b' page WF.TaskPage outcomes 'x' { } 'y' { };
jump to A;`, "MDL-WF10"},

		// `return` is a microflow's.
		{"return in a branch", `user task A 'a' page WF.TaskPage outcomes 'Reject' { return; } 'Approve' { };` + endNextTask, "MDL-WF11"},
		{"return in the main flow", `user task A 'a' page WF.TaskPage outcomes 'x' { } 'y' { };
return;`, "MDL-WF11"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vs := workflowViolations(t, endWF(c.body))
			got := strings.Join(firedEndRules(vs), ",")
			if got != c.want {
				t.Errorf("End rules fired = [%s], want [%s]\nviolations: %v", got, c.want, vs)
			}
		})
	}
}

// A single outcome may not carry activities at all (CE1876, MDL-WF02), `end
// workflow` included — measured, the build reports that AND CE6689 for the main
// End. One mistake gets one report: MDL-WF02, with advice that fits an End,
// rather than "move the activities to the main flow".
func TestValidateWorkflow_SingleOutcomeEndIsMDLWF02Only(t *testing.T) {
	vs := workflowViolations(t, endWF(`user task A 'a' page WF.TaskPage outcomes 'Done' { end workflow; };`+endNextTask))
	if !hasRule(vs, "MDL-WF02") {
		t.Fatalf("expected MDL-WF02, got %v", vs)
	}
	if fired := firedEndRules(vs); len(fired) > 0 {
		t.Errorf("a single-outcome End is MDL-WF02's to report, but %v also fired", fired)
	}
	for _, v := range vs {
		if v[0] == "MDL-WF02" && !strings.Contains(v[1], "end workflow") {
			t.Errorf("MDL-WF02 for an End-only outcome should say so, got %q", v[1])
		}
	}
}

func alterViolations(t *testing.T, src string) [][2]string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var out [][2]string
	for _, stmt := range prog.Statements {
		if s, ok := stmt.(*ast.AlterWorkflowStmt); ok {
			for _, v := range ValidateAlterWorkflow(s) {
				out = append(out, [2]string{v.RuleID, v.Message})
			}
		}
	}
	return out
}

// ALTER inserts take a body too. What the statement alone decides — a path
// body is under a split, a non-interrupting boundary body is what it says — is
// checked without a project.
func TestValidateAlterWorkflow_EndPlacement(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"insert outcome that ends", `alter workflow WF.W insert outcome 'Stop' on T { end workflow; };`, ""},
		{"insert path that ends", `alter workflow WF.W insert path on split1 { end workflow; };`, "MDL-WF08"},
		{"insert non-interrupting boundary that ends", `alter workflow WF.W insert boundary event on T non interrupting timer 'addDays([%CurrentDateTime%], 3)' { end workflow; };`, "MDL-WF08"},
		{"insert interrupting boundary that ends", `alter workflow WF.W insert boundary event on T interrupting timer 'addDays([%CurrentDateTime%], 3)' { end workflow; };`, ""},
		{"insert outcome with an activity after the end", `alter workflow WF.W insert outcome 'Stop' on T { end workflow; call microflow WF.ACT; };`, "MDL-WF09"},
		{"insert outcome with return", `alter workflow WF.W insert outcome 'Stop' on T { return; };`, "MDL-WF11"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vs := alterViolations(t, c.src)
			if got := strings.Join(firedEndRules(vs), ","); got != c.want {
				t.Errorf("End rules fired = [%s], want [%s]\nviolations: %v", got, c.want, vs)
			}
		})
	}
}

// storedEndAncestryCtx is a project holding WF.W: a top-level task, a task inside a
// parallel split path, and a task inside a non-interrupting boundary event path.
func storedEndAncestryCtx(t *testing.T) *ExecContext {
	t.Helper()
	task := func(name string) *workflows.UserTask {
		ut := &workflows.UserTask{Outcomes: []*workflows.UserTaskOutcome{{Value: "a"}, {Value: "b"}}}
		ut.Name = name
		return ut
	}
	top := task("Top")
	top.BoundaryEvents = []*workflows.BoundaryEvent{{
		EventType: "NonInterruptingTimer",
		Flow:      &workflows.Flow{Activities: []workflows.WorkflowActivity{task("InBoundary")}},
	}}
	split := &workflows.ParallelSplitActivity{Outcomes: []*workflows.ParallelSplitOutcome{
		{Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{task("InPath")}}},
	}}
	split.Name = "split1"
	wf := &workflows.Workflow{Name: "W", Flow: &workflows.Flow{Activities: []workflows.WorkflowActivity{top, split}}}
	wf.ContainerID = "mod1"
	mod := &model.Module{Name: "WF"}
	mod.ID = "mod1"
	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{wf}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx
}

// An inserted outcome looks harmless in the statement; whether its `end
// workflow` is legal depends on where its task already sits (CE1844).
func TestValidateAlterWorkflow_EndAncestryFromStoredWorkflow(t *testing.T) {
	cases := []struct {
		name, src string
		refused   bool
	}{
		{"task in a parallel path", `alter workflow WF.W insert outcome 'Stop' on InPath { end workflow; };`, true},
		{"task in a non-interrupting boundary path", `alter workflow WF.W insert outcome 'Stop' on InBoundary { end workflow; };`, true},
		{"top-level task", `alter workflow WF.W insert outcome 'Stop' on Top { end workflow; };`, false},
		// Control: the same target, nothing that ends.
		{"task in a parallel path, no end", `alter workflow WF.W insert outcome 'Stop' on InPath { };`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, errs := visitor.Build(c.src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			got := validateAlterWorkflowEndAncestry(storedEndAncestryCtx(t), prog.Statements[0].(*ast.AlterWorkflowStmt))
			if refused := len(got) > 0; refused != c.refused {
				t.Errorf("refused = %v, want %v: %v", refused, c.refused, got)
			}
			if c.refused && (len(got) != 1 || !strings.Contains(got[0], "CE1844")) {
				t.Errorf("expected one CE1844 refusal, got %v", got)
			}
		})
	}
}

// A caption is free text and a name is an identifier. The End's name was its
// caption, so `end workflow comment 'Rejected by manager'` built CE7247 on both
// engines — caught by the doctype script, not by the end-to-end fixture, whose
// captions were all one word.
func TestBuildEndWorkflow_NameIsAnIdentifierWhateverTheCaption(t *testing.T) {
	valid := func(name string) bool {
		if name == "" {
			return false
		}
		for i, r := range name {
			letter := r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
			digit := r >= '0' && r <= '9'
			if !letter && !(digit && i > 0) {
				return false
			}
		}
		return true
	}
	for _, caption := range []string{"", "Rejected", "Rejected by manager", "3 days passed", "Überschritten!"} {
		end := buildEndWorkflow(&ast.WorkflowEndNode{Caption: caption})
		if !valid(end.Name) {
			t.Errorf("caption %q built the name %q, which mxbuild refuses (CE7247)", caption, end.Name)
		}
		want := caption
		if want == "" {
			want = "End"
		}
		if end.Caption != want {
			t.Errorf("caption %q must be kept as written, got %q", caption, end.Caption)
		}
	}
}
