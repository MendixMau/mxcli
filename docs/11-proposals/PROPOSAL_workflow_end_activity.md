---
title: End activity for MDL workflows
status: implemented
date: 2026-09-14
---

# Proposal: End activity for MDL workflows

**Status:** Implemented — grammar, builder naming, describe, MDL-WF08..11, the rewrite guard and ALTER, measured end to end. The open questions that remain are below.

## Problem Statement

MDL cannot end a workflow from inside a branch. A user-task outcome, a decision
branch or a boundary-event path either continues into the main flow or jumps
somewhere — there is no way to say *"on Reject, the workflow is over"*.

It is the one structural gap a team building native workflows from MDL reported
after four projects (briefing, 14 Sep 2026): *"ending a branch early is the one
thing an approval workflow cannot say, and the fallthrough is silent at check
time"*. Its consequences in their projects:

- **Silent fallthrough.** An empty outcome block written to stop the instance
  instead rejoins the main flow, and every later task is created for a request
  that was rejected. `mxcli check`, `exec` and `mx check` all pass.
- **Redesigned processes.** "Reject ends the workflow" had to be rebuilt as a
  backward `jump to`, or the workflow split, or the End hand-added in Studio Pro
  after every scripted rewrite.
- **Interrupting boundary events are half-expressible.** Their path must end in
  an End or a jump; only the jump can be written.

The gap is older than it looks. The first workflow grammar (519de885) carried
the note `// workflowEndStmt removed - END activities are implicit and conflict
with END WORKFLOW`, while `ast.WorkflowEndNode` and `buildEndWorkflow` were
written and never wired. The conflict is real — `workflowBody` is shared by the
top-level `BEGIN … END WORKFLOW` and every nested `{ … }` — but it only exists at
the top level, which is the one place an explicit End is never legal (below).

**The read side is broken today, independently of the syntax.** `describe
workflow` skips every `EndWorkflowActivity`, nested ones included, so a Studio
Pro workflow whose `Reject` outcome ends the workflow describes as
`'Reject' { }` — which, re-executed, *falls through*. Measured on both engines.
This is the "DESCRIBE emits MDL that parses and rebuilds a different document"
case from `docs-wiki/bug-patterns/capability-gap-as-parse-error.md`, and it is
why the read side is part of this change rather than a follow-up.

## Measured Platform Rules

Settled before any syntax was designed, by a throwaway spike that placed an
`EndWorkflowActivity` through the existing `buildEndWorkflow` wherever a script
marked one, on Mendix 11.13.0, one workflow per placement, verdict = the literal
`mx check` line. The first ten rows were run on **both** engines with identical
results; rows 11–13 on the default (modelsdk) engine.

| # | End placed … | mxbuild |
|---|---|---|
| 1 | closing a user-task outcome | **0 errors** |
| 2 | closing a boolean decision branch | **0 errors** |
| 3 | closing a call-microflow outcome | **0 errors** |
| 4 | closing an **interrupting** boundary-event path | **0 errors** |
| 12 | closing a user-task outcome nested inside a decision branch | **0 errors** — any depth |
| 5 | closing a **non-interrupting** boundary-event path | **CE1844** "A 'End' cannot be used for a 'Non interrupting timer boundary event'" |
| 6 | inside a parallel-split path | **CE1844** "A 'End' cannot be used for a 'Parallel split'" |
| 13 | in an outcome nested inside a parallel-split path | **CE1844** — reaches through any depth |
| 7 | followed by another activity in the same path | **CE6671** "An end activity can only be placed at the end of a path" + **CE6689** on what follows |
| 10 | in the middle of the main flow | **CE6671** + **CE6689** |
| 9 | as the last main-flow activity | **CE6671** + **CE6689** — mxcli appends the main End after it |
| 11 | the only content of a **single-outcome** user task | **CE1876** (already MDL-WF02) + **CE6689** on the main End |
| 8 | two nested Ends with the default caption | **CE0495** "Duplicate name 'End'" — an mxcli naming defect, not a platform rule |

Read together:

- **Where an End may go:** the last activity of a user-task outcome, a decision
  branch, a call-microflow outcome or an interrupting boundary-event path, at any
  nesting depth — **never** under a parallel split (a path cannot end the whole
  instance while its siblings run; this is Mendix declining BPMN's terminate-end)
  and never on a non-interrupting boundary path.
- **The main flow's End is not authorable** — it is the one Mendix requires, and
  `END WORKFLOW` already *is* it. That is why the closer conflict dissolves.
- **A path that ends does not continue.** Row 11 shows the consequence for the
  builder: when every path of the last main-flow activity ends, the End mxcli
  appends is unreachable (CE6689).

## BSON Structure

No new storage. `Workflows$EndWorkflowActivity` is what both writers already emit
as the main flow's last activity (`execCreateWorkflow` →
`serializeEndWorkflow` / `simpleActivityToGen`), and the spike's nested Ends built
under both engines. No Studio Pro reference with a nested End was available (see
Open Questions), so per the capability-gap pattern the shape is derived from that
proven sibling rather than guessed.

| Field | Value written | Notes |
|---|---|---|
| `$Type` | `Workflows$EndWorkflowActivity` | |
| `$ID` | fresh | |
| `Name` | unique within the workflow | CE0495 otherwise — row 8 |
| `Caption` | `End`, or the author's caption | shown on the canvas |
| base activity fields | as for the main End | `appendActivityBaseFields` / `addActivityBaseFields` |

Placement is the only new thing, and the existing path-marker code already
expects it: `workflows.EndParallelSplitPath` and `EndBoundaryEventPath` do not
append their end-of-path marker after an `EndWorkflowActivity`
(`sdk/workflows/workflow.go`).

## Proposed MDL Syntax

One statement, valid inside any `{ … }` body of a workflow:

```sql
end workflow [comment '<caption>'];
```

```sql
create workflow HR.LeaveApproval
  parameter $Request: HR.LeaveRequest
begin
  decision '$WorkflowContext/IsDuplicate'
    outcomes
      true  -> { end workflow comment 'Duplicate'; }
      false -> { };

  user task Review 'Review the request'
    page HR.ReviewPage
    outcomes
      'Approve' { }
      'Reject' {
        call microflow HR.ACT_NotifyRejected;
        end workflow comment 'Rejected';
      }
    boundary event interrupting timer 'addDays([%CurrentDateTime%], 5)' {
      call microflow HR.ACT_Escalate;
      end workflow comment 'Expired';
    };

  call microflow HR.ACT_Book;
end workflow;
```

Read aloud: *"on Reject, notify, then end workflow."* The same two words close the
body because they mean the same thing — the main flow's End is `end workflow`;
a branch's End is `end workflow` said early.

- **`comment` is the caption**, exactly as on `jump to`, `decision` and the call
  activities. Omitted → `End`.
- **No name slot.** Every other activity takes one because `jump to` resolves by
  name (ako/mxcli#408); an End is never a jump target (CE6681, and MDL-WF05's
  `collectJumpableNames` already excludes it). A unique name is generated.
- **ALTER needs nothing new.** Every insert that takes a body takes the statement:
  `alter workflow HR.LeaveApproval insert outcome 'Withdraw' on Review { end workflow; };`

### What `describe workflow` emits

- A nested `EndWorkflowActivity` → `end workflow;`, with `comment '<caption>'`
  when the caption is not `End`.
- The main flow's final End and the two end-of-path markers stay implicit, as
  today.

### Refusals

Illegal placements parse and are refused by check rules, not by the grammar — a
platform rule reported as a parse error reads as "not implemented", which is the
failure the capability-gap pattern documents. All three are no-project rules in
`ValidateWorkflow`, so plain `mxcli check` catches them, and `exec` refuses a
script they flag.

| Rule | Refuses | Predicts |
|---|---|---|
| **MDL-WF08** | `end workflow` anywhere under a `parallel split`, or under a `non interrupting` boundary-event path — at any depth (measured for both) | CE1844 |
| **MDL-WF09** | a statement after `end workflow` in the same block | CE6671 + CE6689 |
| **MDL-WF10** | an activity whose every path ends — in `end workflow` or `jump to`, also through a nested decision — followed by anything, or last in the main flow; and a main flow ending in a direct `jump to` | CE6689 (+ CE6679 for the direct jump) |
| **MDL-WF11** | `return;` in a workflow body | — (exec refuses too: nothing is built from it) |

A single-outcome user task whose outcome holds only `end workflow` is CE1876 and
CE6689 at once; it is reported once, as MDL-WF02, with advice that fits an End.
ALTER: what the statement shows (an inserted path, an inserted non-interrupting
boundary) is checked without a project; a target that already sits under a split
or a non-interrupting boundary is found in the stored workflow by `check
--references` and exec.

Sample message (MDL-WF08):

```
✗ end workflow inside a parallel split path — a path cannot end the workflow
  while the other paths run; the build fails CE1844 [MDL-WF08]
  → end the workflow after the split, or leave the path with `jump to`
```

The one spelling that stays a parse error is `end workflow;` in the middle of the
top-level body: there it is the closer, and Mendix refuses the placement anyway
(row 10).

### Rejected alternatives

- **`return;`** — `end workflow` *is* the workflow counterpart of a microflow's
  `return`: both are the End event of their flow, placed early in a branch, and
  both must be last on their path. It still reads wrong where it matters: inside a
  `{ }` block, `'Reject' { return; }` reads as "leave this block and rejoin the
  main flow" — exactly the silent fallthrough this feature exists to prevent — and
  a workflow End carries no value and, at the top level, has no caller. So the
  analogy is documented rather than spelled, and `return;` in a workflow body
  parses only so MDL-WF11 can point at `end workflow;` (it is the spelling a
  microflow author, and an LLM, reaches for), with exec refusing it rather than
  dropping it.
- **`end;`** — "end of what" is unclear, it sits one token from the closer, and it
  reads as a microflow block's `end`.
- **`terminate workflow;`** — promises BPMN's terminate-end, which also stops
  parallel siblings; Mendix refuses exactly that (CE1844, rows 6 and 13).
- **`abort workflow`** — already means the microflow action that leaves an
  instance **Aborted**; an End leaves it **Completed**.
- **Grammar-enforced placement** (End only as the last statement of an outcome,
  branch or interrupting boundary body) — simpler, but turns three platform rules
  into `extraneous input` errors.

## Implementation Plan

1. **Grammar.** Add `workflowEndStmt : END WORKFLOW (COMMENT STRING_LITERAL)?` as
   an alternative of the brace body only: `workflowBody` gains it, and the
   top-level `BEGIN … END WORKFLOW` switches to a new `workflowMainBody :
   workflowActivityStmt*`. That keeps every `ctx.WorkflowBody()` accessor for the
   nine brace bodies and moves exactly one call site. `make grammar` must report
   no ambiguity.
2. **Visitor.** Build `ast.WorkflowEndNode{Caption}`; the top-level rule calls the
   same body builder.
3. **Builder naming.** Seed de-duplication with the main End's name before
   `deduplicateActivityNames` runs over user activities, so a nested End is never
   named `End` (row 8). The ALTER path's `wfnames.Dedup` already starts from
   every stored name.
4. **Describer.** Emit nested Ends; keep skipping the main End and path markers.
   Round-trip test against a document built with nested Ends.
5. **Check rules.** MDL-WF08/09/10 on a walker that carries *inside parallel
   split* / *non-interrupting boundary path* context (`walkWorkflowActivities`
   does not). MDL-WF02's suggestion ("move the activities to the main flow") is
   wrong advice when the single outcome holds only `end workflow` — reword it for
   that case. ALTER: `insert path` bodies and non-interrupting `insert boundary
   event` bodies are refused by construction; `insert after` / `replace` into an
   existing path resolve their target's ancestry from the stored workflow.
6. **Docs.** `mxcli syntax workflow end` topic, the `write-workflows` skill, the
   quick reference, and a note in `workflow.jump-to` that an End is not a target.

### Files to modify/create

| File | Change |
|------|--------|
| `mdl/grammar/domains/MDLWorkflow.g4` | `workflowEndStmt`; `workflowMainBody` for the top-level body |
| `mdl/visitor/visitor_workflow.go` | build `WorkflowEndNode`; top-level body call site |
| `mdl/ast/ast_workflow.go` | none — `WorkflowEndNode{Caption}` exists |
| `mdl/executor/cmd_workflows_write.go` | seed name de-duplication with the main End |
| `mdl/executor/cmd_workflows.go` | describe nested Ends |
| `mdl/executor/validate_workflow_end.go` (new) | MDL-WF08/09/10 + context walker |
| `mdl/executor/validate_workflow.go` | wire the rules; MDL-WF02 wording |
| `mdl/executor/validate_workflow_refs.go` | ALTER insert placement |
| `cmd/mxcli/syntax/features_workflow.go` | `workflow.end` topic |
| `.claude/skills/mendix/write-workflows/SKILL.md`, `docs/01-project/MDL_QUICK_REFERENCE.md` | syntax and rules |
| `mdl-examples/doctype-tests/24-workflow-examples.mdl` | End in each legal position, CREATE and ALTER |
| `mdl-examples/bug-tests/workflow-end-*.mdl` | positive fixture + `.fail.mdl` per rule |

No backend interface change: both engines already serialize the type.

## Version Compatibility

`Workflows$EndWorkflowActivity` has existed since workflows did (9.0), and mxcli
writes it on every version today, so **the statement needs no version gate**.

The **placement rules are measured on 11.13 only**, and they are **not
version-gated** — a change from the draft, which proposed gating them like
MDL-WF07. They run in the no-project check, where there is no project version to
gate on, and that is where they are worth most: the fault they catch is the one a
team reported plain `mxcli check` missing. They are graph-shape rules (where an
End may sit, what can be reached) rather than a feature a later Mendix introduced,
so a 9/10 difference is unlikely — but it is unmeasured, and a rule that is wrong
for a version is a false refusal there.

## Test Plan

- **Parser:** `end workflow` with and without `comment` in every brace body; a
  top-level mid-flow `end workflow;` stays a parse error; the closer still parses
  with `;`, `/`, and neither.
- **Builder:** two nested Ends plus the main End get three distinct names — the
  control is row 8's CE0495.
- **Rules:** each of MDL-WF08/09/10 with a stub control, and clean cases for
  every legal row (1–4, 12), including deep nesting.
- **Describe round-trip:** describe → exec → describe is stable for a workflow
  with nested Ends; the pre-fix describer's `'Reject' { }` is the control.
- **Doctype gate:** `TestMxCheck_DoctypeScripts` at 0 errors on both engines.
- **Runtime** (`.claude/skills/verify-in-runtime.md`): the fault being fixed is a
  runtime one, so boot the fixture under `run --local`, complete a task with its
  ending outcome, and assert the instance is **Completed** and that no task of
  a later activity was created. The pre-change fallthrough (empty outcome) is the
  control.

## Open Questions

1. **Studio Pro's naming of nested Ends.** No Studio Pro reference was available;
   its activities are named by type and ordinal (`decision1`, `callMicroflow1`),
   so nested Ends are plausibly `end1`, `end2`. The name carries no references
   (an End is not a jump target), so only describe/exec *identity* depends on it:
   a re-executed workflow may rename its Ends. Settle with any Studio Pro
   workflow that has one.
2. **Runtime semantics are inferred, not measured.** An End in a branch is
   expected to complete the instance; the runtime test above confirms it.
3. ~~MDL-WF10 for two all-ending outcomes~~ — **settled** before shipping:
   measured CE6689 for two outcomes that both End, a decision whose branches both
   End, outcomes that all jump, End + jump, termination through a nested decision,
   and the main flow's own End when such an activity is last. The same round
   measured End nested under an interrupting boundary (clean) and a
   non-interrupting one (CE1844), and a main flow ending in a direct jump
   (CE6679 + CE6689).
4. **Mendix 9/10 placement rules** are unmeasured (see Version Compatibility).
5. **MCP backend.** `mdl/backend/mcp/workflow.go` strips a trailing End; whether a
   nested End maps through Studio Pro's PED unchanged is unverified. Refuse there
   until checked, per ADR-0004's refuse-don't-drop.
6. **`comment` is the caption, and the keyword says otherwise.** Across every
   workflow activity `comment '…'` writes `Caption` — model data shown on the
   canvas, and for user tasks demonstrably visible at runtime
   (`WorkflowUserTask.Name` holds the caption) — while it reads like a code
   comment, and `describe` emits a derivable caption as a real `-- …` comment.
   `end workflow` follows its siblings rather than introduce a second spelling;
   renaming the keyword to `caption` across all workflow activities (with
   `comment` as a deprecated alias) is a separate change.
7. **Whether an End's caption reaches runtime data** is unmeasured. The runtime
   records End executions (activity type `end`, event `EndEventExecuted`); the
   runtime test should check whether the caption travels with them.
8. **Out of scope:** boundary events on notifications (not in the grammar), event
   sub-processes, and a user task's On-created handler — the briefing's other
   hand-add rows.
