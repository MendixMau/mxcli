# Microflows and Nanoflows

Microflows and nanoflows are the primary logic constructs in Mendix applications. They define executable sequences of activities -- retrieving data, creating objects, calling services, showing pages, and more. In MDL, you create and manage them with declarative SQL-like syntax.

## What is a Microflow?

A microflow is a server-side logic flow. It executes on the Mendix runtime, has access to the database, can call external services, and supports transactions with error handling. Microflows are the workhorse of Mendix application logic.

Typical uses:
- CRUD operations (create, read, update, delete objects)
- Validation logic before saving
- Calling external web services or Java actions
- Batch processing and data transformations
- Scheduled event handlers

## What is a Nanoflow?

A nanoflow is a client-side logic flow. It executes in the user's browser (or on mobile devices), providing fast response times without server round-trips. Nanoflows have a more limited set of available activities -- they cannot access the database directly or use Java actions.

Typical uses:
- Client-side validation
- Showing/closing pages
- Changing objects already in memory
- Calling nanoflows or microflows

## Basic Syntax

Both microflows and nanoflows follow the same structural pattern in MDL:

```sql
CREATE MICROFLOW Module.ActionName
FOLDER 'OptionalFolder'
BEGIN
  -- parameters, variables, activities, return
END;
```

```sql
CREATE NANOFLOW Module.ClientAction
BEGIN
  -- activities (client-side subset)
END;
```

## SHOW and DESCRIBE

List microflows and nanoflows in the project:

```sql
SHOW MICROFLOWS
SHOW MICROFLOWS IN MyModule
SHOW NANOFLOWS
SHOW NANOFLOWS IN MyModule
```

View the full MDL definition of an existing microflow or nanoflow (round-trippable output):

```sql
DESCRIBE MICROFLOW MyModule.ACT_CreateOrder
DESCRIBE NANOFLOW MyModule.NAV_ShowDetails
```

## DROP

Remove a microflow or nanoflow:

```sql
DROP MICROFLOW MyModule.ACT_CreateOrder;
DROP NANOFLOW MyModule.NAV_ShowDetails;
```

## OR REPLACE

Use `CREATE OR REPLACE` to overwrite an existing microflow or nanoflow if it already exists, or create it if it does not:

```sql
CREATE OR REPLACE MICROFLOW MyModule.ACT_CreateOrder
BEGIN
  -- updated logic
END;
```

### What a rewrite keeps

A rewrite rebuilds the document from your statement, so a fair question is what
happens to everything the statement does not mention. Properties MDL cannot
express are **carried over**, not reset:

| Property | Studio Pro calls it |
|---|---|
| `Url`, `UrlSearchParameters` | **URL** — the deep link (Mendix 10.6+), e.g. `item/{Key}` |
| `ExportLevel` | **Export level** — `Hidden` or `API` |
| `AllowConcurrentExecution` + its error message / microflow | **Disallow concurrent execution** |
| `MarkAsUsed` | **Mark as used** |

Annotations you *can* write follow the same rule in a stricter form: an absent
`@excluded` or `@applyentityaccess` means "the script does not say", so it
preserves what is stored. Turning one off is explicit — `@applyentityaccess(false)`.

This matters because nothing would tell you otherwise. A microflow with no deep
link, a hidden microflow and a microflow allowing concurrency are all perfectly
valid documents, so `mxcli check`, `mx check` and mxbuild all report success
either way — the loss would only be visible in Studio Pro.

Two ways to lose them anyway, both by design:

- **`DROP` then `CREATE`** is a new document. There is nothing to preserve from.
- **`DESCRIBE` → rename → `exec`** copies the body into a *different* microflow,
  which likewise starts with none of these. `DESCRIBE` flags the ones it cannot
  re-emit as comments, so the gap is visible in its output:

  ```
  -- URL: item/{Key}  (deep link; MDL cannot author one. Kept when this
     microflow is rewritten, NOT copied to a new one — set it in Studio Pro.)
  ```

Use `CREATE OR MODIFY` to edit a microflow that carries any of them.

## Folder Organization

Place microflows and nanoflows into folders for project organization:

```sql
CREATE MICROFLOW Sales.ACT_CreateOrder
FOLDER 'Orders'
BEGIN
  -- ...
END;
```

Move an existing microflow to a different folder:

```sql
MOVE MICROFLOW Sales.ACT_CreateOrder TO FOLDER 'Orders/Actions';
MOVE NANOFLOW Sales.NAV_ShowDetail TO FOLDER 'Navigation';
```

Nested folders use `/` as the separator. Missing folders are created automatically.

## Quick Example

```sql
CREATE MICROFLOW Sales.ACT_CreateOrder
FOLDER 'Orders'
BEGIN
  DECLARE $Order Sales.Order;
  $Order = CREATE Sales.Order (
    OrderDate = [%CurrentDateTime%],
    Status = 'Draft'
  );
  COMMIT $Order;
  SHOW PAGE Sales.Order_Edit ($Order = $Order);
  RETURN $Order;
END;
```

This microflow creates a new Order object with default values, commits it to the database, opens the edit page, and returns the new order.

## Next Steps

- [Structure](microflow-structure.md) -- parameters, variables, return types, and the full `CREATE MICROFLOW` syntax
- [Activity Types](activity-types.md) -- all available activities (RETRIEVE, CREATE, CHANGE, COMMIT, DELETE, CALL, LOG, etc.)
- [Control Flow](control-flow.md) -- IF/ELSE, LOOP, WHILE, error handling
- [Expressions](expressions.md) -- expression syntax for conditions and calculations
- [Nanoflows vs Microflows](nanoflows.md) -- differences and nanoflow-specific syntax
- [Common Patterns](microflow-patterns.md) -- CRUD, validation, batch processing, and anti-patterns to avoid
