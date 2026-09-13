// SPDX-License-Identifier: Apache-2.0

package executor

// DESCRIBE must be able to say where an error path rejoins.
//
// The defect these pin: two graphs with DIFFERENT behaviour described to the
// SAME MDL. Measured on FeedbackModule.SUB_Feedback_SendToServer — repointing
// its error edge from the tail merge to the merge before the AppId split gave
// byte-identical output bar a layout annotation, and executing that output
// reproduced the tail-merge graph in both cases. `mxcli check`, Studio Pro and
// mxbuild were all clean throughout, so nothing else could have caught it.

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// A graph builder small enough to state the topology in the test.
type rejoinFixture struct {
	col *microflows.MicroflowObjectCollection
	ids map[string]model.ID
}

func newRejoinFixture() *rejoinFixture {
	return &rejoinFixture{
		col: &microflows.MicroflowObjectCollection{},
		ids: map[string]model.ID{},
	}
}

func (f *rejoinFixture) add(name string, obj microflows.MicroflowObject) {
	f.ids[name] = obj.GetID()
	f.col.Objects = append(f.col.Objects, obj)
}

func (f *rejoinFixture) base(x int) microflows.BaseMicroflowObject {
	return microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: model.ID(randomTestID())},
		Position:    model.Point{X: x, Y: 100},
	}
}

func (f *rejoinFixture) edge(from, to string, isError bool) {
	f.col.Flows = append(f.col.Flows, &microflows.SequenceFlow{
		BaseElement:    model.BaseElement{ID: model.ID(randomTestID())},
		OriginID:       f.ids[from],
		DestinationID:  f.ids[to],
		IsErrorHandler: isError,
	})
}

var testIDCounter int

func randomTestID() string {
	testIDCounter++
	// Deterministic, and shaped like the 16-byte ids elsewhere so nothing
	// downstream has to special-case a test value.
	return string(rune('a'+testIDCounter%26)) + "0000000-0000-0000-0000-00000000000" +
		string(rune('0'+testIDCounter%10))
}

// call →(error) handler → merge; call →(normal) merge; merge → end.
// The merge is shared, so the handler cannot be described without naming it.
func TestLabelRejoinMerges_SharedMergeIsLabelled(t *testing.T) {
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
	if _, ok := labels.of(f.ids["merge"]); !ok {
		t.Fatal("the shared merge was not labelled; DESCRIBE has no way to name the rejoin")
	}
	if len(labels) != 1 {
		t.Errorf("labelled %d merges, want exactly 1", len(labels))
	}
}

// The control that keeps the labelling narrow. A handler that terminates on its
// own end event describes correctly today, and labelling it would churn the
// output of every project that has one.
func TestLabelRejoinMerges_HandlerWithItsOwnEndIsNotLabelled(t *testing.T) {
	f := newRejoinFixture()
	f.add("start", &microflows.StartEvent{BaseMicroflowObject: f.base(0)})
	f.add("call", &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(100)}})
	f.add("handler", &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(100)}})
	f.add("errEnd", &microflows.EndEvent{BaseMicroflowObject: f.base(200)})
	f.add("end", &microflows.EndEvent{BaseMicroflowObject: f.base(300)})
	f.edge("start", "call", false)
	f.edge("call", "end", false)
	f.edge("call", "handler", true)
	f.edge("handler", "errEnd", false)

	if got := len(labelRejoinMerges(f.col)); got != 0 {
		t.Errorf("labelled %d merges in a graph with none to share", got)
	}
}

// A microflow with no error handling at all must not be touched — the narrowest
// statement that this does not relabel every graph in a project.
func TestLabelRejoinMerges_NoErrorHandlerMeansNoLabels(t *testing.T) {
	f := newRejoinFixture()
	f.add("start", &microflows.StartEvent{BaseMicroflowObject: f.base(0)})
	f.add("split", &microflows.ExclusiveSplit{BaseMicroflowObject: f.base(100)})
	f.add("merge", &microflows.ExclusiveMerge{BaseMicroflowObject: f.base(200)})
	f.add("end", &microflows.EndEvent{BaseMicroflowObject: f.base(300)})
	f.edge("start", "split", false)
	f.edge("split", "merge", false)
	f.edge("merge", "end", false)

	if got := len(labelRejoinMerges(f.col)); got != 0 {
		t.Errorf("labelled %d merges in a graph with no error handler", got)
	}
}

// Labels must be a function of the graph, not of map iteration order: an
// unstable label turns every re-describe of an unchanged microflow into a diff.
func TestLabelRejoinMerges_LabelsAreStableAcrossRuns(t *testing.T) {
	build := func() (*rejoinFixture, mergeLabels) {
		f := newRejoinFixture()
		f.add("start", &microflows.StartEvent{BaseMicroflowObject: f.base(0)})
		f.add("call1", &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(100)}})
		f.add("m1", &microflows.ExclusiveMerge{BaseMicroflowObject: f.base(150)})
		f.add("call2", &microflows.ActionActivity{BaseActivity: microflows.BaseActivity{BaseMicroflowObject: f.base(200)}})
		f.add("m2", &microflows.ExclusiveMerge{BaseMicroflowObject: f.base(250)})
		f.add("end", &microflows.EndEvent{BaseMicroflowObject: f.base(300)})
		f.edge("start", "call1", false)
		f.edge("call1", "m1", false)
		f.edge("call1", "m1", true)
		f.edge("m1", "call2", false)
		f.edge("call2", "m2", false)
		f.edge("call2", "m2", true)
		f.edge("m2", "end", false)
		return f, labelRejoinMerges(f.col)
	}
	for i := 0; i < 20; i++ {
		f, labels := build()
		if got, _ := labels.of(f.ids["m1"]); got != "rejoin1" {
			t.Fatalf("run %d: leftmost merge labelled %q, want rejoin1", i, got)
		}
		if got, _ := labels.of(f.ids["m2"]); got != "rejoin2" {
			t.Fatalf("run %d: rightmost merge labelled %q, want rejoin2", i, got)
		}
	}
}
