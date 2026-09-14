// SPDX-License-Identifier: Apache-2.0

package executor

// A merge the description does not represent is DELETED by describe -> exec,
// and used to be deleted silently.
//
// Measured on Mendix 11.14.0 before this warning existed: a microflow authored
// with two merges — one a 2-input error rejoin, one a 1-input pass-through —
// described to ONE `merge`, and executing that output left the model with one.
// No MDL-FLOW01 (the graph is perfectly reducible), no warning on exec, just
// `Replaced microflow` a node lighter.
//
// The negative controls matter more than the positive one here. This warning
// prints on every DESCRIBE of a real microflow if it is wrong about what
// "represented" means, so the ordinary if/else merge and the labelled rejoin
// each get a test proving they stay quiet.

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The defect. A merge with one incoming path, reached by neither a split nor an
// error handler, is represented by nothing in the emitted MDL.
func TestDroppedMergeWarnings_PassThroughMergeIsFlagged(t *testing.T) {
	f := newRejoinFixture()
	f.add("start", &microflows.StartEvent{BaseMicroflowObject: f.base(0)})
	f.add("act", &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(100)}})
	f.add("merge", &microflows.ExclusiveMerge{BaseMicroflowObject: f.base(200)})
	f.add("end", &microflows.EndEvent{BaseMicroflowObject: f.base(300)})
	f.edge("start", "act", false)
	f.edge("act", "merge", false)
	f.edge("merge", "end", false)

	warnings := droppedMergeWarnings(nil, f.col, labelRejoinMerges(f.col))
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "WARNING") || !strings.Contains(warnings[0], "merge") {
		t.Errorf("warning does not name the problem: %q", warnings[0])
	}
	// The position is what lets someone find the node in Studio Pro.
	if !strings.Contains(warnings[0], "(200, 100)") {
		t.Errorf("warning does not carry the merge's position: %q", warnings[0])
	}
}

// NEGATIVE CONTROL, and the important one: the merge that closes an ordinary
// if/else is represented by `end if`, so it round-trips. Every microflow with a
// decision has one of these, and warning on them would make the warning noise.
func TestDroppedMergeWarnings_IfElseMergeIsNotFlagged(t *testing.T) {
	f := newRejoinFixture()
	f.add("start", &microflows.StartEvent{BaseMicroflowObject: f.base(0)})
	f.add("split", &microflows.ExclusiveSplit{BaseMicroflowObject: f.base(100)})
	f.add("then", &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(150)}})
	f.add("else", &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(150)}})
	f.add("merge", &microflows.ExclusiveMerge{BaseMicroflowObject: f.base(200)})
	f.add("end", &microflows.EndEvent{BaseMicroflowObject: f.base(300)})
	f.edge("start", "split", false)
	f.edge("split", "then", false)
	f.edge("split", "else", false)
	f.edge("then", "merge", false)
	f.edge("else", "merge", false)
	f.edge("merge", "end", false)

	if got := droppedMergeWarnings(nil, f.col, labelRejoinMerges(f.col)); len(got) != 0 {
		t.Errorf("flagged the merge of an ordinary if/else: %v", got)
	}
}

// NEGATIVE CONTROL: a labelled error rejoin is emitted as `merge <label>`, so it
// is represented and must stay quiet.
func TestDroppedMergeWarnings_LabelledRejoinIsNotFlagged(t *testing.T) {
	f := newRejoinFixture()
	f.add("start", &microflows.StartEvent{BaseMicroflowObject: f.base(0)})
	f.add("call", &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(100)}})
	f.add("handler", &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(100)}})
	f.add("merge", &microflows.ExclusiveMerge{BaseMicroflowObject: f.base(200)})
	f.add("end", &microflows.EndEvent{BaseMicroflowObject: f.base(300)})
	f.edge("start", "call", false)
	f.edge("call", "merge", false)
	f.edge("call", "handler", true)
	f.edge("handler", "merge", false)
	f.edge("merge", "end", false)

	labels := labelRejoinMerges(f.col)
	if len(labels) != 1 {
		t.Fatalf("fixture is wrong: expected the rejoin to be labelled, got %d labels", len(labels))
	}
	if got := droppedMergeWarnings(nil, f.col, labels); len(got) != 0 {
		t.Errorf("flagged a merge that DESCRIBE emits as `merge <label>`: %v", got)
	}
}

// A merge inside a loop body is described by the loop's own traversal, which has
// its own object collection. Recursing is what keeps this from reporting every
// in-loop merge as dropped.
func TestDroppedMergeWarnings_RecursesIntoLoopBodies(t *testing.T) {
	inner := &microflows.MicroflowObjectCollection{}
	innerSplit := &microflows.ExclusiveSplit{BaseMicroflowObject: microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: model.ID(randomTestID())}, Position: model.Point{X: 10, Y: 10}}}
	innerA := &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: model.ID(randomTestID())}, Position: model.Point{X: 20, Y: 5}}}}
	innerB := &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: model.ID(randomTestID())}, Position: model.Point{X: 20, Y: 15}}}}
	innerMerge := &microflows.ExclusiveMerge{BaseMicroflowObject: microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: model.ID(randomTestID())}, Position: model.Point{X: 30, Y: 10}}}
	inner.Objects = append(inner.Objects, innerSplit, innerA, innerB, innerMerge)
	edge := func(from, to microflows.MicroflowObject) {
		inner.Flows = append(inner.Flows, &microflows.SequenceFlow{
			BaseElement:   model.BaseElement{ID: model.ID(randomTestID())},
			OriginID:      from.GetID(),
			DestinationID: to.GetID(),
		})
	}
	edge(innerSplit, innerA)
	edge(innerSplit, innerB)
	edge(innerA, innerMerge)
	edge(innerB, innerMerge)

	f := newRejoinFixture()
	f.add("start", &microflows.StartEvent{BaseMicroflowObject: f.base(0)})
	f.add("loop", &microflows.LoopedActivity{
		BaseMicroflowObject: f.base(100),
		ObjectCollection:    inner,
	})
	f.add("end", &microflows.EndEvent{BaseMicroflowObject: f.base(300)})
	f.edge("start", "loop", false)
	f.edge("loop", "end", false)

	if got := droppedMergeWarnings(nil, f.col, labelRejoinMerges(f.col)); len(got) != 0 {
		t.Errorf("flagged an if/else merge inside a loop body: %v", got)
	}
}

// NEGATIVE CONTROL taken from real code, and the one that a purely
// split-join-based rule gets wrong: a split where one branch RETURNS. There is
// then no join common to all branches, so findMergeForSplit pairs nothing, yet
// the merge is still emitted as the continuation after the split closes and
// survives describe → exec with its $ID intact.
//
// Shape of Administration.ManageMyAccount (Administration 4.3.2), measured:
// merge $ID 9b634100… identical before and after the round trip. Flagging it
// would put a warning on ordinary Marketplace code.
func TestDroppedMergeWarnings_SplitWithReturningBranchIsNotFlagged(t *testing.T) {
	f := newRejoinFixture()
	f.add("start", &microflows.StartEvent{BaseMicroflowObject: f.base(0)})
	f.add("split", &microflows.ExclusiveSplit{BaseMicroflowObject: f.base(100)})
	f.add("act", &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(150)}})
	f.add("earlyReturn", &microflows.EndEvent{BaseMicroflowObject: f.base(200)})
	f.add("merge", &microflows.ExclusiveMerge{BaseMicroflowObject: f.base(250)})
	f.add("tail", &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(300)}})
	f.add("end", &microflows.EndEvent{BaseMicroflowObject: f.base(350)})
	f.edge("start", "split", false)
	// One branch terminates, so no join is common to all of them and
	// findMergeForSplit pairs nothing...
	f.edge("split", "act", false)
	f.edge("act", "earlyReturn", false)
	// ...but the other two converge on the merge, which is what keeps it real.
	// ManageMyAccount's merge has in-degree 2 for exactly this reason.
	f.edge("split", "merge", false)
	f.edge("split", "merge", false)
	f.edge("merge", "tail", false)
	f.edge("tail", "end", false)

	// Precondition: the fixture really is the shape that defeats split pairing.
	if got := droppedMergeWarnings(nil, f.col, labelRejoinMerges(f.col)); len(got) != 0 {
		t.Errorf("flagged a merge that survives the round trip: %v", got)
	}
}
