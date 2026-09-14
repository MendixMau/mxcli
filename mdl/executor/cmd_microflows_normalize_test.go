// SPDX-License-Identifier: Apache-2.0

package executor

// Mode 3 rewrites a copy of the graph and lets the ordinary describer render
// it. These tests pin the rewrite; the folding algebra itself is tested in
// mdl/microflownorm (including a truth-table check over 3000 random formulas).
//
// The refusals matter as much as the fold. Every one of them is a case where
// producing a guard anyway would give output that looks right and is not.

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// reporterGraph builds mendixlabs/mxcli#923's graph:
//
//	split1 : true → split2      false → merge1
//	split2 : true → merge1      false → merge2
//	merge1 → act → merge2 → end
//
// `act` runs on ¬c1 ∨ c2. Nested ifs describe it as c1 ∧ c2 — the defect.
func reporterGraph(innerCond string) *rejoinFixture {
	f := newRejoinFixture()
	f.add("start", &microflows.StartEvent{BaseMicroflowObject: f.base(0)})
	f.add("split1", &microflows.ExclusiveSplit{
		BaseMicroflowObject: f.base(100),
		SplitCondition:      &microflows.ExpressionSplitCondition{Expression: "$A"},
	})
	f.add("split2", &microflows.ExclusiveSplit{
		BaseMicroflowObject: f.base(200),
		SplitCondition:      &microflows.ExpressionSplitCondition{Expression: innerCond},
	})
	f.add("merge1", &microflows.ExclusiveMerge{BaseMicroflowObject: f.base(300)})
	f.add("act", &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(350)}})
	f.add("merge2", &microflows.ExclusiveMerge{BaseMicroflowObject: f.base(400)})
	f.add("end", &microflows.EndEvent{BaseMicroflowObject: f.base(500)})
	f.edge("start", "split1", false)
	f.branch("split1", "split2", true)
	f.branch("split1", "merge1", false)
	f.branch("split2", "merge1", true)
	f.branch("split2", "merge2", false)
	f.edge("merge1", "act", false)
	f.edge("act", "merge2", false)
	f.edge("merge2", "end", false)
	return f
}

func TestNormalizeCollection_FoldsTheReportersGraph(t *testing.T) {
	f := reporterGraph("$B")
	got, notes := normalizeCollection(f.col)

	if len(notes) != 1 || !strings.Contains(notes[0], "NORMALIZED") {
		t.Fatalf("expected one NORMALIZED note, got %v", notes)
	}

	var folded *microflows.ExclusiveSplit
	splits := 0
	for _, o := range got.Objects {
		if s, ok := o.(*microflows.ExclusiveSplit); ok {
			splits++
			folded = s
		}
	}
	if splits != 1 {
		t.Fatalf("got %d splits after folding, want 1 — the inner split should be gone", splits)
	}
	expr, ok := folded.SplitCondition.(*microflows.ExpressionSplitCondition)
	if !ok {
		t.Fatal("folded split lost its expression condition")
	}
	if want := "$B or not($A)"; expr.Expression != want {
		t.Errorf("folded condition = %q, want %q", expr.Expression, want)
	}
	// The #923 misdescription, stated so a regression names itself.
	if expr.Expression == "$A and $B" {
		t.Error("folded to the conjunction — this is the bug Mode 3 exists to fix")
	}
}

// DESCRIBE must not touch the stored model, so the fold works on a copy.
func TestNormalizeCollection_DoesNotMutateInput(t *testing.T) {
	f := reporterGraph("$B")
	before := len(f.col.Objects)
	var originalCond string
	for _, o := range f.col.Objects {
		if s, ok := o.(*microflows.ExclusiveSplit); ok {
			if e, ok := s.SplitCondition.(*microflows.ExpressionSplitCondition); ok && e.Expression == "$A" {
				originalCond = e.Expression
			}
		}
	}

	normalizeCollection(f.col)

	if len(f.col.Objects) != before {
		t.Errorf("input collection lost objects: %d -> %d", before, len(f.col.Objects))
	}
	for _, o := range f.col.Objects {
		if s, ok := o.(*microflows.ExclusiveSplit); ok {
			if e, ok := s.SplitCondition.(*microflows.ExpressionSplitCondition); ok && e.Expression == "$B or not($A)" {
				t.Fatal("the fold wrote its condition back into the input — a DESCRIBE must not mutate the model")
			}
		}
	}
	if originalCond != "$A" {
		t.Fatalf("fixture is wrong: outer condition was %q", originalCond)
	}
}

// A properly nested graph has nothing to fold, and Mode 3 must leave it exactly
// as Mode 1 renders it. This is the regression risk of the whole feature.
func TestNormalizeCollection_LeavesNestedGraphAlone(t *testing.T) {
	f := newRejoinFixture()
	f.add("start", &microflows.StartEvent{BaseMicroflowObject: f.base(0)})
	f.add("split", &microflows.ExclusiveSplit{
		BaseMicroflowObject: f.base(100),
		SplitCondition:      &microflows.ExpressionSplitCondition{Expression: "$X"},
	})
	f.add("then", &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(150)}})
	f.add("merge", &microflows.ExclusiveMerge{BaseMicroflowObject: f.base(200)})
	f.add("end", &microflows.EndEvent{BaseMicroflowObject: f.base(300)})
	f.edge("start", "split", false)
	f.branch("split", "then", true)
	f.branch("split", "merge", false)
	f.edge("then", "merge", false)
	f.edge("merge", "end", false)

	got, notes := normalizeCollection(f.col)
	if len(notes) != 0 {
		t.Errorf("a properly nested graph produced notes: %v", notes)
	}
	if len(got.Objects) != len(f.col.Objects) {
		t.Errorf("a properly nested graph was rewritten: %d -> %d objects", len(f.col.Objects), len(got.Objects))
	}
}

// THE PRECONDITION, at the executor level. An activity between the split and
// the shared suffix means folding would move a side effect, so the fold must
// refuse and say so rather than hoisting it.
func TestNormalizeCollection_RefusesToFoldOverAnActivity(t *testing.T) {
	f := newRejoinFixture()
	f.add("start", &microflows.StartEvent{BaseMicroflowObject: f.base(0)})
	f.add("split1", &microflows.ExclusiveSplit{
		BaseMicroflowObject: f.base(100),
		SplitCondition:      &microflows.ExpressionSplitCondition{Expression: "$A"},
	})
	// A create-object sits on the path to the inner split.
	f.add("create", &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(150)}})
	f.add("split2", &microflows.ExclusiveSplit{
		BaseMicroflowObject: f.base(200),
		SplitCondition:      &microflows.ExpressionSplitCondition{Expression: "$B"},
	})
	f.add("merge1", &microflows.ExclusiveMerge{BaseMicroflowObject: f.base(300)})
	f.add("act", &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(350)}})
	f.add("merge2", &microflows.ExclusiveMerge{BaseMicroflowObject: f.base(400)})
	f.add("end", &microflows.EndEvent{BaseMicroflowObject: f.base(500)})
	f.edge("start", "split1", false)
	f.branch("split1", "create", true)
	f.branch("split1", "merge1", false)
	f.edge("create", "split2", false)
	f.branch("split2", "merge1", true)
	f.branch("split2", "merge2", false)
	f.edge("merge1", "act", false)
	f.edge("act", "merge2", false)
	f.edge("merge2", "end", false)

	_, notes := normalizeCollection(f.col)
	if len(notes) != 1 || !strings.Contains(notes[0], "could not be normalized") {
		t.Fatalf("expected a refusal note, got %v", notes)
	}
	if !strings.Contains(notes[0], "side effect") {
		t.Errorf("the refusal does not say why: %q", notes[0])
	}
}

// A rule-based decision has no expression to fold into, so it is refused rather
// than rendered with an invented condition.
func TestNormalizeCollection_RefusesRuleBasedDecision(t *testing.T) {
	f := reporterGraph("$B")
	for _, o := range f.col.Objects {
		if s, ok := o.(*microflows.ExclusiveSplit); ok {
			if e, ok := s.SplitCondition.(*microflows.ExpressionSplitCondition); ok && e.Expression == "$B" {
				s.SplitCondition = &microflows.RuleSplitCondition{}
			}
		}
	}
	_, notes := normalizeCollection(f.col)
	if len(notes) != 1 || !strings.Contains(notes[0], "rule-based") {
		t.Fatalf("expected a rule-based refusal, got %v", notes)
	}
}

// Folding is deterministic, so DESCRIBE stays a fixed point.
func TestNormalizeCollection_IsDeterministic(t *testing.T) {
	first := ""
	for i := 0; i < 25; i++ {
		got, _ := normalizeCollection(reporterGraph("$B").col)
		var cond string
		for _, o := range got.Objects {
			if s, ok := o.(*microflows.ExclusiveSplit); ok {
				if e, ok := s.SplitCondition.(*microflows.ExpressionSplitCondition); ok {
					cond = e.Expression
				}
			}
		}
		if i == 0 {
			first = cond
			continue
		}
		if cond != first {
			t.Fatalf("run %d folded to %q, first run gave %q", i, cond, first)
		}
	}
}
