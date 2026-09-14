// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// checkNoDroppedWorkflowConstructs refuses a CREATE OR REPLACE/MODIFY WORKFLOW
// that would delete a stored construct the statement does not restate.
//
// A rewrite rebuilds the workflow from the statement, so anything the script
// does not mention is gone, and nothing signals the loss afterwards: measured on
// the v1 fixture, a stored interrupting timer boundary event and its whole
// handler flow went 1 -> 0 while exec reported "Created workflow" and exit 0
// (issue #948). That is the same shape as a dropped queue binding
// (checkNoQueuedCalls) — guard-don't-drop, ADR-0005.
//
// Boundary events ARE authorable (`boundary event interrupting timer '…' { … }`),
// so a script that restates them is allowed straight through — that is the normal
// way to edit a workflow that has one. Event sub-processes are not authorable in
// MDL at all, so any stored one refuses the rewrite outright.
//
// The stored side is read from the raw unit rather than through the semantic
// model deliberately: the reader is what was blind here in the first place, and a
// guard that shares the reader's blind spot cannot see what it is meant to
// protect. Reading the BSON also covers constructs no engine models yet.
func checkNoDroppedWorkflowConstructs(ctx *ExecContext, workflowID model.ID, qualifiedName string, stmt *ast.CreateWorkflowStmt) error {
	if ctx == nil || ctx.Backend == nil || workflowID == "" {
		return nil
	}
	raw, err := ctx.Backend.GetRawUnit(workflowID)
	if err != nil {
		// An unreadable stored unit is not this guard's business; the rewrite
		// path reports its own errors.
		return nil
	}

	// Constructs MDL cannot express at all. A rebuild writes the default for
	// each — measured on ako/TestApp (11.14.0): an on-created microflow becomes
	// NoEvent, workflow event handlers an empty list, a completion rule
	// Consensus on the first outcome, and describe shows an AI agent task only as
	// a comment — so a rewrite loses them without a word. Refused outright, like
	// an event sub-process, and every reason is listed at once.
	var cannotExpress []string
	if n := countEventSubProcesses(raw); n > 0 {
		cannotExpress = append(cannotExpress, fmt.Sprintf("%d event sub-process(es), which it would delete", n))
	}
	cannotExpress = append(cannotExpress, studioProOnlyWorkflowState(raw)...)
	if len(cannotExpress) > 0 {
		return mdlerrors.NewUnsupported(fmt.Sprintf(
			"workflow %s holds state MDL cannot express, and rewriting it would delete or reset it silently:\n  - %s\n"+
				"  Edit the workflow in Studio Pro, or use ALTER WORKFLOW to change one activity at a time — ALTER edits "+
				"the stored document and keeps what it does not touch.",
			qualifiedName, strings.Join(cannotExpress, "\n  - ")))
	}

	storedBE := countRawBoundaryEvents(raw)
	if storedBE == 0 {
		return nil
	}
	authored := countAuthoredBoundaryEvents(stmt.Activities)
	if authored >= storedBE {
		return nil
	}
	return mdlerrors.NewUnsupported(fmt.Sprintf(
		"workflow %s has %d stored boundary event(s) but this statement declares %d — "+
			"rewriting it would delete the difference, along with each one's handler flow.\n"+
			"  Restate them (`boundary event interrupting timer '<expr>' { … }`), which "+
			"`describe workflow %s` now emits, or use ALTER WORKFLOW to change one activity at a time.",
		qualifiedName, storedBE, authored, qualifiedName))
}

// countRawWorkflowNodes counts BSON sub-documents whose $Type contains the given
// marker. Matching on a substring rather than an exact type is deliberate: the
// three timer boundary-event variants and the two event-sub-process start
// activities all differ by prefix, and a variant added later should be caught by
// the guard rather than slip past it.
// countRawBoundaryEvents counts the boundary events stored in a raw workflow.
//
// It matches a $Type that ENDS in "BoundaryEvent". A substring match also
// counted Workflows$EndOfBoundaryEventPathActivity — the marker that ends every
// boundary path, and which mxcli now writes — so one event read as two and
// `create or modify` refused a workflow whose statement restated it exactly.
func countRawBoundaryEvents(v any) int {
	switch t := v.(type) {
	case map[string]any:
		n := 0
		if s, ok := t["$Type"].(string); ok && strings.HasSuffix(s, "BoundaryEvent") {
			n++
		}
		for k, child := range t {
			if k != "$Type" {
				n += countRawBoundaryEvents(child)
			}
		}
		return n
	case []any:
		n := 0
		for _, e := range t {
			n += countRawBoundaryEvents(e)
		}
		return n
	}
	return 0
}

func countRawWorkflowNodes(v any, marker string) int {
	switch t := v.(type) {
	case map[string]any:
		n := 0
		if s, ok := t["$Type"].(string); ok && strings.Contains(s, marker) {
			n++
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys) // deterministic traversal; the count itself is order-free
		for _, k := range keys {
			if k == "$Type" {
				continue
			}
			n += countRawWorkflowNodes(t[k], marker)
		}
		return n
	case []any:
		n := 0
		for _, e := range t {
			n += countRawWorkflowNodes(e, marker)
		}
		return n
	}
	return 0
}

// countAuthoredBoundaryEvents counts the boundary events the statement declares,
// walking nested flows the same way the rest of workflow validation does.
func countAuthoredBoundaryEvents(activities []ast.WorkflowActivityNode) int {
	n := 0
	walkWorkflowActivities(activities, func(act ast.WorkflowActivityNode) {
		switch a := act.(type) {
		case *ast.WorkflowUserTaskNode:
			n += len(a.BoundaryEvents)
		case *ast.WorkflowCallMicroflowNode:
			n += len(a.BoundaryEvents)
		case *ast.WorkflowWaitForNotificationNode:
			n += len(a.BoundaryEvents)
		}
	})
	return n
}

// studioProOnlyWorkflowState lists what a stored workflow holds that a rebuild
// from MDL would reset: workflow event handlers, AI agent tasks, and per activity
// an on-created microflow or a completion rule other than the one mxcli writes.
func studioProOnlyWorkflowState(raw map[string]any) []string {
	var out []string
	for _, h := range rawList(raw["OnWorkflowEvent"]) {
		if hm, ok := h.(map[string]any); ok {
			microflow := ""
			if mh, ok := hm["MicroflowEventHandler"].(map[string]any); ok {
				microflow, _ = mh["Microflow"].(string)
			}
			out = append(out, fmt.Sprintf("a workflow event handler running %s, which it would delete", orUnnamed(microflow)))
		}
	}
	walkRawDocs(raw, func(d map[string]any) {
		if t, _ := d["$Type"].(string); t == "Workflows$AIAgentTaskActivity" {
			name, _ := d["Name"].(string)
			out = append(out, fmt.Sprintf("AI agent task '%s', which describe shows only as a comment and it would delete", name))
			return
		}
		out = append(out, rawActivityState(d)...)
	})
	return out
}

// rawActivityState lists what one stored activity holds that rebuilding it would
// reset. Both the workflow rewrite and REPLACE ACTIVITY ask.
func rawActivityState(d map[string]any) []string {
	var out []string
	name, _ := d["Name"].(string)
	if ev, ok := d["OnCreatedEvent"].(map[string]any); ok {
		if t, _ := ev["$Type"].(string); t == "Workflows$MicroflowBasedEvent" {
			microflow, _ := ev["Microflow"].(string)
			out = append(out, fmt.Sprintf("user task '%s' runs on-created microflow %s, which it would reset to none", name, orUnnamed(microflow)))
		}
	}
	cc, ok := d["CompletionCriteria"].(map[string]any)
	if !ok {
		return out
	}
	kind, _ := cc["$Type"].(string)
	var outcomes []map[string]any
	for _, o := range rawList(d["Outcomes"]) {
		if om, ok := o.(map[string]any); ok {
			outcomes = append(outcomes, om)
		}
	}
	switch kind {
	case "Workflows$ConsensusCompletionCriteria":
		// What mxcli writes: Consensus falling back to the first outcome.
		if len(outcomes) > 0 && reflect.DeepEqual(cc["FallbackOutcomePointer"], outcomes[0]["$ID"]) {
			return out
		}
		fallback := "another outcome"
		for _, o := range outcomes {
			if reflect.DeepEqual(cc["FallbackOutcomePointer"], o["$ID"]) {
				fallback = "outcome '" + rawOutcomeLabel(o) + "'"
			}
		}
		out = append(out, fmt.Sprintf("multi user task '%s' falls back to %s when consensus fails, which it would move to the first outcome", name, fallback))
	default:
		rule := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(kind, "Workflows$"), "CompletionCriteria"))
		out = append(out, fmt.Sprintf("multi user task '%s' decides by %s, which it would reset to consensus", name, rule))
	}
	return out
}

// validateAlterReplaceKeepsStudioProState refuses REPLACE ACTIVITY on an activity
// whose stored document holds state the replacement is rebuilt without. Measured
// on ako/TestApp: replacing a user task with an identical one reset its
// on-created microflow to NoEvent while exec reported "Altered workflow"; SET
// ACTIVITY, which edits the stored document in place, kept it.
func validateAlterReplaceKeepsStudioProState(ctx *ExecContext, s *ast.AlterWorkflowStmt) []string {
	var replaces []*ast.ReplaceActivityOp
	for _, op := range s.Operations {
		if o, ok := op.(*ast.ReplaceActivityOp); ok {
			replaces = append(replaces, o)
		}
	}
	if len(replaces) == 0 || ctx == nil || ctx.Backend == nil {
		return nil
	}
	wf := findStoredWorkflow(ctx, s.Name)
	if wf == nil || wf.Flow == nil {
		return nil
	}
	raw, err := ctx.Backend.GetRawUnit(wf.ID)
	if err != nil || raw == nil {
		return nil
	}
	var errs []string
	for _, o := range replaces {
		act := resolveStoredActivity(wf.Flow, o.ActivityRef, o.AtPosition)
		if act == nil {
			continue
		}
		var reasons []string
		walkRawDocs(raw, func(d map[string]any) {
			if n, _ := d["Name"].(string); n != "" && n == act.GetName() {
				reasons = append(reasons, rawActivityState(d)...)
			}
		})
		if len(reasons) > 0 {
			errs = append(errs, fmt.Sprintf(
				"replace activity '%s' is refused: the activity is rebuilt from the statement, and MDL cannot express what it "+
					"holds — %s. Change it with SET ACTIVITY, which edits it in place, or in Studio Pro.",
				o.ActivityRef, strings.Join(reasons, "; ")))
		}
	}
	return errs
}

// walkRawDocs visits every sub-document of a raw unit, keys in sorted order so
// the reasons come out the same way every time.
func walkRawDocs(v any, visit func(map[string]any)) {
	switch t := v.(type) {
	case map[string]any:
		visit(t)
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			walkRawDocs(t[k], visit)
		}
	case []any:
		for _, e := range t {
			walkRawDocs(e, visit)
		}
	}
}

// rawList returns a stored list's elements; a list's first element may be the
// typed-array marker, which is not a map and is skipped by callers.
func rawList(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	return nil
}

func rawOutcomeLabel(o map[string]any) string {
	for _, k := range []string{"Value", "Caption", "Name"} {
		if s, _ := o[k].(string); s != "" {
			return s
		}
	}
	return "?"
}

func orUnnamed(s string) string {
	if s == "" {
		return "(unnamed)"
	}
	return s
}

// countEventSubProcesses counts a workflow's event sub-processes. The substring
// match countRawWorkflowNodes uses also counts each one's start activity
// (…NotificationEventSubProcessStartActivity lives inside it), so the reference
// workflow's single sub-process was reported as two. The exact type is counted;
// start activities stand in only when no sub-process document is present, one
// start per sub-process.
func countEventSubProcesses(raw map[string]any) int {
	subProcesses, starts := 0, 0
	walkRawDocs(raw, func(d map[string]any) {
		t, _ := d["$Type"].(string)
		switch {
		case t == "Workflows$EventSubProcess":
			subProcesses++
		case strings.HasSuffix(t, "EventSubProcessStartActivity"):
			starts++
		}
	})
	if subProcesses > 0 {
		return subProcesses
	}
	return starts
}
