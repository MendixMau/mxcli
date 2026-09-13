// SPDX-License-Identifier: Apache-2.0

package executor

// `merge <label>` / `join <label>` — the graphs they build, and the ones they
// must NOT change.
//
// Every test here goes through the parser rather than a hand-built AST. That is
// not incidental: the CE0709 bug this feature grew out of could not be
// reproduced from a hand-built AST at all, because the two colliding paths only
// meet on the AST the visitor actually produces.

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// graphEdges renders the collection as origin-kind → destination-kind pairs, so
// a test can assert topology without pinning generated IDs.
type edgeKind struct {
	from, to string
	isError  bool
}

func graphEdges(col *microflows.MicroflowObjectCollection) []edgeKind {
	kind := map[model.ID]string{}
	for _, o := range col.Objects {
		switch o.(type) {
		case *microflows.StartEvent:
			kind[o.GetID()] = "start"
		case *microflows.EndEvent:
			kind[o.GetID()] = "end"
		case *microflows.ExclusiveMerge:
			kind[o.GetID()] = "merge"
		case *microflows.ExclusiveSplit:
			kind[o.GetID()] = "split"
		default:
			kind[o.GetID()] = "activity"
		}
	}
	out := make([]edgeKind, 0, len(col.Flows))
	for _, f := range col.Flows {
		if f == nil {
			continue
		}
		out = append(out, edgeKind{kind[f.OriginID], kind[f.DestinationID], f.IsErrorHandler})
	}
	return out
}

func countKind(col *microflows.MicroflowObjectCollection, want string) int {
	n := 0
	for _, e := range graphEdges(col) {
		_ = e
	}
	for _, o := range col.Objects {
		switch o.(type) {
		case *microflows.ExclusiveMerge:
			if want == "merge" {
				n++
			}
		case *microflows.EndEvent:
			if want == "end" {
				n++
			}
		}
	}
	return n
}

func countEdges(col *microflows.MicroflowObjectCollection, from, to string, isError bool) int {
	n := 0
	for _, e := range graphEdges(col) {
		if e.from == from && e.to == to && e.isError == isError {
			n++
		}
	}
	return n
}

// inDegree counts inbound flows per object id.
func inDegree(col *microflows.MicroflowObjectCollection) map[model.ID]int {
	in := map[model.ID]int{}
	for _, f := range col.Flows {
		if f != nil {
			in[f.DestinationID]++
		}
	}
	return in
}

const joinFromErrorHandlerMDL = `create microflow M.Rejoin (Payload: String) returns String
begin
  declare $Status String = 'sent';
  $r = call microflow M.Sub() on error without rollback {
    set $Status = 'degraded';
    join recovered;
  };
  join recovered;

  merge recovered;
  return $Status;
end;`

// THE case: an error path that rejoins the normal one at a named point. MDL
// could not say this before, and DESCRIBE rendered it as an empty handler that
// re-executed to a different graph.
func TestMergeJoin_ErrorHandlerRejoinsNamedMerge(t *testing.T) {
	col := buildMicroflowFromMDL(t, joinFromErrorHandlerMDL)

	if got := countKind(col, "merge"); got != 1 {
		t.Fatalf("merges = %d, want exactly 1 (both paths join the same label)", got)
	}
	// Both the handler's tail and the normal path must land on it.
	if got := countEdges(col, "activity", "merge", false); got < 2 {
		t.Errorf("normal edges into the merge = %d, want 2 (handler tail + main path)", got)
	}
	if got := countEdges(col, "merge", "end", false); got != 1 {
		t.Errorf("merge → end edges = %d, want 1", got)
	}
	// The error edge itself must survive as an error edge, not be flattened
	// into an ordinary one.
	if got := countEdges(col, "activity", "activity", true); got != 1 {
		t.Errorf("error-handler edges = %d, want 1", got)
	}
}

const backwardJoinMDL = `create microflow M.Retry (Payload: String) returns String
begin
  merge attempt;
  $r = call microflow M.Sub() on error without rollback {
    join attempt;
  };
  return $r;
end;`

// A backward reference is what makes a retry loop expressible. It also pins that
// resolution is a post-pass: the merge exists before the join here, the forward
// case above has it after, and both must work.
func TestMergeJoin_BackwardReferenceBuildsRetryLoop(t *testing.T) {
	col := buildMicroflowFromMDL(t, backwardJoinMDL)

	if got := countKind(col, "merge"); got != 1 {
		t.Fatalf("merges = %d, want 1", got)
	}
	if got := countEdges(col, "start", "merge", false); got != 1 {
		t.Errorf("start → merge = %d, want 1 (the merge precedes the activity)", got)
	}
	if got := countEdges(col, "merge", "activity", false); got != 1 {
		t.Errorf("merge → activity = %d, want 1", got)
	}
	// The loop-back: the handler's error edge reaches the merge that feeds the
	// activity it came from.
	if got := countEdges(col, "activity", "merge", true); got != 1 {
		t.Errorf("error edge → merge = %d, want 1 (the retry back-edge)", got)
	}
}

const crossedBranchesMDL = `create microflow M.Crossed () returns Boolean
begin
  if 1 = 1 then
    if 2 = 2 then
      join m1;
    else
      join m2;
    end if;
  else
    join m1;
  end if;

  merge m1;
  log info node 'N' 'shared';
  join m2;

  merge m2;
  return true;
end;`

// The irreducible shape from the proposal: the inner split's TRUE branch and the
// outer split's FALSE branch land on the same point, which no nesting of `if`
// reproduces. Two merges, and NO duplicate edges — a fall-through into a merge
// declaration after an already-terminated path was the first bug here.
func TestMergeJoin_CrossedBranchesBuildTwoMergesWithoutDuplicates(t *testing.T) {
	col := buildMicroflowFromMDL(t, crossedBranchesMDL)

	if got := countKind(col, "merge"); got != 2 {
		t.Fatalf("merges = %d, want 2", got)
	}
	if got := countEdges(col, "split", "merge", false); got != 3 {
		t.Errorf("split → merge edges = %d, want 3 "+
			"(outer false→m1, inner true→m1, inner false→m2)", got)
	}
	if got := countEdges(col, "merge", "activity", false); got != 1 {
		t.Errorf("merge → activity = %d, want 1 (m1 → log)", got)
	}
	if got := countEdges(col, "activity", "merge", false); got != 1 {
		t.Errorf("activity → merge = %d, want 1 (log → m2)", got)
	}

	// No duplicates anywhere: the same (origin, destination) pair twice is the
	// signature of the fall-through bug, and mxbuild does not reject it.
	seen := map[[2]model.ID]int{}
	for _, f := range col.Flows {
		if f != nil {
			seen[[2]model.ID{f.OriginID, f.DestinationID}]++
		}
	}
	for pair, n := range seen {
		if n > 1 {
			t.Errorf("duplicate flow %s → %s emitted %d times", pair[0], pair[1], n)
		}
	}
}

// The controls. Without these the suite would pass equally against a builder
// that sprayed merges everywhere, and against one that broke ordinary graphs.

func TestMergeJoin_PlainMicroflowGainsNoMerge(t *testing.T) {
	col := buildMicroflowFromMDL(t, plainIfMDL)
	if got := countKind(col, "merge"); got != 0 {
		t.Errorf("a plain if/else with no merge/join gained %d merge(s)", got)
	}
}

func TestMergeJoin_FilledErrorHandlerIsUnaffected(t *testing.T) {
	col := buildMicroflowFromMDL(t, filledHandlerMDL)
	if got := countKind(col, "merge"); got != 0 {
		t.Errorf("a handler that terminates on its own gained %d merge(s)", got)
	}
}

// Every end event still takes exactly one inbound flow — the CE0709 invariant.
// A `join` lands on a merge, never on an end event, so this must hold for the
// new graphs too.
func TestMergeJoin_EndEventsStaySinglyConnected(t *testing.T) {
	for name, src := range map[string]string{
		"rejoin":  joinFromErrorHandlerMDL,
		"retry":   backwardJoinMDL,
		"crossed": crossedBranchesMDL,
	} {
		t.Run(name, func(t *testing.T) {
			col := buildMicroflowFromMDL(t, src)
			ends := map[model.ID]bool{}
			for _, o := range col.Objects {
				if _, ok := o.(*microflows.EndEvent); ok {
					ends[o.GetID()] = true
				}
			}
			if len(ends) == 0 {
				t.Fatal("no end event; the assertion would be vacuous")
			}
			for id, n := range inDegree(col) {
				if ends[id] && n > 1 {
					t.Errorf("end event %s has %d inbound flows, want 1 (CE0709)", id, n)
				}
			}
		})
	}
}

// An unresolved label must not ship a half-built graph. mergeForLabel creates
// the merge object on first mention, so a typo would otherwise leave an orphan
// merge in the collection — valid-looking BSON that mxbuild rejects.
func TestMergeJoin_UnresolvedLabelDropsTheOrphanMerge(t *testing.T) {
	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing,
		measurer: &layoutMeasurer{}, varTypes: map[string]string{}}
	fb.pendingJoin = nil
	fb.labels().requested++
	fb.mergeForLabel("typo")
	fb.labels().handled++
	fb.labels().joins = append(fb.labels().joins, joinEdge{origin: model.ID("x"), label: "typo"})

	fb.resolveJoins()

	if got := countKind(&microflows.MicroflowObjectCollection{Objects: fb.objects}, "merge"); got != 0 {
		t.Errorf("orphan merge for an undeclared label was kept (%d in the collection)", got)
	}
	if len(fb.errors) == 0 {
		t.Error("an undeclared label was dropped silently; it must be reported")
	}
}
