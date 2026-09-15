# Proposal: Workflow constructs only Studio Pro could author

**Status:** Phases 1 and 2 implemented; phases 3–4 designed, awaiting reference documents
**Date:** 2026-09-14

## Problem Statement

A team building Mendix workflows from MDL reported the constructs they still had
to add in Studio Pro after every scripted rewrite. Since #466 mxcli refuses a
rewrite that would lose them (guard-don't-drop, ADR-0005), which keeps the model
safe but leaves the work manual. This proposal makes them authorable, in the order
the team's workflows reference them:

| Phase | Construct | Stored as | Reference |
|-------|-----------|-----------|-----------|
| 1 | on-created microflow of a user task | `OnCreatedEvent` → `Workflows$MicroflowBasedEvent` | ako/TestApp `workflow.Workflow1` |
| 1 | workflow event handlers | `Workflow.OnWorkflowEvent` → `Workflows$WorkflowEventHandler` | ako/TestApp `workflow.Workflow1` |
| 2 | AI agent task | `Workflows$AIAgentTaskActivity` | ako/TestApp `workflow.Workflow1` |
| 3 | multi-user completion rules (majority, threshold, veto, microflow) | `CompletionCriteria` variants | **needed** |
| 4 | event sub-processes, and `notify workflow` targeting one | `Workflow.EventSubProcesses`, `NotifyWorkflowAction.NotifyTarget` | partial (TestApp has one interrupting notification sub-process) |

## Phase 1 — handlers (implemented)

### BSON structure (ako/TestApp, Studio Pro 11.14.0, 0 errors)

```
SingleUserTaskActivity / MultiUserTaskActivity
  OnCreatedEvent: { $Type: "Workflows$MicroflowBasedEvent", Microflow: "workflow.UserTaskEventHandle" }
               or { $Type: "Workflows$NoEvent" }

Workflow
  OnWorkflowEvent: [2,
    { $Type: "Workflows$WorkflowEventHandler",
      Description: "OnAnyEvent",
      Documentation: "",
      EventTypes: [1, "WorkflowCompleted", "WorkflowInitiated", …],   // explicit list, marker 1
      MicroflowEventHandler: { $Type: "Workflows$MicroflowEventHandler", Microflow: "workflow.WorkflowEventHandle" } } ]
```

Studio Pro stores a handler's event types as an **explicit list even when every
box is ticked** — the handler named "OnAnyEvent" stores all 42. There is no "any"
flag.

Before this change both writers hard-coded `NoEvent` and an empty handler list,
and the legacy reader parsed `OnCreatedEvent` as a string (it is a document), so an
on-created microflow read back as none on that engine.

### MDL syntax

```sql
create workflow HR.LeaveApproval
  parameter $Request: HR.LeaveRequest
  on workflow events (UserTaskStarted, UserTaskEnded) microflow HR.ACT_AuditTask as 'Task audit'
  on any workflow event microflow HR.ACT_LogEvent as 'OnAnyEvent'
begin
  user task Review 'Review the request'
    page HR.ReviewPage
    targeting microflow HR.GetReviewers
    on created microflow HR.ACT_AssignReviewer
    outcomes 'Approve' { } 'Reject' { };
end workflow;
```

- Handler clauses follow the other header options, before `begin`, and repeat.
  `as '…'` is the handler's description — how Studio Pro lists handlers.
- `on created microflow` sits after the targeting clauses on both `user task`
  and `multi user task`.
- A named list is written in Studio Pro's order and de-duplicated; names match
  case-insensitively. `describe` emits `on any workflow event` exactly when the
  stored list equals the project version's full set, and otherwise the list (one
  type per line beyond three), so re-executing stores the same list.
- A handler's `Documentation` has no MDL spelling; a rewrite carries it from the
  stored handler with the same microflow and description.

### Measured rules (mxbuild 11.13.0, one task or handler per shape)

| Shape | Verdict |
|-------|---------|
| on created `(WorkflowUserTask, Ctx)`, either order | 0 errors |
| on created `(WorkflowUserTask)`, `(Ctx)`, `()`, `(+ String)`, `(WorkflowUserTask, <specialization>)` | CE6683 |
| on created returning a value | CE5012 |
| handler `(WorkflowEvent, WorkflowRecord, WorkflowActivityRecord)`, any order | 0 errors |
| handler `(WorkflowEvent)`, `()`, `(+ String)`, `(WorkflowEvent, Ctx)` | CE6691 |
| handler with no microflow | CE0113 |
| handler listing all 42 types | 0 errors |
| handler listing an **invented** type | **0 errors** |
| handler with an empty type list | 0 errors |

The invented-type row decides the design: mxbuild does not check type names, so a
misspelt type would be written and never fire. mxcli checks them itself —
**MDL-WF12** without a project, and against the project's version with one.

### Event types per Mendix version

Measured as the type names present in each mxbuild's `Mendix.Modeler.*.dll`:

| Version | Types | Added |
|---------|-------|-------|
| 11.6.0 | 32 | workflow, activity, user-task and boundary-timer events |
| 11.10.0 | 36 | `AIAgentTaskStarted/Ended`, `(Non)InterruptingNotificationEventSubProcessStartExecuted` |
| 11.13.0, 11.14.0 | 42 | `NotificationStarted/Ended`, `(Non)InterruptingNotificationEventExecuted`, `(Non)InterruptingTimerEventSubProcessStartExecuted` |

- `on any workflow event` writes the set of the newest measured point not above
  the project version. Below 11.6.0 it is refused with a hint to name the types:
  no older mxbuild could be measured (Mendix publishes no arm64 10.24 build), and
  a guess would write types the version may not have.
- A named type is refused only where it is **measured absent** (at or below the
  last point before it appeared). Between that point and the one it was seen at,
  the answer is unknown and the author's choice stands.

### Guard changes

A rewrite now writes handlers and on-created microflows, so they move from
"cannot express" to "restated?" — the boundary-event rule: refused when the
statement declares fewer handlers, or fewer on-created microflows, than are
stored. `replace activity` is refused only when the replacement does not restate
the stored on-created microflow. Still refused outright: an event sub-process, a
non-default completion rule, and a handler with an empty type list (no MDL
spelling). (AI agent tasks were refused outright too until phase 2 made them
restatable.)

Running the end-to-end rewrite on both engines found that **none of the workflow
rewrite guards worked on the legacy engine**: its `GetRawUnit` returns arrays as
`primitive.A`, which the guards' `[]any` type switches never matched, so a rewrite
the modelsdk engine refused was written and deleted the handlers. The guards now
normalize the stored unit before walking it, and their tests decode fixtures the
way the legacy engine does.

### Version compatibility

`workflows.event_handlers` min 10.7.0 (`Workflow.onWorkflowEvent` in modelsdk gen)
gates the handler clauses. `onCreatedEvent` is 9.0.5, older than every supported
workflow version. The signature checks run on Mendix 11+, where they are measured.

### Files

| File | Change |
|------|--------|
| `mdl/grammar/domains/MDLWorkflow.g4` | `workflowEventHandlerClause`; `ON CREATED MICROFLOW` on both user-task alternatives |
| `mdl/ast/ast_workflow.go`, `mdl/visitor/visitor_workflow.go` | `EventHandlers`, `OnCreated` |
| `sdk/workflows/workflow.go` | `WorkflowEventHandler`, `Workflow.EventHandlers` |
| `sdk/mpr/writer_workflow.go`, `sdk/mpr/parser_workflow.go` | legacy write/read; `OnCreatedEvent` parse fix |
| `mdl/backend/modelsdk/workflow_write.go`, `workflow_read.go` | modelsdk write/read |
| `mdl/executor/workflow_event_types.go` | measured per-version type table |
| `mdl/executor/validate_workflow_handlers.go` | MDL-WF12, signatures (CE6683, CE5012, CE6691), version checks |
| `mdl/executor/cmd_workflows_write.go`, `cmd_workflows.go` | build and describe |
| `mdl/executor/validate_workflow_rewrite.go` | restated-handler guard |

### Not in phase 1

- `alter workflow` cannot yet add or change a handler or an on-created microflow
  (`set activity … on created microflow …` and header handler operations). A
  rewrite restating them is the path today.

## Phase 2 — AI agent task (implemented)

```sql
call agent microflow HR.InvokeAgent as aiAgentTask1 comment 'Classify the request'
  with (Request = '$WorkflowContext');
```

Stored as `Workflows$AIAgentTaskActivity` with `Caption`, `Name`, `Microflow`,
`BoundaryEvents` (marker 2), `Outcomes` (marker 3, a `VoidConditionOutcome` with its
`Flow`) and `ParameterMappings` (marker 2, `MicroflowCallParameterMapping`) — the
call-microflow shape under a different `$Type` (ako/TestApp, 11.14.0).

Because the shape is identical, the semantic model is `CallMicroflowTask` with an
`IsAgent` flag rather than a new type: every walker, validator, catalog edge,
activity-name rule and ALTER path that handles a call microflow handles an agent
task unchanged, and only the `$Type`, describe and the rewrite guard branch on it.

**Measured** (mxbuild 11.13.0) by writing each shape as a call-microflow activity
and switching only the `$Type`, so the two columns differ in nothing else:

| Agent microflow | call microflow | AI agent task |
|---|---|---|
| `(Ctx)`, mapped | 0 errors | 0 errors |
| `()` — no parameters | 0 errors | **CE1590** "Missing parameter" |
| `(Ctx, String)`, both mapped | 0 errors | 0 errors |
| `(System.Workflow)` | 0 errors | 0 errors |
| returns Boolean, true/false outcomes | 0 errors | 0 errors |
| returns an enumeration, value outcomes | 0 errors | 0 errors |
| interrupting timer boundary event | 0 errors | 0 errors |

So `check` refuses an agent microflow with no parameters, and nothing else new.

**Confirmed in Studio Pro 11.14 over MCP** (ped_get_schema, then a created
document checked with `ped_check_errors` once its error list settled): the agent
task, event handler and on-created shapes mxcli writes are accepted; an on-created
microflow with the handler signature and a handler with the on-created signature
report the CE6683 / CE6691 messages mxbuild reports; an agent task without
parameter mappings is flagged; and an invented event type is refused at create by
the schema's enum of the same 42 names. Details in
`docs/03-development/PED_MCP_CAPABILITIES.md`.

**Version.** `AIAgentTaskActivity` is absent from the 11.6 mxbuild and present
from 11.10; the writer already records that Mendix 11.9 split
`MicroflowBasedActivity` into `CallMicroflowActivity` + `AIAgentTaskActivity`.
`workflows.ai_agent_task` min 11.9.0 gates CREATE and the activities ALTER adds.

**Guard.** A rewrite declaring fewer agent tasks than are stored is refused
(restate with `call agent microflow`, which describe emits); it was refused
outright before, since describe printed an agent task only as a comment.

**Engines.** Written and read by the modelsdk engine. The retiring legacy engine
carries the flag through (same shape, agent `$Type`) rather than silently turning
an agent task into a call microflow; the MCP backend refuses one, since PED's
agent element has not been measured.

## Phase 3 — completion rules

gen carries `ConsensusCompletionCriteria` (FallbackOutcomePointer),
`MajorityCompletionCriteria` (CompletionType Absolute|Relative,
FallbackOutcomePointer), `ThresholdCompletionCriteria` (+ Threshold),
`VetoCompletionCriteria` (VetoOutcomePointer) and `MicroflowCompletionCriteria`
(Microflow). **Needed before syntax is designed:** a Studio Pro workflow with one
multi-user task per rule, including both completion types for majority and
threshold — pointer semantics and defaults cannot be read off gen.

## Phase 4 — event sub-processes and notify targets

`Workflow.EventSubProcesses` (11.8.0) holds `Workflows$EventSubProcess` with a
`Caption`, `Name` and `Flow` whose first activity is an
`(Non)InterruptingNotificationEventSubProcessStartActivity`. Timer starts appear in
11.13's event types but have no gen type yet. `NotifyWorkflowAction.notifyTarget`
(11.7.0) is how a microflow notifies a sub-process start; the MDL `notify workflow`
statement has no target today. **Needed:** a Studio Pro example with a
non-interrupting and a timer sub-process, and a microflow that notifies one.

## Test plan (phase 1)

- Unit: visitor (clause positions with and without targeting), legacy
  serialize/parse, modelsdk write→read round trip with the raw unit's markers,
  the per-version type table, signature rows as mock-backed `check` cases, the
  version checks, MDL-WF12, describe → re-parse → rebuild, the guard's
  restated/unrestated cases.
- Controls: each fix stubbed in turn makes its test fail.
- Doctype: `24-workflow-examples.mdl` part F builds under `mx check` on both
  engines.
- End to end: exec on an 11.13 app, `mx check`, describe → re-exec → identical
  describe; describe of TestApp's `workflow.Workflow1`.

## Open questions

- Whether a 10.x project knows every type in the 11.6 set. Named types are let
  through there; `any` is refused.
- A stored type newer than the table (a future Mendix) is refused by MDL-WF12 on
  re-execute until the table gains that version's point.
