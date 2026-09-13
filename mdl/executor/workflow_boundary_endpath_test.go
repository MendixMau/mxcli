// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// Every boundary event path built from MDL must end with Mendix's marker.
// Measured on 11.14.0 with a `call microflow` as the path's last statement:
// interrupting is CE0105 at build; non-interrupting builds at 0 errors and the
// runtime then refuses to start the application — "Expected the flow to end
// with an end event. But it ends with ModelCallMicroflowActivity(…)". The
// shipped write-workflows skill example is exactly that interrupting shape.
func TestBuildBoundaryEventsEndEveryPath(t *testing.T) {
	call := &ast.WorkflowCallMicroflowNode{Microflow: ast.QualifiedName{Module: "M", Name: "ACT_Escalate"}}
	events := buildBoundaryEvents([]ast.WorkflowBoundaryEventNode{
		{EventType: "InterruptingTimer", Delay: "addDays([%CurrentDateTime%], 3)", Activities: []ast.WorkflowActivityNode{call}},
		{EventType: "NonInterruptingTimer", Delay: "addDays([%CurrentDateTime%], 3)", Activities: []ast.WorkflowActivityNode{call}},
		{EventType: "InterruptingTimer", Delay: "addDays([%CurrentDateTime%], 3)"}, // no body
	})
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3", len(events))
	}
	for i, ev := range events {
		if ev.Flow == nil || len(ev.Flow.Activities) == 0 {
			t.Fatalf("event %d (%s) has no flow", i+1, ev.EventType)
		}
		last := ev.Flow.Activities[len(ev.Flow.Activities)-1]
		if _, ok := last.(*workflows.EndOfBoundaryEventPathActivity); !ok {
			t.Errorf("event %d (%s) ends with %T, want *workflows.EndOfBoundaryEventPathActivity", i+1, ev.EventType, last)
		}
	}
}

// The rewrite guard counted every $Type containing "BoundaryEvent", so the
// marker that now ends each boundary path read as a second event and `create
// or modify` refused a statement restating the workflow exactly.
func TestCountRawBoundaryEventsIgnoresTheEndMarker(t *testing.T) {
	raw := map[string]any{
		"$Type": "Workflows$Workflow",
		"Flow": map[string]any{"$Type": "Workflows$Flow", "Activities": []any{
			map[string]any{"$Type": "Workflows$SingleUserTaskActivity", "BoundaryEvents": []any{
				map[string]any{"$Type": "Workflows$InterruptingTimerBoundaryEvent", "Flow": map[string]any{
					"$Type": "Workflows$Flow", "Activities": []any{
						map[string]any{"$Type": "Workflows$CallMicroflowActivity"},
						map[string]any{"$Type": "Workflows$EndOfBoundaryEventPathActivity"},
					},
				}},
				map[string]any{"$Type": "Workflows$NonInterruptingTimerBoundaryEvent"},
			}},
		}},
	}
	if got := countRawBoundaryEvents(raw); got != 2 {
		t.Errorf("countRawBoundaryEvents = %d, want 2 (two events; the end marker is not one)", got)
	}
	if got := countRawWorkflowNodes(raw, "BoundaryEvent"); got != 3 {
		t.Errorf("control: the substring count = %d, want 3 — this is the miscount the guard used to make", got)
	}
}
