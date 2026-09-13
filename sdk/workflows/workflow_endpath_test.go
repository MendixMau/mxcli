// SPDX-License-Identifier: Apache-2.0

package workflows

import (
	"fmt"
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// A parallel split path stored without Mendix's end-of-path marker runs from the
// split straight to a synthesised end at runtime, skipping everything inside it.
// Measured on Mendix 11.14.0 with the marker as the only variable between two
// instances of one workflow: without it neither path's call-microflow produced an
// activity record; with it both finished (ako/view-entity-examples §6).

func counterIDs() func() model.ID {
	n := 0
	return func() model.ID { n++; return model.ID(fmt.Sprintf("id-%d", n)) }
}

func lastIsEndOfPath(t *testing.T, f *Flow) {
	t.Helper()
	if f == nil || len(f.Activities) == 0 {
		t.Fatal("flow is empty; want it to end with EndOfParallelSplitPathActivity")
	}
	if _, ok := f.Activities[len(f.Activities)-1].(*EndOfParallelSplitPathActivity); !ok {
		t.Fatalf("last activity is %T, want *EndOfParallelSplitPathActivity", f.Activities[len(f.Activities)-1])
	}
}

func TestEndParallelSplitPathAppendsTheMarker(t *testing.T) {
	call := &CallMicroflowTask{}
	f := EndParallelSplitPath(&Flow{Activities: []WorkflowActivity{call}}, counterIDs())
	if len(f.Activities) != 2 || f.Activities[0] != call {
		t.Fatalf("activities = %d, want the call followed by the marker", len(f.Activities))
	}
	lastIsEndOfPath(t, f)
	end := f.Activities[1].(*EndOfParallelSplitPathActivity)
	if end.ID == "" {
		t.Error("the marker needs an id")
	}
	if end.Caption != "End of parallel split path" {
		t.Errorf("caption = %q; the runtime reports this caption in its activity records", end.Caption)
	}
}

// An empty path is stored as a flow holding only the marker.
func TestEndParallelSplitPathGivesAnEmptyPathAFlow(t *testing.T) {
	f := EndParallelSplitPath(nil, counterIDs())
	if f == nil || f.ID == "" {
		t.Fatal("an empty path must get a flow with an id")
	}
	if len(f.Activities) != 1 {
		t.Fatalf("activities = %d, want only the marker", len(f.Activities))
	}
	lastIsEndOfPath(t, f)
}

func TestEndParallelSplitPathIsIdempotent(t *testing.T) {
	ids := counterIDs()
	f := EndParallelSplitPath(&Flow{Activities: []WorkflowActivity{&CallMicroflowTask{}}}, ids)
	again := EndParallelSplitPath(f, ids)
	if len(again.Activities) != 2 {
		t.Fatalf("activities = %d after a second call, want 2", len(again.Activities))
	}
}

// A path that already ends — with a jump or an end-of-workflow — gets no marker,
// which would be unreachable after it.
func TestEndParallelSplitPathLeavesAnEndedPathAlone(t *testing.T) {
	for name, last := range map[string]WorkflowActivity{
		"jump":            &JumpToActivity{},
		"end of workflow": &EndWorkflowActivity{},
	} {
		f := EndParallelSplitPath(&Flow{Activities: []WorkflowActivity{&CallMicroflowTask{}, last}}, counterIDs())
		if len(f.Activities) != 2 {
			t.Errorf("%s: activities = %d, want no marker appended after it", name, len(f.Activities))
		}
	}
}
