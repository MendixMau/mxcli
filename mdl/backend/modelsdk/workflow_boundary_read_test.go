// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genWf "github.com/mendixlabs/mxcli/modelsdk/gen/workflows"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// Issue #948. The default engine's workflow reader had no boundary-event support
// at all, while the legacy engine's parser has had it all along. Both engines
// WRITE them, so a boundary event mxcli itself had just written read back as
// absent: DESCRIBE rendered nothing, and a describe -> edit -> re-exec round trip
// silently dropped the timer, its handler flow and the jump inside it.
func TestWorkflowRead_BoundaryEventsRoundTrip(t *testing.T) {
	inner := &workflows.CallMicroflowTask{Microflow: "M.ACT_Escalate"}
	inner.Name = "ACT_Escalate"
	jump := &workflows.JumpToActivity{TargetActivity: "Review"}
	jump.Name = "back"

	call := &workflows.CallMicroflowTask{Microflow: "M.ACT_Step"}
	call.Name = "Step"
	call.BoundaryEvents = []*workflows.BoundaryEvent{{
		EventType:  "InterruptingTimer",
		TimerDelay: "addHours([%CurrentDateTime%], 2)",
		Caption:    "escalate",
		Flow:       &workflows.Flow{Activities: []workflows.WorkflowActivity{inner, jump}},
	}}

	got := roundTripWorkflowActivity(t, call)

	rt, ok := got.(*workflows.CallMicroflowTask)
	if !ok {
		t.Fatalf("round-tripped to %T, want *workflows.CallMicroflowTask", got)
	}
	if len(rt.BoundaryEvents) != 1 {
		t.Fatalf("boundary events after round trip = %d, want 1 (the reader dropped it)", len(rt.BoundaryEvents))
	}
	be := rt.BoundaryEvents[0]
	if be.EventType != "InterruptingTimer" {
		t.Errorf("EventType = %q, want InterruptingTimer", be.EventType)
	}
	if be.TimerDelay != "addHours([%CurrentDateTime%], 2)" {
		t.Errorf("TimerDelay = %q, want the delay expression", be.TimerDelay)
	}
	if be.Caption != "escalate" {
		t.Errorf("Caption = %q, want escalate", be.Caption)
	}
	if be.Flow == nil || len(be.Flow.Activities) != 2 {
		t.Fatalf("handler flow = %v, want 2 activities", be.Flow)
	}
	if _, ok := be.Flow.Activities[1].(*workflows.JumpToActivity); !ok {
		t.Errorf("handler activity[1] = %T, want *workflows.JumpToActivity", be.Flow.Activities[1])
	}
}

// A non-interrupting timer is a different $Type and must not be flattened onto
// the interrupting one.
func TestWorkflowRead_NonInterruptingBoundaryEvent(t *testing.T) {
	call := &workflows.CallMicroflowTask{Microflow: "M.ACT_Step"}
	call.Name = "Step"
	call.BoundaryEvents = []*workflows.BoundaryEvent{{
		EventType:  "NonInterruptingTimer",
		TimerDelay: "[%CurrentDateTime%]",
	}}

	rt, ok := roundTripWorkflowActivity(t, call).(*workflows.CallMicroflowTask)
	if !ok || len(rt.BoundaryEvents) != 1 {
		t.Fatalf("boundary event lost")
	}
	if got := rt.BoundaryEvents[0].EventType; got != "NonInterruptingTimer" {
		t.Errorf("EventType = %q, want NonInterruptingTimer", got)
	}
}

// Control: an activity with no boundary events must not gain an empty one.
func TestWorkflowRead_NoBoundaryEventsStaysEmpty(t *testing.T) {
	call := &workflows.CallMicroflowTask{Microflow: "M.ACT_Step"}
	call.Name = "Step"

	rt, ok := roundTripWorkflowActivity(t, call).(*workflows.CallMicroflowTask)
	if !ok {
		t.Fatal("round trip failed")
	}
	if len(rt.BoundaryEvents) != 0 {
		t.Errorf("boundary events = %d, want 0", len(rt.BoundaryEvents))
	}
}

// roundTripWorkflowActivity encodes a semantic activity to gen through a
// workflow, runs it through the codec, and reads it back — the exact write→read
// path DESCRIBE and any CREATE OR REPLACE takes. Mirrors roundTripMicroflow.
func roundTripWorkflowActivity(t *testing.T, act workflows.WorkflowActivity) workflows.WorkflowActivity {
	t.Helper()
	wf := &workflows.Workflow{
		Name:      "WF",
		Parameter: &workflows.WorkflowParameter{EntityRef: "M.Ctx"},
		Flow:      &workflows.Flow{Activities: []workflows.WorkflowActivity{act}},
	}
	raw, err := (&codec.Encoder{}).Encode(workflowToGen(wf))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	dec, ok := el.(*genWf.Workflow)
	if !ok {
		t.Fatalf("decoded %T, want *genWf.Workflow", el)
	}
	f, ok := dec.Flow().(*genWf.Flow)
	if !ok || f == nil {
		t.Fatal("decoded workflow has no flow")
	}
	back := workflowFlowFromGen(f)
	if back == nil || len(back.Activities) == 0 {
		t.Fatal("round trip produced no activities")
	}
	return back.Activities[0]
}

// A USER TASK carries boundary events too, and it is the shape the workflow
// roundtrip integration tests use. It is worth pinning separately because the
// wiring is per-gen-type: genWf.UserTask (the older type) has no
// BoundaryEventsItems accessor at all, so the reader can only reach these
// through SingleUserTaskActivity / MultiUserTaskActivity. This asserts the
// encoder puts a user task somewhere the reader can actually see it.
func TestWorkflowRead_UserTaskBoundaryEvents(t *testing.T) {
	ut := &workflows.UserTask{Page: "M.TaskPage"}
	ut.Name = "Review"
	ut.Caption = "Review"
	ut.Outcomes = []*workflows.UserTaskOutcome{{Value: "Approve"}}
	ut.BoundaryEvents = []*workflows.BoundaryEvent{{
		EventType:  "InterruptingTimer",
		TimerDelay: "${PT1H}",
	}}

	rt, ok := roundTripWorkflowActivity(t, ut).(*workflows.UserTask)
	if !ok {
		t.Fatalf("round-tripped to a different type")
	}
	if len(rt.BoundaryEvents) != 1 {
		t.Fatalf("user task boundary events = %d, want 1", len(rt.BoundaryEvents))
	}
	if got := rt.BoundaryEvents[0].TimerDelay; got != "${PT1H}" {
		t.Errorf("TimerDelay = %q, want ${PT1H}", got)
	}
}

// A wait for notification carries boundary events too, and the default engine
// read them as absent: the typed reader switch had no case for it, so it fell
// through to the untyped "simple" path, which sets only the name and caption.
// DESCRIBE printed `wait for notification x;` with its timers — and any `end
// workflow` inside them — gone, while the legacy engine described them.
// Measured on 11.13.0: both engines write them and mxbuild accepts them.
func TestWorkflowRead_WaitForNotificationBoundaryEvents(t *testing.T) {
	end := &workflows.EndWorkflowActivity{}
	end.Name, end.Caption = "End2", "NoReply"
	wait := &workflows.WaitForNotificationActivity{}
	wait.Name, wait.Caption = "notify1", "Wait for reply"
	wait.BoundaryEvents = []*workflows.BoundaryEvent{
		{
			EventType:  "InterruptingTimer",
			TimerDelay: "addDays([%CurrentDateTime%], 3)",
			Flow:       &workflows.Flow{Activities: []workflows.WorkflowActivity{end}},
		},
		{
			EventType:  "NonInterruptingTimer",
			TimerDelay: "addDays([%CurrentDateTime%], 1)",
		},
	}

	rt, ok := roundTripWorkflowActivity(t, wait).(*workflows.WaitForNotificationActivity)
	if !ok {
		t.Fatalf("round-tripped to %T, want *workflows.WaitForNotificationActivity", roundTripWorkflowActivity(t, wait))
	}
	if rt.Name != "notify1" || rt.Caption != "Wait for reply" {
		t.Errorf("name/caption = %q/%q, want notify1/Wait for reply", rt.Name, rt.Caption)
	}
	if len(rt.BoundaryEvents) != 2 {
		t.Fatalf("boundary events after round trip = %d, want 2 (the reader dropped them)", len(rt.BoundaryEvents))
	}
	if rt.BoundaryEvents[0].EventType != "InterruptingTimer" || rt.BoundaryEvents[1].EventType != "NonInterruptingTimer" {
		t.Errorf("event types = %q, %q", rt.BoundaryEvents[0].EventType, rt.BoundaryEvents[1].EventType)
	}
	if f := rt.BoundaryEvents[0].Flow; f == nil || len(f.Activities) == 0 {
		t.Errorf("the interrupting event's handler flow (with its End) was lost")
	}
}
