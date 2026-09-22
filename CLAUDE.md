# CLAUDE.md

This file provides guidance for Claude Code when working with this repository.

## Welcome — Contributing to mxcli

If you're starting a new task, here's how contributions work in this repo:

1. **File an issue first** — describe the bug or feature before coding. See `CONTRIBUTING.md` for details.
2. **Get approval** — wait for maintainer sign-off before starting work.
3. **Create a feature branch** — `feature/123-short-description` or `fix/456-what-broke`.
4. **Use the contributor commands** to stay on track:
   - `/mxcli-dev:proposal` — create a structured feature proposal (asks the right questions, investigates BSON storage)
   - `/mxcli-dev:review` — review your changes against the PR checklist before pushing
5. **Validate locally** — `make build && make test && make lint` must all pass.
6. **Open a PR** — link the issue, document Mendix Studio Pro validation, confirm agentic testing.

For the full workflow, read `CONTRIBUTING.md`. For the review checklist applied to every PR, see the "PR / Commit Review Checklist" section below.

## Project Overview

**ModelSDK Go** is a Go library for reading and modifying Mendix application projects (`.mpr` files) stored locally on disk. It's a Go-native alternative to the TypeScript-based Mendix Model SDK, enabling programmatic access without cloud connectivity.

## Build & Test Commands

```bash
# build the CLI (preferred - uses Makefile)
make build

# run tests
make test

# format and vet code
make fmt
make vet

# run a specific example
go run ./examples/read_project/main.go /path/to/project.mpr
go run ./examples/modify_project/main.go /path/to/project.mpr

# run the code generator
go run ./cmd/codegen/main.go -reflection-dir ./reference/mendixmodellib/reflection-data -version 10.0.0 -output ./generated/metamodel
```

**Note**: This project uses `modernc.org/sqlite` (pure Go) and does **not** require CGO. No C compiler is needed.

**Note**: The VS Code extension (`vscode-mdl/`) uses **bun**, not npm/node. Use `bun install`, `bun run compile`, etc. The Makefile targets (`make vscode-ext`, `make vscode-install`) already use bun.

## Mendix Tools

The `mx` command-line tool validates and builds Mendix projects. Location depends on environment:

| Environment | Path |
|-------------|------|
| Dev container | `~/.mxcli/mxbuild/{version}/modeler/mx` |
| This repo | `reference/mxbuild/modeler/mx` |

```bash
# Auto-download mxbuild for the project's Mendix version
mxcli setup mxbuild -p app.mpr

# check/validate a Mendix project
mxcli docker check -p /path/to/app.mpr

# or use the integrated command (auto-downloads mxbuild)
mxcli docker check -p app.mpr
```

**Devcontainer gotcha — libSkiaSharp/FreeType crash on some mxbuild releases.** Certain bundled `mx` binaries (observed on 11.10.0) abort with `symbol lookup error: .../libSkiaSharp.so: undefined symbol: FT_Get_BDF_Property`. Root cause: `mx`/mxbuild run under the Temurin JVM, whose bundled libfreetype is stripped and lacks `FT_Get_BDF_Property`, so Skia loads the *JVM's* FreeType instead of the system one (which has the symbol). Preloading the system libfreetype makes it load first and fixes `mx check`/`build`/`run` while keeping Skia working.

`mxcli docker check`/`build`/`new` apply this automatically (`docker.PrepareMxCommand`, which globs the system libfreetype and sets `LD_PRELOAD` on the `mx` child — no-op on non-Linux or when none is found). To invoke a bundled `mx` directly, use the wrapper (same fix) or export `LD_PRELOAD` yourself:

```bash
scripts/mx-check.sh -p /path/to/app.mpr --version 11.10.0
# or, for any mx command:
export LD_PRELOAD=/usr/lib/$(uname -m)-linux-gnu/libfreetype.so.6
```

## Project Architecture

```
ModelSDKGo/
├── modelsdk.go              # Main public api (open, OpenForWriting, helpers)
├── model/                   # Core types: ID, QualifiedName, module, Element interface
│
├── api/                     # High-level fluent api (inspired by Mendix Web Extensibility api)
│   ├── api.go               # ModelAPI entry point with namespace access
│   ├── domainmodels.go      # EntityBuilder, AssociationBuilder, AttributeBuilder
│   ├── enumerations.go      # EnumerationBuilder
│   ├── microflows.go        # MicroflowBuilder
│   ├── pages.go             # PageBuilder, widget builders
│   └── modules.go           # ModulesAPI
│
├── sdk/                     # SDK implementation packages
│   ├── domainmodel/         # entity, attribute, association, DomainModel
│   ├── microflows/          # microflow, nanoflow, activities (60+ types)
│   ├── pages/               # page, layout, widget types (50+ widgets)
│   ├── widgets/             # Embedded widget templates for pluggable widgets
│   │   ├── loader.go        # template loading with go:embed
│   │   └── templates/       # json widget type definitions by Mendix version
│   └── versions/            # per-major feature registry (mendix-{9,10,11}.yaml)
│
├── modelsdk/                # The MPR engine (sdk/mpr, the legacy one, is deleted)
│   ├── mpr/                 # MPR file format: reader, writer, raw unit access
│   ├── codec/               # document <-> BSON encode/decode
│   ├── canon/               # canonical form, identity transplant, write elision
│   ├── gen/                 # vendored metamodel types (see the storage-name note)
│   └── widgets/             # pluggable widget augmentation
│
├── mdl/                     # MDL (Mendix Definition Language) parser & CLI
│   ├── grammar/             # ANTLR4 grammar definition
│   │   ├── MDLLexer.g4      # ANTLR4 lexer grammar (tokens)
│   │   ├── MDLParser.g4     # ANTLR4 parser grammar (rules)
│   │   └── parser/          # Generated Go parser code
│   ├── ast/                 # AST node types for MDL statements
│   ├── visitor/             # ANTLR listener to build AST
│   ├── executor/            # Executes AST against modelsdk-go
│   ├── catalog/             # SQLite-based catalog for querying project metadata
│   ├── linter/              # Extensible linting framework
│   │   └── rules/           # Built-in lint rules (MPR001, MPR002, etc.)
│   └── repl/                # Interactive REPL interface
│
├── sql/                     # external database connectivity (PostgreSQL, Oracle, sql Server)
│   ├── driver.go            # DriverName type, ParseDriver()
│   ├── connection.go        # Manager, connection, credential isolation
│   ├── config.go            # DSN resolution (env vars, YAML config)
│   ├── query.go             # execute() — query via database/sql
│   ├── meta.go              # ShowTables(), DescribeTable() via information_schema
│   ├── format.go            # table and json output formatters
│   ├── mendix.go            # Mendix DB DSN builder, table/column name helpers
│   └── import.go            # import pipeline: batch insert, ID generation, sequence tracking
│
├── cmd/                     # Command-line tools
│   ├── mxcli/               # CLI entry point (Cobra-based)
│   └── codegen/             # Code generator CLI
│
├── internal/                # Internal packages (not exported)
│   └── codegen/             # Metamodel code generation system
│       ├── schema/          # json reflection data loading
│       ├── transform/       # transform to Go types
│       └── emit/            # Go source code generation
│
├── generated/metamodel/     # Auto-generated type definitions
├── examples/                # Usage examples
│
└── reference/               # reference materials (not Go code)
    ├── mendixmodellib/      # TypeScript library + reflection data
    ├── mendixmodelsdk/      # TypeScript SDK reference
    └── mdl-grammar/         # Comprehensive MDL grammar reference
```

## Key Concepts

### MPR File Formats
- **v1**: Single `.mpr` SQLite database file (Mendix < 10.18)
- **v2**: `.mpr` metadata + `mprcontents/` folder with individual documents (Mendix >= 10.18)
- Format detection is automatic

### BSON Storage Names vs Qualified Names

**CRITICAL**: Mendix uses different "storage names" in BSON `$type` fields than the "qualified names" shown in the TypeScript SDK documentation. Using the wrong name causes `TypeCacheUnknownTypeException` when opening in Studio Pro.

| Qualified Name (SDK/docs) | Storage Name (BSON $Type) | Note |
|---------------------------|---------------------------|------|
| CreateObjectAction | CreateChangeAction | |
| ChangeObjectAction | ChangeAction | |
| DeleteObjectAction | DeleteAction | |
| CommitObjectsAction | CommitAction | |
| RollbackObjectAction | RollbackAction | |
| AggregateListAction | AggregateAction | |
| ListOperationAction | ListOperationsAction | |
| ShowPageAction | ShowFormAction | "Form" was original term for "Page" |
| ClosePageAction | CloseFormAction | "Form" was original term for "Page" |

When adding new types, always verify the storage name by:
1. Examining existing MPR files with the `mx` tool or SQLite browser
2. Checking the reflection data in `reference/mendixmodellib/reflection-data/`
3. Looking at the decoder in `modelsdk/codec/` and the types in `modelsdk/gen/microflows/`

**IMPORTANT**: When unsure about the correct BSON structure for a new feature, **ask the user to create a working example in Mendix Studio Pro** so you can compare the generated BSON against a known-good reference.

### Mendix Expression String Escaping

When generating Mendix expression strings (e.g., in `expressionToString()`), single quotes within string literals must be escaped by doubling them: `'it''s here'`. Do NOT use backslash escaping (`\'`). This matches Mendix Studio Pro's expression syntax.

### Quoting Escapes Parser Keywords, Not Platform-Reserved Member Names

The skills advise **quoting all identifiers** to avoid keyword collisions, but this only escapes **MDL parser** keywords (so `"create"`, `"status"`, `"end"` become valid attribute names). It does **not** exempt names the Mendix **platform** reserves for entity members — those are rejected by Studio Pro (and by `mxcli check --references`) **even when quoted**, because the check strips the quotes and validates the bare name:

- `Type` → CE7247 / `MDL021` ("reserved word"). Rename (e.g. `ResourceType`, `TypeValue`). Also `ID`, `GUID`, `CurrentUser`, and the Java-keyword list.
- `CreatedDate` / `ChangedDate` / `Owner` / `ChangedBy` → `MDL020` on persistent entities. Use the `AutoCreatedDate` / `AutoChangedDate` / `AutoOwner` / `AutoChangedBy` pseudo-types for the audit fields, or a different name for an unrelated value.

The reserved-word lists live in `mdl/executor/cmd_enumerations.go` (`mendixReservedWords`, `mendixSystemAttributeNames`). "Always safe to quote" in the skills means *parser*-safe, not *platform*-safe.

### A `GUID` Is the Database's Identity — Never Mint One for an Existing Element

An element's `GUID` is not decorative and is not interchangeable with its `$ID`.
The **runtime keys the database on it**: `mendixsystem$entity.id` and
`mendixsystem$attribute.id` hold the model's `GUID` verbatim (byte-identical once
the .NET field order is undone). Measured on Mendix 11.12.1 against a live
PostgreSQL: changing **only** an entity's `GUID` — same name, same table name,
same attributes — makes the runtime treat it as a different entity and **destroys
its rows**. An unchanged reboot is the control, and preserves them. See
[PROPOSAL_marketplace_module_upgrade.md §8](docs/11-proposals/PROPOSAL_marketplace_module_upgrade.md).

Consequences for any write path:

1. **Preserve the stored `GUID` when rewriting an existing element.** A codec that
   mints a fresh one on rebuild silently drops a table's worth of production data
   on the next deploy — a failure that no `mx check` and no build will catch,
   because the model is perfectly valid. This is the same class as the identity
   properties in `canon.identityFields` and belongs in that decision.
2. **`$ID` renumbering is irrelevant to data safety** — the inverse of the natural
   assumption. Studio Pro renumbers every `$ID` in a module on update (94 of 94)
   and preserves every `GUID` (9 of 9), which is exactly why its update does not
   lose data. `$ID` matters for *intra-unit pointer consistency* (see below);
   `GUID` matters for the database.
3. **A new element must get a fresh `GUID`**, and an element copied from another
   model must not keep the source's — two elements sharing a `GUID` are one entity
   as far as the runtime is concerned.

### The Tunnel Is Linux-Only, On Purpose — Do Not "Restore" It

`run --hub` / `tunnel-hub` embed chisel, which got the Windows and macOS builds
flagged by Defender and denied by enterprise EDR. Linux-only is the fix, not a
portability gap: making it cross-platform again re-introduces the detection for
most downloads. Reasoning and alternatives in
[ADR-0009](docs/13-decisions/0009-tunnel-is-linux-only.md).

Two rules that are not in the ADR:

- **Never obfuscate, pack, or rename to evade detection.** That is attacker
  tradecraft and makes things strictly worse; code signing does not substitute,
  because a signed binary containing chisel is still flagged behaviourally.
- **Every chisel import lives behind one of two seams** (`tunnel_linux.go` /
  `tunnel_other.go`, `control_linux.go` / `control_other.go`). An import anywhere
  else is what `make check-tunnel-deps` exists to catch.

### Theme Files: Where SCSS Actually Compiles

Styling written to the wrong place fails **silently** — the build succeeds and
the rules are simply absent. Which file compiles, in what order, and why a
literal colour outside the palette is wrong under every theme but one:
`.claude/skills/mendix/theme-styling/SKILL.md`.

### Writes Are Conditional, and an `$ID` Is Never Renumbered In Place

Storage does not write a unit whose new content is semantically equal to what is
stored ([ADR-0008](docs/13-decisions/0008-identity-and-idempotence.md)), and when
a write does land the stored element `$ID`s are carried onto it rather than
replaced. Mechanism, measurements and the reporting rules:
[idempotent-writes](docs-site/src/internals/idempotent-writes.md).

Three rules, each already violated once:

1. **Never rewrite an element `$ID` without rewriting every reference to it in the
   same pass.** Pointers are primitive properties holding an `element.ID`, so a
   containment walk never sees one. PR #125 renumbered this way and made projects
   unopenable. A unit is rewritten wholesale or not at all.
2. **A new write path must be wired to `canon.Reconcile`.** One that writes
   directly churns silently while everything else is quiet, so the diff blames the
   wrong change.
3. **A new document type with an identity property needs a row in
   `canon.identityFields`.** It cannot be generated — Mendix's `IsIdentifier` is
   not in the reflection data.

**Any test asserting "nothing changed" must include a control.** Without one it
passes against a build that never had the fix, which is how PR #125 shipped green.

### Association Parent/Child Pointer Semantics (Counter-Intuitive)

**CRITICAL**: Mendix BSON uses inverted naming for association pointers:

| BSON Field | Points To | MDL Keyword |
|------------|-----------|-------------|
| `ParentPointer` | **FROM** entity (FK owner) | `from Module.Child` |
| `ChildPointer` | **TO** entity (referenced) | `to Module.Parent` |

`create association Mod.Child_Parent from Mod.Child to Mod.Parent` stores:
- `ParentPointer = Child.$ID` (the FROM entity owns the foreign key)
- `ChildPointer = Parent.$ID` (the TO entity is being referenced)

This affects **entity access rules**: MemberAccess entries for associations must only be added to the **FROM** entity (the one stored in `ParentPointer`). Adding them to the TO entity triggers CE0066 "Entity access is out of date".

The same convention applies in `domainmodel.Association`: `ParentID` = FROM entity, `ChildID` = TO entity.


## Code Style Guidelines

- Follow standard Go conventions (`go fmt`, `go vet`)
- Use descriptive names matching Mendix terminology
- Keep BSON/JSON tags consistent with Mendix serialization format
- Export types that should be part of the public API
- Use interfaces for polymorphic types (e.g., `Element`, `MicroflowObject`)

## Documentation Artifacts

mxcli uses a layered documentation system — each artifact type has a single canonical home. If a value can change without anyone touching the artifact, it does not belong there; link to the canonical home instead.

| Artifact | Lives in | Created via | Purpose |
|----------|----------|-------------|---------|
| PRD / feature proposal | `docs/11-proposals/` | `/mxcli-dev:proposal` | What to build and why |
| Bug report | `docs/12-bug-reports/` | — | Reproduction + diagnosis |
| ADR | `docs/13-decisions/` | `/mxcli-dev:adr-new` | Cross-cutting decisions (immutable audit trail) |
| User manual | `docs-site/src/` | hand-edited | How to use mxcli / MDL |
| Concept wiki | `docs-wiki/` | `/mxcli-dev:wiki-sync` | Synthesized brain — framing and connecting only |
| Skill | `.claude/skills/` | hand-edited | Step-by-step procedure for a recurring task |
| Bug findings | `.claude/skills/fix-issue/findings/*.jsonl` | append on every bug fix | Bug symptom → file → fix recipe (evidence; grep or DuckDB, never read whole) |
| Load-bearing rule | this file | hand-edited | Always-in-context invariants and routing |

**State stays in its native home.** Proposal status, PR / issue numbers, roadmap, version registries — these live only where they're authoritative (proposal frontmatter, GitHub, the `sdk/versions/*.yaml` files). The wiki and CLAUDE.md may cite this state but never mirror it.

**ADRs are immutable once accepted.** Supersede with a new ADR rather than editing in place. Conventions and template in [`docs/13-decisions/README.md`](docs/13-decisions/README.md).

**Bug findings are read in the opposite order from how they are written.** A fix *appends* one record to `.claude/skills/fix-issue/findings/<area>.jsonl`; a diagnosis *starts* at `docs-wiki/bug-patterns/`, which digests those records into failure classes, and drills into the findings only for the specific instance. The findings are append-only evidence — grep them, or query them with DuckDB (`select … from 'findings/*.jsonl'`), never read them whole. Coverage is **computed, never quoted**: `make digest-status` reports pattern pages, findings, and per-area coverage. A figure written into prose here is stale the next time anyone appends a finding — the previous version of this sentence claimed 83% for an area that had since doubled. A pattern miss means "not yet digested", not "not seen before".

**The wiki is synthesized, not stated.** It frames and connects across the other artifacts — it never restates content that has a canonical home. Rules and seed page list in [`.claude/skills/maintain-wiki.md`](.claude/skills/maintain-wiki.md).

## PR / Commit Review Checklist

When reviewing pull requests or validating work before commit, verify these items:

### Bug fixes
- [ ] **Fix-issue skill consulted** — start at [`docs-wiki/bug-patterns/`](docs-wiki/bug-patterns/) for the failure *class*, then `grep -i` `.claude/skills/fix-issue/findings/*.jsonl` for the *instance*; match before opening files. A pattern-page miss means the finding has not been digested yet, never that it has not been seen
- [ ] **Finding recorded** — one JSON line appended to `.claude/skills/fix-issue/findings/<area>.jsonl` if the symptom is not already covered, and `make check-findings` passes (it prints how far `docs-wiki/bug-patterns/` has fallen behind; `make digest-status` breaks it down by area). **If the class of failure keeps recurring, sync its pattern page** — the digest is on-demand and nothing else asks for it, which is how it went three months without a sync. Write the *insight* (what would have made it cheaper to find, what measurement settled it), not the changelog. `merge=union` in `.gitattributes` keeps both sides when two fixes append at once; order carries no meaning, since these are looked up by matching a symptom
- [ ] **Test written first** — failing test exists before implementation (codec/parser test in `modelsdk/codec/` or `modelsdk/mpr/`, backend mutation test in `mdl/backend/modelsdk/`, executor handler test in `mdl/executor/` using `MockBackend`)
- [ ] **Verified at the layer the symptom lives in** — a test proves something about the layer it exercises and nothing more. Parser/grammar → unit test. BSON we write → unit test on the encoded document. Files on disk after `mx` runs → integration test (`-tags integration`). **The rendered app's behaviour or appearance → `.claude/skills/verify-in-runtime.md`** (boot with `run --local`, assert in Playwright). A page can serialize to valid-looking BSON, pass `mx check`, build cleanly, and still render wrong — that was #812.
- [ ] **Fix proven to be the cause** — revert the fix (or stub the guard) and confirm the test fails with the reported symptom. A test that only passes against fixed code has not been shown to detect anything; two bugs this week had a green suite while live (#812 a clobbered `RegisterTypeDefaults`, #808 an integration test that had only ever skipped)

### Overlap & duplication
- [ ] Check `docs/11-proposals/` for existing proposals covering the same functionality
- [ ] Search the codebase for existing implementations (grep for key function names, command names, types)
- [ ] Check `mdl-examples/doctype-tests/` for existing test coverage of the feature area
- [ ] Verify the PR doesn't re-document already-shipped features as new

### Syntax design for MDL features
New or modified MDL syntax must follow the design guidelines. See [ADR-0003: MDL is SQL-shaped](docs/13-decisions/0003-mdl-is-sql-shaped.md) for the underlying decision and rejected alternatives; the design checklist below operationalises it.
- [ ] **Design skill consulted** — read `.claude/skills/design-mdl-syntax.md` before designing syntax
- [ ] **Follows standard patterns** — uses `create`/`alter`/`drop`/`show`/`describe`, not custom verbs
- [ ] **Reads as English** — a business analyst understands the statement on first reading
- [ ] **Qualified names** — uses `Module.Element` everywhere, no implicit module context
- [ ] **Property format** — uses `( key: value, ... )` with colon separators, one per line
- [ ] **LLM-friendly** — one example is sufficient for an LLM to generate correct variants
- [ ] **Diff-friendly** — adding one property is a one-line diff

### Version compatibility
New features that depend on a specific Mendix version must be version-gated:
- [ ] **Registry entry** — feature added to `sdk/versions/mendix-{9,10,11}.yaml` with correct `min_version`
- [ ] **Executor pre-check** — `checkFeature()` called before BSON writes, with actionable error and hint
- [ ] **Test coverage** — version-gated tests use `-- @version:` directives or `requireMinVersion()`
- [ ] **Skill updated** — `.claude/skills/version-awareness.md` updated if the feature has a workaround for older versions

### Backend abstraction compliance
All executor code must go through the backend abstraction layer. **`sdk/mpr` no longer exists** — the package was deleted once its importer count reached zero, so reaching past the abstraction is now a compile error rather than a rule to remember. See [ADR-0002: Backend Abstraction Layer](docs/13-decisions/0002-backend-abstraction.md) for the context and alternatives. The codec (`modelsdk`) engine is the only local engine — the legacy `sdk/mpr` backend was deleted (`docs/plans/2026-09-14-retire-legacy-engine.md`), and `--engine`/`MXCLI_ENGINE` survive only as a warning-only no-op. It routes **all** document types — domain models included — through the codec, not a codec/legacy hybrid; see [ADR-0004: Full codec engine](docs/13-decisions/0004-full-codec-engine.md). Where the codec path cannot yet reproduce a construct, the backend **refuses** the op rather than dropping data. The backend interface speaks the **semantic model**, not gen/BSON or AST types — gen+codec are the MPR backend's internal storage adapter, one of several (MPR, MCP/PED, a future storage format); see [ADR-0005](docs/13-decisions/0005-semantic-model-interface-currency.md). CREATE is model→gen; fidelity-sensitive ALTER uses backend-internal gen-mutation, not a model round-trip.
- [ ] **No engine internals in the executor** — executor files must not reach into `modelsdk/mpr`, `modelsdk/codec` or `modelsdk/gen` directly; use `ctx.Backend.*` instead. A method missing from the backend gets implemented there, not bypassed
- [ ] **New backend methods on the interface** — any new data access or mutation goes in the appropriate interface in `mdl/backend/` (e.g., `DomainModelBackend`, `MicroflowBackend`), not as a direct SDK call
- [ ] **MPR implementation in `mdl/backend/mpr/`** — the concrete implementation lives here; all BSON/reader/writer logic stays in this package
- [ ] **Mock stub in `mdl/backend/mock/`** — every new backend method has a `Func`-field stub with a descriptive `"MockBackend.X not configured"` error default (not `nil, nil`)
- [ ] **Compile-time interface check** — new backend implementations have `var _ backend.SomeInterface = (*impl)(nil)`
- [ ] **ALTER operations use mutator pattern** — page/workflow mutations go through `ctx.Backend.OpenPageForMutation()` / `OpenWorkflowForMutation()`, not inline BSON construction
- [ ] **New shared types in `mdl/types/`** — a type used by more than one layer goes in `mdl/types/` and the others alias it (`type Foo = types.Foo`), never as duplicate definitions. A same-shape duplicate compiles and tests green; it shows up only as an assignment failure *across* the boundary, naming the same type on both sides of "want". `modelsdk/mpr/version.ProjectVersion` was that case and is now an alias — the guard is a compile-time assertion (`var _ *types.ProjectVersion = (*version.ProjectVersion)(nil)`, `version_alias_test.go`), which builds only under an alias and so is stronger than anything a test body can assert
- [ ] **Map iteration is deterministic** — any map iterated for serialization output must sort keys first (`sort.Strings(keys)` pattern); non-deterministic output causes flaky diffs and BSON instability
- [ ] **Pluggable widgets via WidgetEngine** — new pluggable widget support uses `.def.json` + `WidgetRegistry`; no hardcoded BSON widget builders in the executor

### Full-stack consistency for MDL features
New MDL commands or language features must be wired through the full pipeline:
- [ ] **Grammar** — rule added to `MDLParser.g4` (and `MDLLexer.g4` if new tokens)
- [ ] **Parser regenerated** — `make grammar` run; generated files in `mdl/grammar/parser/` are **not** committed (they are regenerated by `make` at build time)
- [ ] **AST** — node type added in `mdl/ast/`
- [ ] **Visitor** — ANTLR listener bridges parse tree to AST in `mdl/visitor/`
- [ ] **Executor** — thin handler in `mdl/executor/` dispatches to `ctx.Backend.*`; no BSON in the handler
- [ ] **Backend method** — data access or mutation wired through `mdl/backend/` interface and implemented in `mdl/backend/mpr/`
- [ ] **LSP** — if the feature adds formatting, diagnostics, or navigation targets, wire it into `cmd/mxcli/lsp.go` and register the capability
- [ ] **DESCRIBE roundtrip** — if the feature creates artifacts, `describe` should output re-executable MDL
- [ ] **VS Code extension** — if new LSP capabilities are added, update `vscode-mdl/package.json`

### Test coverage
- [ ] New packages have test files
- [ ] New executor commands have MDL examples in `mdl-examples/doctype-tests/`
- [ ] **MDL syntax changes** — any PR that adds or modifies MDL syntax must include working examples in `mdl-examples/doctype-tests/`
- [ ] **Bug fixes** — every bug fix should include an MDL test script in `mdl-examples/bug-tests/` that reproduces the issue, so the fix can be verified in Studio Pro if applicable. **Three numbering namespaces meet in that directory**: the historical files are named after `mendixlabs/mxcli` **PR** numbers (`261-mx9-microflow-roundtrip.mdl` is upstream PR #261), issues filed on the fork are `ako/mxcli` numbers — and the two sequences already collide on 261–266 — while a few names are a **Mendix version** with the dot dropped (`1113-database-query-type-enum.mdl` is Mendix 11.13, not issue 1113). Name a file after a fork issue with a topic prefix (`mapping-261-object-handling-backup.mdl`) and write the reference qualified (`ako/mxcli#261`) wherever it appears, or the number silently resolves to the wrong thing
- [ ] Integration paths (not just helpers) are tested
- [ ] Tests don't rely on `time.Sleep` for synchronization — use channels or polling with timeout

### Security & robustness
- [ ] Unix sockets use restrictive permissions (`os.Chmod(path, 0600)`)
- [ ] File I/O is not in hot paths (event loops, per-keystroke handlers) — cache in memory
- [ ] No silent side effects on typos (e.g., auto-creating resources on misspelled names should be flagged)
- [ ] Method receivers are correct (pointer vs value) for mutations

### Scope & atomicity
- [ ] Each commit does **one thing** — a feature, a bugfix, or a refactor, not a mix
- [ ] Each PR is scoped to a **single feature or concern** — if the description needs "and" between unrelated items, split it
- [ ] Independent features (e.g., a new command, a formatter, UX improvements) go in separate PRs even if developed together
- [ ] Refactors that touch many files (e.g., renaming a helper across executors) are their own commit, not bundled with feature work

### Documentation
- [ ] **Skills** — new features documented in `.claude/skills/` (syntax, examples, gotchas)
- [ ] **CLI help (Cobra)** — `mxcli` subcommand help text updated (Cobra `Short`/`Long`/`Example` fields)
- [ ] **CLI help (syntax topics)** — `cmd/mxcli/syntax/features_*.go` updated with new/changed MDL syntax; new `SyntaxFeature` entries added for new document types; `OR MODIFY` / `OR REPLACE` variants reflected in existing `Syntax` fields; accessible via `mxcli syntax <topic>` and REPL `help`
- [ ] **Syntax reference** — `docs/01-project/MDL_QUICK_REFERENCE.md` updated with new statement syntax
- [ ] **MDL examples** — working examples added to `mdl-examples/` for new commands
- [ ] **Site docs** — `docs-site/src/` pages added or updated for user-facing features

### Code quality
- [ ] Refactors are applied consistently across all relevant files (grep for the old pattern)
- [ ] Manually maintained lists (keyword lists, type mappings) are flagged as maintenance risks
- [ ] Design docs match the actual implementation — remove or update stale plans
- [ ] Numeric type conversions are bounds-checked — `float64→int` casts need overflow guards (`±2^53` for safe integer range); silent overflow produces garbage in serialized output
- [ ] `convert.go` updated when structs in `mdl/types/` gain or lose fields — `TestFieldCountDrift` will catch this at test time, but `convert.go` must be updated before merging

## Dependencies

- `modernc.org/sqlite` - Pure Go SQLite driver (no CGO required)
- `go.mongodb.org/mongo-driver` - BSON parsing for Mendix document format
- `github.com/jackc/pgx/v5` - PostgreSQL driver for external SQL connectivity
- `github.com/sijms/go-ora/v2` - Oracle driver for external SQL connectivity
- `github.com/microsoft/go-mssqldb` - SQL Server driver for external SQL connectivity

## MDL CLI (mxcli)

The `mxcli` command-line tool allows reading and modifying Mendix projects using MDL (Mendix Definition Language), a SQL-like syntax.

```bash
# build the CLI
go build -o bin/mxcli ./cmd/mxcli

# run interactive REPL
./bin/mxcli

# execute commands directly
./bin/mxcli -p /path/to/app.mpr -c "show entities"

# execute MDL script file
./bin/mxcli exec script.mdl -p /path/to/app.mpr

# check MDL syntax (no project needed)
./bin/mxcli check script.mdl

# check syntax and validate references
./bin/mxcli check script.mdl -p app.mpr --references
```

### Key CLI Features

| Feature | Commands | Details |
|---------|----------|---------|
| **Project structure** | `show structure [depth 1\|2\|3] [in module] [all]` | Compact overview at 3 depth levels |
| **Catalog queries** | `show catalog tables`, `select ... from CATALOG.table` | SQL querying of project metadata |
| **Code search** | `show callers\|callees\|references\|impact\|context of ...` | Cross-reference navigation (requires `refresh catalog full`) |
| **Full-text search** | `search 'keyword'` | Search across all strings and source |
| **Linting** | `mxcli lint -p app.mpr [--format json\|sarif]` | 15 built-in rules + 29 Starlark rules (MDL, SEC, QUAL, ARCH, DESIGN, CONV) |
| **Report** | `mxcli report -p app.mpr [--format markdown\|json\|html]` | Scored best practices report with category breakdown |
| **Testing** | `mxcli test tests/ -p app.mpr [--local] [--watch] [--attach]` | `.test.mdl` / `.test.md` files. `--local` runs on mxcli's own runtime (no Docker daemon), on its own ports + `<project>_test` database, driving a **token-guarded test endpoint** (one microflow per test, invoked over HTTP — a throwing test fails only itself, results are returned not log-scraped). `--watch` keeps the runtime warm (~30s first run, then ~2s). `--attach` runs against an app already up under `run --local --test-endpoint` (no boot; uses **that app's** database) |
| **Diff** | `mxcli diff -p app.mpr changes.mdl` | Compare script against project state |
| **Diff local** | `mxcli diff-local -p app.mpr --ref head` | Git diff for MPR v2 projects |
| **Diff revisions** | `mxcli diff-local -p app.mpr --ref main..feature` | Compare two arbitrary git revisions |
| **OQL** | `mxcli oql -p app.mpr "select ..."` | Query running Mendix runtime |
| **Widgets** | `show widgets`, `update widgets set ...`, `mxcli widget describe <id>` | Widget discovery, bulk updates (experimental), and inspecting a widget's discovered properties + dynamic rules |
| **External SQL** | `sql connect`, `sql <alias> select ...`, `mxcli sql` | Direct SQL queries against PostgreSQL, Oracle, SQL Server (credential isolation) |
| **Data import** | `import from <alias> query '...' into Module.Entity map (...)` | Import from external DB into Mendix app PostgreSQL (batch insert with ID generation) |
| **Connector gen** | `sql <alias> generate connector into <module> [tables (...)] [views (...)] [exec]` | Auto-generate Database Connector MDL from discovered schema |
| **Marketplace drift** | `mxcli marketplace diff <id> -p app.mpr [--to V] [--json]` | Which elements of an installed marketplace module have been edited locally, and what an upgrade would overwrite |
| **Model repair** | `mxcli fix widgets`, `mxcli fix design-properties` | Runs `mx update-widgets` / `mx rename-design-properties` and **persists** the result without their MPR v2 → v1 collapse (harvest: let the tool convert, read the units back, restore v2, write the changed ones through mxcli's writer). Clears CE0463 / CE6087 after a headless install — measured 203 → 0 errors on a vanilla 11.12.1 app |
| **Domain-model layout** | `mxcli layout -p app.mpr [--module M] [--dry-run]` | Arranges entities from the **association graph**: an entity referencing nothing is a lookup and goes left, everything else one column past the furthest thing it references, so lines run one way instead of crossing. Unconnected entities (non-persistent helpers) go in a band below rather than among the lookups. Positions are a function of the model, so a second run moves nothing. Replaces hand-arranged positions in the modules it touches — hence opt-in, with `--dry-run`; Marketplace modules and System are skipped. The **default** for an entity with no `@Position` is a wrapping grid (`mdl/dmlayout`), not the single 6,000px row it used to be |
| **Diagnostics** | `mxcli diag [--bundle]` | Session logs, version info, bug report bundles |
| **Loop report** | `mxcli diag loop-report [--json]` | Where a session's mxcli calls went — per-command counts, wall time and runs that did not close — read off the session logs every invocation already writes. Counts mxcli **processes**, not model calls, and says so; output size and `run --local` reloads are not recorded anywhere, so it reports what it cannot answer rather than estimating it |
| **Project brain** | `mxcli brain init\|capture\|staged\|promote\|drop\|check\|show\|plan\|resolve` | Opt-in store in `docs/brain/` for what mxcli **cannot** compute (why a pattern was chosen here, which marketplace version broke what). Sharded by module — an entry's first anchor names its file — so a session loads `project.md` plus the modules it is touching, not the whole store. Also holds the **plan**: requirements grouped into slices, whose anchors point *forward*, so `brain plan` reports progress **derived from the model** rather than from a status column. An agent captures to a git-ignored queue; a person promotes |
| **New project** | `mxcli new <name> --version X.Y.Z [--output-dir dir] [--theme none] [--layout none]` | Downloads mxbuild, creates blank project, applies default styling, scaffolds a project-owned layout, runs init, installs Linux mxcli for devcontainer |
| **Default styling** | `mxcli theme list\|show\|apply\|remove` | Applies a theme (signal/ledger/console) — files under `theme/` only, the model is never touched |
| **Project themes** | `mxcli theme create <name> [--from <theme\|design-file>]` | Scaffolds a theme the project owns into `theme/mxcli-themes/`; `--from <file>` seeds the palette from `--mxt-*` declarations |
| **Theme switching** | `mxcli theme apply <name> --variant auto\|light\|dark`, `mxcli theme switcher install` | `auto` ships both palettes (follows the OS + honours a `theme-light`/`theme-dark` class); `switcher install` adds the JS actions + nanoflow for a user toggle (**this one does write to the model**) |
| **Switchable sets** | `mxcli theme apply signal ledger console` | Several themes in one stylesheet, each palette scoped to `:root.mxt-<name>`; the app picks one with a class on `<html>` — no rebuild, no reload |
| **Setup mxcli** | `mxcli setup mxcli [--os linux] [--arch amd64] [--output ./mxcli]` | Download platform-specific mxcli binary from GitHub releases |

### mxcli new

`mxcli new` creates a complete Mendix project from scratch in one step:

```bash
mxcli new MyApp --version 11.8.0
mxcli new MyApp --version 10.24.0 --output-dir ./projects/my-app
```

Steps performed: downloads MxBuild → `mx create-project` → `mxcli theme apply` → scaffolds `<YourModule>.App_Default` and moves the project's pages onto it (`--layout none` to keep Atlas's) → `mxcli init` → one `mxbuild --target=deploy` run (`--skip-build` to skip) → downloads correct Linux mxcli binary for devcontainer. That build settles the JS/Java action stubs MxBuild rewrites on first build (48 tracked files in a blank 11.12 app), so a fresh clone does not go dirty the first time anyone builds it. The result is a ready-to-open project with `.devcontainer/`, AI tooling, mxcli's default styling, a layout the project owns, and a working `./mxcli` binary. Pass `--theme none` for plain Atlas.

The layout is **not** a copy of Atlas's: every Atlas layout a real app uses carries widgets MDL cannot spell (`Atlas_TopBar` has a `Forms$MenuBar`, a `Forms$SidebarToggleButton` and a pluggable image), so a describe → exec copy renders with no navigation and no logo. It reproduces the *result* instead — same layout class, same region classes, topbar navigation, `Main` for page content.

### Slash Command Namespaces

Commands in `.claude/commands/` are organised by audience:

| Namespace | Folder | Invoked as | Purpose |
|-----------|--------|------------|---------|
| `mendix:` | `.claude/commands/mendix/` | `/mendix:lint` | mxcli **user** commands — synced to Mendix projects via `mxcli init` |
| `mxcli-dev:` | `.claude/commands/mxcli-dev/` | `/mxcli-dev:review` | **Contributor** commands — this repo only, never synced to user projects |

Both namespaces are discoverable by typing `/mxcli` in Claude Code. Add new contributor tooling (review workflows, debugging helpers, etc.) under `mxcli-dev/`. Add commands intended for Mendix project users under `mendix/`.

### mxcli init

`mxcli init` creates a `.claude/` folder with skills, commands, CLAUDE.md, and VS Code MDL extension in a target Mendix project. Source of truth for synced assets:
- Skills: `.claude/skills/mendix/<name>/SKILL.md` — directory-shaped, per the [Agent Skills](https://agentskills.io) standard, with `name` and `description` frontmatter. `make sync-skills` mirrors the tree into the `cmd/mxcli/skills/` embed dir (`//go:embed all:skills`), and `mxcli init` writes it into the project **twice**: `.ai-context/skills/` for every tool, and `.claude/skills/` — the only path Claude Code scans — when the project is set up for Claude. **Edit the `mendix/` source, not the embed dir** (it is regenerated, and the sync is `rsync --delete`). The `description` is the routing mechanism; the table in the generated CLAUDE.md is a shortcut, not the index. Upgrading a project retires the flat `<name>.md` files older mxcli versions wrote, but never a skill the user added. The top-level `.claude/skills/*.md` are contributor/dev skills and are **not** synced.
- Commands: `.claude/commands/mendix/` (the `mxcli-dev/` folder is **not** synced)
- VS Code extension: `vscode-mdl/vscode-mdl-*.vsix`

Build-time sync: `make build` syncs everything automatically. Individual targets: `make sync-skills`, `make sync-commands`, `make sync-vsix`.

### VS Code Extension

The `vscode-mdl` extension provides MDL language support: syntax highlighting, parse/semantic diagnostics, completion, symbols, folding, hover, go-to-definition, clickable terminal links, and context menu commands. The extension spawns `mxcli lsp --stdio` as the language server. Build with `make vscode-ext` (requires bun).

### ANTLR4 Parser

Regenerate after modifying `MDLLexer.g4`, `MDLParser.g4`, or any `domains/*.g4` file: `make grammar`. Generated files in `mdl/grammar/parser/` are **not** committed to git. See `docs/03-development/MDL_PARSER_ARCHITECTURE.md` for design details.

## IMPORTANT: Before Writing MDL Scripts or Working with Data

**Read the relevant skill files FIRST before writing any MDL, seeding data, or doing database/import work:**
- `.claude/skills/version-awareness.md` - **CHECK project version first** - Run `show features` before using version-gated syntax
- `.claude/skills/design-mdl-syntax.md` - **READ before designing new MDL syntax** - Design principles, decision framework, anti-patterns, checklist
- `.claude/skills/write-microflows.md` - Microflow syntax, common mistakes, validation checklist
- `.claude/skills/write-nanoflows.md` - Nanoflow syntax, restrictions, disallowed activities, validation checklist
- `.claude/skills/mendix/project-brain/SKILL.md` - **Project brain** (`mxcli brain`): the opt-in store for what mxcli cannot compute; why anything derivable from the model must never be written there, how anchors route an entry to its shard, and which `check` outcomes are failures
- `.claude/skills/mendix/write-rules.md` - **Rules** (CREATE/LIST/DESCRIBE/DROP/MOVE RULE): a rule returns Boolean or an enumeration and is callable only from a decision; what its body may not contain and the CE numbers behind each refusal; why there is no `grant execute on rule`
- `.claude/skills/write-workflows.md` - **Workflow authoring** (CREATE/DROP/ALTER WORKFLOW): activities (user task, decision, parallel split, jump, wait, boundary events), header options, gotchas. Workflows are authorable, not read-only.
- `.claude/skills/create-page.md` - Page/widget syntax reference
- `.claude/skills/mendix/write-layouts/SKILL.md` - **Layouts** (CREATE/DESCRIBE LAYOUT): the frame a page renders inside — scroll-container regions, the navigation tree, the placeholders pages bind to; why Atlas_Core is refused and why `mainplaceholder:` does not exist
- `.claude/skills/alter-page.md` - ALTER PAGE/SNIPPET in-place modifications (SET, INSERT, DROP, REPLACE, SET Layout)
- `.claude/skills/overview-pages.md` - CRUD page patterns
- `.claude/skills/master-detail-pages.md` - Master-detail page patterns
- `.claude/skills/generate-domain-model.md` - Entity/Association syntax
- `.claude/skills/mendix/scheduled-events-and-queues.md` - **Scheduled events (Mendix's cron) and task queues**: the eight Repeat variants and which fields each one takes, why a queue does NOT throttle a scheduled event, and why rewriting a microflow with a queued call is refused
- `.claude/skills/check-syntax.md` - Pre-flight validation checklist
- `.claude/skills/organize-project.md` - Folders, MOVE command, project structure conventions
- `.claude/skills/manage-security.md` - Security roles, access control, GRANT/REVOKE patterns
- `.claude/skills/manage-navigation.md` - Navigation profiles, home pages, menus, login pages
- `.claude/skills/demo-data.md` - **READ for any database/import work** - Mendix ID system, association storage, demo data insertion
- `.claude/skills/xpath-constraints.md` - XPath syntax in WHERE clauses, association paths, nested predicates, functions
- `.claude/skills/database-connections.md` - External database connections from microflows
- `.claude/skills/test-microflows.md` - **READ for testing work** - Test annotations, file formats, Docker setup requirement

### Mendix Microflow/Nanoflow Idioms (MUST follow)

These rules apply whenever generating microflow or nanoflow MDL. Violations are caught by `mxcli check`.

1. **NEVER create empty list variables as loop sources.** If processing imported data, accept the list as a microflow parameter — `declare $Items list of ... = empty` followed by `loop $item in $Items` is always wrong.
2. **NEVER use nested LOOPs for list matching.** Loop over the primary list and use `$match = FIND($TargetList, key = $item/key)` for an O(N) in-memory lookup. A plain `retrieve … where` **cannot** filter a list variable (only a database/association source), so `retrieve $match from $TargetList where …` is a parse error — use `FIND`/`FILTER`. Nested loops are O(N^2). The `$item` there is the enclosing loop's iterator and stays valid — MDL-LISTOP01 flags a predicate variable that is *not in scope*, not the name. Inside the predicate itself, the item under test is `$currentObject` (a bare attribute name resolves to it).
3. **NEVER nest one list operation inside another.** Each of HEAD/TAIL/FIND/FILTER/SORT/UNION/INTERSECT/SUBTRACT/RANGE and the aggregates is a separate **activity**, and an activity stores its list as a **variable reference** — there is no slot for a nested computation. `$n = COUNT(FILTER($reqs, …))` parses, and used to drop the inner call entirely and write an activity with an empty list: `check` clean, `exec` reporting "Created microflow", then CE0012 / CE0096 at build time — and `sort(filter(…), Attr)` made mxbuild abort outright, because the sort attribute resolves against the now-absent list's entity. One statement each: `$approved = FILTER($reqs, …); $n = COUNT($approved);`. `mxcli check` now refuses the nested form as MDL-LISTOP02 (mendixlabs/mxcli#1101).
4. **Use append logic when merging**, not overwrite: `$Existing/Field + '\n' + $New/Field` inside an `if $New/Field != empty` guard.
5. **`retrieve … limit 1` binds a single OBJECT, not a one-element list** — it is Mendix's "First object" range, so `head()`, `count()` or a `loop` over that variable is **CE0097**. Drop the `limit` for a list; `limit 1 offset n` and every other `limit` ARE lists. Note the same word means the opposite on `import from mapping`, where `first` binds the object and `limit 1` a one-element list. `describe` re-emits `limit 1` either way, so the source of an object retrieve and a list retrieve are identical text and only MDL-RETRIEVE01 distinguishes them before a build (mendixlabs/mxcli#1103).
6. **Read `.claude/skills/patterns-data-processing.md`** for delta merge, batch processing, and list operation patterns.

**Always validate before presenting to user:**
```bash
./bin/mxcli check script.mdl                    # Syntax + anti-pattern check
./bin/mxcli check script.mdl -p app.mpr --references  # With reference validation
```

## MDL Syntax Quick Reference

Full syntax tables for all MDL statements (microflows, pages, security, navigation, settings, business events, ALTER PAGE, reserved words) are in **[docs/01-project/MDL_QUICK_REFERENCE.md](docs/01-project/MDL_QUICK_REFERENCE.md)**.

## What mxcli Can Do

**This file does not list features.** `./bin/mxcli syntax` enumerates every MDL
statement (`--json` for bulk), `./bin/mxcli help <command>` documents each command,
and `./bin/mxcli lint --list-rules` names every rule. A list here is a transcription
of what those answer authoritatively, and it goes stale the next time anything ships.

Per-doctype gotchas, CE numbers and the measurements behind them live in the skill
for that doctype (`.claude/skills/mendix/<name>/SKILL.md`) — loaded when you touch
that area rather than re-read into every session. Design rationale lives in
`docs/11-proposals/`; cross-cutting decisions in `docs/13-decisions/`.

Still absent: 47 of 52 metamodel domains, delta/change tracking, runtime type
reflection.

## Useful Files for Context

- `README.md` - User documentation and API reference
- `api/api.go` - High-level fluent API entry point
- `api/domainmodels.go` - Entity/Association/Attribute builders
- `docs/01-project/SDK_EQUIVALENCE.md` - Detailed comparison with TypeScript SDK, gap analysis
- `modelsdk/codec/decoder.go` - BSON decoding (handles polymorphic types)
- `modelsdk/codec/encoder.go` - BSON encoding
- `mdl/backend/modelsdk/widget_pluggable_write.go` - Pluggable widget BSON, and the v1/v2 BSON driver conversion at the backend boundary
- `sdk/widgets/templates/` - Embedded widget templates for pluggable widgets (ComboBox, DataGrid2, etc.)
- `sdk/widgets/templates/README.md` - **Critical**: Template extraction requirements (must include both `type` AND `object`)
- `generated/metamodel/enums.go` - All Mendix enumeration types
- `modelsdk/meta/system_module.go` - The virtual System module's entities, attributes and associations. String lengths are **measured**, from the System module's domain model inside a built `deployment/model/model.mdp` (a BSON document stream, one `mxbuild --target=deploy` for all 115 at once) — not from the Model SDK, which describes metamodel types and does not contain them. `modelsdk/meta/testdata/system_string_lengths.txt` is the measurement and `TestSystemStringLengths` holds the table to it; a `Length` of 0 is Mendix's "unlimited", never "unmeasured". Measured identical across 10.24.4 and 11.14.0, which is why there is one table and not a per-version registry
- `mdl/grammar/MDL.g4` - ANTLR4 grammar for MDL syntax (production)
- `mdl/executor/executor.go` - MDL statement execution logic
- `reference/mdl-grammar/` - Comprehensive MDL grammar reference
- `reference/mendixmodellib/reflection-data/` - Type definitions with storage names and default values
- `docs/03-development/MDL_PARSER_ARCHITECTURE.md` - ANTLR4 parser design documentation
- `docs/03-development/MODELSDK_ENGINE_ARCHITECTURE.md` - **Read before extending the modelsdk engine**: layers, the canonical write/read/ALTER patterns, codec mechanisms (TypeDefaults, list markers, storage-name overrides), the engalar harvest rule, and the add-a-document-type recipe
- `docs/03-development/PAGE_BSON_SERIALIZATION.md` - Page/widget BSON format, type mappings, required defaults
- `docs/03-development/WIDGET_BSON_VERSION_COMPATIBILITY.md` - What's version-resilient vs version-fragile in widget BSON output, and how to onboard a new Mendix minor (e.g. 11.10)
- `.claude/skills/debug-bson.md` - Workflow for debugging BSON serialization issues with `mx` tool (includes the "Studio Pro Update Widget" diff methodology that closed CE0463)
- `.claude/skills/diagnose-ce0463.md` - **Read first for any CE0463 report**: the elimination order, the two controls that separate "the user upgraded a widget package" (not our bug) from a real mxcli defect, and the measurement traps that make CE0463 investigations go wrong
- `.claude/skills/verify-in-runtime.md` - Proving a fix in a real app in a real browser (`run --local` + Playwright). For symptoms that only exist at render time, where valid-looking BSON and a clean `mx check` prove nothing — see #812
- `cmd/mxcli/lsp.go` - LSP server implementation (hover, definition, diagnostics, completion, symbols)
- `cmd/mxcli/init.go` - `mxcli init` command (project initialization + VS Code extension install)
- `cmd/mxcli/docker/oql.go` - OQL query execution against running Mendix runtime via M2EE admin API
- `sql/connection.go` - External SQL connection manager (credential isolation)
- `sql/config.go` - DSN resolution (env vars, YAML config)
- `sql/import.go` - IMPORT pipeline (batch insert, Mendix ID generation, sequence tracking)
- `sql/generate.go` - Database Connector MDL generation from external schema
- `sql/typemap.go` - SQL → Mendix type mapping, DSN → JDBC URL conversion
- `sql/mendix.go` - Mendix DB helpers (DSN builder, table/column name conversion)
- `cmd/mxcli/cmd_sql.go` - `mxcli sql` CLI subcommand
- `mdl/executor/cmd_sql.go` - SQL statement executor handlers
- `mdl/executor/cmd_import.go` - IMPORT statement executor (auto-connects to Mendix DB)
- `vscode-mdl/src/extension.ts` - VS Code extension entry point
- `vscode-mdl/package.json` - VS Code extension manifest (commands, menus, settings)
