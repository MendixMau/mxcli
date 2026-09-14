# Implementation Plan — Retire the legacy engine

**Date:** 2026-09-14
**Status:** Draft plan
**Continues:** [`2026-06-05-adopt-modelsdk-engine.md`](2026-06-05-adopt-modelsdk-engine.md), which
stops at the cutover. That plan still reads as though `legacy` were the default; it is not, and has
not been since the codec engine took over. This plan covers what the earlier one deferred to
"Phase 5 — Cleanup" and never specified.
**Related:** [ADR-0004](../13-decisions/0004-full-codec-engine.md) (route all document types through
the codec), [ADR-0002](../13-decisions/0002-backend-abstraction.md) (the seam this all hangs off).

---

## 1. The plan in one paragraph

Retiring the legacy engine is **three independent removals wearing one name**, and the whole value
of writing this down is refusing to treat them as one job. Deleting the legacy *backend*
(`mdl/backend/mpr`, 2,808 lines) is small, unblocked, and reversible. Deleting the legacy
*serializer* underneath it (`sdk/mpr`, 41,243 lines) is fifteen times larger and is gated on
migrating consumers that never touched the engine seam at all — the public `api/` package, the MCP
backend, eight `cmd/mxcli` commands. The mongo-driver v1→v2 migration the earlier plan promised at
cutover is **gated on the second, not the first**, which is the opposite of what that plan assumed.
Phase 1 can start today; Phases 2 and 3 need decisions that are not this plan's to make.

## 2. Why this is not one job (the measurement that reorders everything)

The 2026-06-05 plan deferred the tree-wide driver migration "to the cutover", on the reasoning that
the two engines could coexist because "v1 and v2 are different module paths". They can, and they
still do. But the split does not fall where that plan implies:

| Package | mongo-driver v1 | v2 | What it is |
|---|---|---|---|
| `modelsdk/` | **0** | **117** | the codec engine |
| `sdk/mpr` | **113** | **0** | the legacy serializer |
| `mdl/backend/modelsdk` | 44 | 45 | the adapter where the two meet |
| `mdl/executor` | 17 | 5 | |

The driver split maps almost exactly onto the **serializer** split, not onto the engine flag. The
codec is already wholly on v2; `sdk/mpr` is wholly on v1; the adapter straddles both because it
converts between semantic types and gen documents. So **deleting the legacy backend does not move
the driver migration at all** — what unblocks v2 is deleting `sdk/mpr`, and that is Phase 3, behind
a decision about the public API.

Stating this is the point of the plan. Sequenced the other way round, Phase 1 looks like it owes a
41k-line migration and never gets started.

## 3. What is already true (verified 2026-09-14, not assumed)

Every one of these was checked rather than inherited from the earlier plan:

- **No feature falls back to legacy.** No "not supported by the modelsdk engine" message exists
  anywhere in the tree. The last five widget gaps are closed
  (`mdl/backend/modelsdk/widget_write_legacy_gaps.go` records which were real: the name-based scan
  said twenty-three, the reachable set was five, and one of those five turned out to be a Mendix
  type that does not exist).
- **`errUnimplemented` cannot fire on a default run.** Of 19 unported `FullBackend` methods, a
  per-method build probe found 17 dead and 2 live; the live pair is implemented and the dead set is
  pinned by `mdl/backend/modelsdk/unimplemented_reachability_test.go`, which passes.
- **The dependency runs one way.** `mdl/backend/modelsdk` does not import `mdl/backend/mpr`; its
  reader is `modelsdk/mpr`. (A grep says otherwise — it matches a comment. Check the import block.)
- **Legacy is strictly weaker.** Rules, menus, layouts, message definitions and regular expressions
  all refuse on it.
- **The legacy backend has five production importers**: `cmd/mxcli/engine.go` (the factory seam),
  `mdl/repl/repl.go`, `mdl/enginecompare/compare.go`, and two `examples/`. Plus six test files.

## 4. Phases

### Phase 1 — Delete the legacy backend and its flag *(Effort: S, Risk: Low)*

Everything in §3 says this is unblocked. It is the phase that delivers the stated goal.

1. Remove `engineLegacy` and `engineCompare` from `cmd/mxcli/engine.go`; the flag and
   `MXCLI_ENGINE` become either absent or a one-value no-op (see §5, decision A).
2. Delete `mdl/backend/mpr/` and its tests.
3. Delete `mdl/enginecompare/` and the `engine-diff` make target. The package compares two engines
   and has no meaning with one; its own header still describes write comparison as "Phase 2", which
   never landed.
4. Drop the `engines` matrix leg from `.github/workflows/nightly.yml`. Its comment already says
   *"drop this line only together with the engine itself"* — this is that moment. Per-push CI
   already runs modelsdk alone.
5. Repoint the two `examples/` and the six test files at the codec backend.
6. **`mdl/repl.New()` hardcodes `mprbackend.New()` as its factory default.** Not a live bug —
   `cmd/mxcli/main.go:171` overrides it immediately — but it must go with the package, and a
   caller that forgets to override should not silently get a deleted engine.

**Done when**: `grep -r "MXCLI_ENGINE=legacy"` returns nothing outside history, the full gate passes,
and the doctype corpus runs green on the one remaining engine.

**Reversibility**: total, until Phase 3. `sdk/mpr` is untouched and every deleted file is one
`git revert` away.

### Phase 2 — Fix what goes stale the moment legacy leaves *(Effort: S, Risk: Low)*

These currently misdescribe the engine and would become actively false, so they belong with Phase 1
rather than after it:

- `mdl/backend/modelsdk`'s **package doc still says the engine is a read slice**: *"Phase 1 … is a
  READ slice … Write methods are NOT implemented yet — callers must not rely on them persisting;
  the CLI prints a read-only warning when this engine is selected."* All three clauses are false,
  and the read-only warning no longer exists in the code.
- The 2026-06-05 plan needs a status line pointing here; it reads as current and is not.
- `PROPOSAL_backend_strategy.md` is still `status: draft` from 2026-05-31 and describes a
  multi-backend future in which legacy is one of the backends.
- CLAUDE.md's `--engine` line, and the flag's help text.

### Phase 3 — Decide the fate of `sdk/mpr` *(Effort: L, Risk: Med — NOT scheduled here)*

This is where the size is, and this plan deliberately does not schedule it: **it is a product
decision about the public API, not a cleanup.**

`sdk/mpr` is 41,243 lines and has consumers that never routed through the engine seam:

| Consumer | Why it matters |
|---|---|
| `api/` (9 files, 3,287 lines) | the public fluent API — `modelsdk.Open` / `OpenForWriting` |
| `mdl/backend/mcp` | a **shipped, live backend**; breaking it breaks Studio Pro integration |
| 8 `cmd/mxcli` commands | `bson dump`/`compare`/`discover`, `extract-templates`, `new`, … — these hold a concrete reader on purpose |
| `examples/`, `scripts/mprsnapshot` | |

Three options, and the answer is not obvious:

- **(a) Keep `sdk/mpr` as a reader-only SDK.** Cheapest. Leaves the tree on two drivers
  indefinitely, which means v2's improvements stay out of reach and every new file has to pick a
  driver.
- **(b) Port the consumers to `modelsdk/mpr` and delete `sdk/mpr`.** Unblocks the driver migration.
  Costs an `api/` rewrite and therefore a breaking change to the one thing mxcli publishes as a
  library.
- **(c) Reimplement `api/` on the codec** and keep its signatures, absorbing the cost inside the
  package. Most work, least disruption to users.

**The prerequisite either way** is knowing whether anything outside this repository depends on
`api/`. That is a question for the maintainer, not a measurement.

### Phase 4 — mongo-driver v1 → v2 *(Effort: L, Risk: Med — gated on Phase 3)*

Only reachable via 3(b) or 3(c). With `sdk/mpr` gone the remaining v1 files are the
`mdl/backend/modelsdk` adapter's 44 and `mdl/executor`'s 17, both of which exist to bridge the two
worlds and shrink as the semantic types move to v2. Not worth sequencing until Phase 3 resolves.

## 5. Decisions to confirm before Phase 1 starts

- **A. Does `--engine` survive as a no-op?** Keeping it as an accepted-and-ignored value is kinder
  to scripts in the wild; removing it is honest. The current code makes an unrecognised value
  **fatal** so typos are loud, which argues for an explicit, friendly error naming the removal
  rather than silence.
- **B. One release of deprecation, or delete now?** The earlier plan promised legacy would stay
  "reachable for one release as an escape hatch" after cutover. Whether that release has passed is
  a release-history question, not a code one.
- **C. Does `mxcli bson compare` still make sense?** It is a user-facing command whose purpose was
  comparing engine output. It may have a second life as a Studio-Pro-vs-mxcli diff, which is a
  different feature wearing the same name.

## 6. What could go wrong

- **The nightly is the only thing exercising legacy.** It is also the only thing that would notice
  if some path still reaches it. Delete the matrix leg and the backend in the **same** change, so
  there is no window where an untested engine is still shippable — the `#808` failure mode (an
  integration test that had only ever skipped) is exactly this shape.
- **A concrete-reader caller is not an engine caller.** Several `cmd/mxcli` commands hold a
  `*mpr.Reader` directly. They survive Phase 1 untouched and must not be swept into it; conflating
  them is what makes Phase 1 look big.
- **Deleting `mdl/enginecompare` deletes a measurement capability**, not just a test. If a future
  change needs "does the codec still agree with a known-good serializer", that ability leaves with
  Phase 1 — which is an argument for doing it *after* any outstanding codec parity work, not
  before.

## 7. First concrete steps

1. Answer decisions A and B (§5). They are one-line answers and they gate the diff's shape.
2. Phase 1 as a single PR, with the CI matrix change in it.
3. Phase 2 in the same PR — the stale docs are wrong the moment Phase 1 lands.
4. Open Phase 3 as a **proposal**, not a plan: it needs the `api/` decision before a sequence exists.
