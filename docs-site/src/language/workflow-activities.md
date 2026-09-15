# Activity Types

Workflow activities are the building blocks of a workflow definition. Each activity type serves a different purpose, from waiting for user input to calling microflows or branching execution.

## User Task

A user task pauses the workflow until a user completes it. Each outcome resumes the workflow down a different path.

```sql
USER TASK <name> '<caption>'
  [PAGE <Module>.<Page>]
  [TARGETING MICROFLOW <Module>.<Microflow>]
  [ON CREATED MICROFLOW <Module>.<Microflow>]
  OUTCOMES '<outcome>' { <activities> } ['<outcome>' { <activities> }] ...;
```

| Element | Description |
|---------|-------------|
| `<name>` | Internal activity name |
| `<caption>` | Display label shown to users |
| `PAGE` | The page opened when the user acts on the task |
| `TARGETING MICROFLOW` | Microflow that determines which users see the task |
| `ON CREATED MICROFLOW` | Microflow run when the task is created — for example to assign it. It takes exactly `System.WorkflowUserTask` and the workflow's context entity, in either order (else CE6683), and returns nothing (else CE5012) |
| `OUTCOMES` | Named outcomes, each with a block of follow-up activities |

Example:

```sql
USER TASK ReviewTask 'Review the request'
  PAGE Approval.ReviewPage
  TARGETING MICROFLOW Approval.ACT_GetReviewers
  ON CREATED MICROFLOW Approval.ACT_AssignReviewer
  OUTCOMES 'Approve' {
    CALL MICROFLOW Approval.ACT_Approve;
  } 'Reject' {
    CALL MICROFLOW Approval.ACT_Reject;
    END WORKFLOW;
  };
```

## Call Microflow

Execute a microflow as part of the workflow. Optionally specify a comment and outcomes:

```sql
CALL MICROFLOW <Module>.<Name> [COMMENT '<text>']
  [OUTCOMES '<outcome>' { <activities> } ...];
```

Example:

```sql
CALL MICROFLOW HR.ACT_SendNotification COMMENT 'Notify the applicant';
```

## AI Agent Task

A step that runs an AI agent (Mendix 11.9 or later). It is written like `CALL
MICROFLOW` with `AGENT` added, and takes the same name, comment, parameter
mappings, outcomes and boundary events. The microflow is where the agent is
invoked — build agents with the Studio Pro Agent Editor, or `CREATE AGENT`.

```sql
CALL AGENT MICROFLOW <Module>.<Name> [AS <name>] [COMMENT '<text>']
  [WITH (<Param> = '<expression>', ...)]
  [OUTCOMES <true|false|'Module.Enum.Value'|''> -> { <activities> } ...];
```

Example — branch on the agent's answer:

```sql
CALL AGENT MICROFLOW HR.ACT_ClassifyRequest AS aiAgentTask1 COMMENT 'Classify the request'
  WITH (Request = '$WorkflowContext')
  OUTCOMES true -> {
    USER TASK Expedite 'Expedite the request' PAGE HR.TaskPage OUTCOMES 'Done' { };
  } false -> { };
```

The microflow must take at least one parameter — usually the workflow's context
object — or the build fails with CE1590. Return Boolean or an enumeration to
branch with `OUTCOMES`; return nothing for a single path.

## Call Workflow

Start a sub-workflow:

```sql
CALL WORKFLOW <Module>.<Name> [COMMENT '<text>'];
```

Example:

```sql
CALL WORKFLOW HR.BackgroundCheck COMMENT 'Run background check sub-process';
```

## Decision

Branch the workflow based on a condition. Each outcome contains a block of activities:

```sql
DECISION ['<caption>']
  OUTCOMES '<outcome>' { <activities> } ['<outcome>' { <activities> }] ...;
```

Example:

```sql
DECISION 'Order value over $1000?'
  OUTCOMES 'Yes' {
    USER TASK ManagerApproval 'Manager must approve'
      PAGE Shop.ApprovalPage
      OUTCOMES 'Approved' { } 'Rejected' { END; };
  } 'No' { };
```

## Parallel Split

Execute multiple paths concurrently. The workflow continues after all paths complete:

```sql
PARALLEL SPLIT
  PATH 1 { <activities> }
  PATH 2 { <activities> }
  [PATH 3 { <activities> }] ...;
```

Example:

```sql
PARALLEL SPLIT
  PATH 1 {
    USER TASK LegalReview 'Legal review'
      PAGE Legal.ReviewPage
      OUTCOMES 'Approved' { };
  }
  PATH 2 {
    USER TASK FinanceReview 'Finance review'
      PAGE Finance.ReviewPage
      OUTCOMES 'Approved' { };
  };
```

## Jump To

Jump to a named activity elsewhere in the workflow (creates a loop or skip):

```sql
JUMP TO <activity-name>;
```

Example:

```sql
JUMP TO ReviewTask;
```

## Wait for Timer

Pause the workflow until a timer expression evaluates:

```sql
WAIT FOR TIMER ['<expression>'];
```

Example:

```sql
WAIT FOR TIMER 'addDays([%CurrentDateTime%], 3)';
```

## Wait for Notification

Pause the workflow until an external notification resumes it:

```sql
WAIT FOR NOTIFICATION;
```

## End

Terminate the current workflow path:

```sql
END;
```

Typically used inside an outcome block to stop the workflow after a rejection or cancellation.

## Summary Table

| Activity | Purpose | Pauses Workflow? |
|----------|---------|-----------------|
| `USER TASK` | Wait for human action | Yes |
| `CALL MICROFLOW` | Execute server logic | No |
| `CALL AGENT MICROFLOW` | Run an AI agent step (11.9+) | No |
| `CALL WORKFLOW` | Start sub-workflow | Depends on sub-workflow |
| `DECISION` | Branch on condition | No |
| `PARALLEL SPLIT` | Concurrent execution | Yes (waits for all paths) |
| `JUMP TO` | Go to named activity | No |
| `WAIT FOR TIMER` | Delay execution | Yes |
| `WAIT FOR NOTIFICATION` | Wait for external signal | Yes |
| `END` | Terminate path | N/A |

## See Also

- [Workflows](./workflows.md) -- overview and when to use workflows
- [Workflow Structure](./workflow-structure.md) -- CREATE WORKFLOW syntax
- [Workflow vs Microflow](./workflow-vs-microflow.md) -- choosing between workflows and microflows
