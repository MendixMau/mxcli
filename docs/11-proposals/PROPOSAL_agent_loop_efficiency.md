---
title: Agent loop efficiency — making an mxcli session cost what the work costs
status: draft
date: 2026-09-22
related:
  - PROPOSAL_mxcli_dev_warm_loop.md
  - PROPOSAL_playwright_session_reuse.md
  - PROPOSAL_llm_mdl_assistance.md
  - PROPOSAL_session_logging.md
  - PROPOSAL_check_diagnostics_catalog.md
  - cmd/mxcli/init_claudemd_budget_test.go
---

# Agent loop efficiency — making an mxcli session cost what the work costs

## The report

A side-by-side test had Opus build an app twice: once on Vercel (TypeScript files
straight to disk), once with mxcli against Mendix.

| | mxcli / Mendix | Vercel |
|---|---|---|
| Duration | 2 h 33 | 1 h 07 |
| Model calls | **523** | **123** |
| Conversation re-read from cache | **228 M** | **42 M** |
| Cache writes | 1.27 M | 0.48 M |
| Output tokens | 500 k | 205 k |
| Avg conversation per call | 435 k | 345 k |
| Bash commands | 425 | 58 |

The bill is dominated by the 228 M re-read, and that figure is not a mystery:
523 × 435 k ≈ 228 M. It is the product of two numbers and nothing else.

## Why this is a square, not a line

Every model call re-reads the whole conversation. Conversation size grows with
the content the calls put into it. So for a session of **N** calls whose
transcript grows roughly linearly to size **S**:

```
total re-read  ≈  N × S/2
```

and when the removed calls take their own tool results out with them, **S falls
with N** — so the total falls with **N²**. Halving the call count on the same
work is a ~4× cut, not a 2× cut. The measured pair is consistent with this:
4.25× the calls and 1.26× the conversation gives 5.4×, which is what the logs
show.

That arithmetic sets the priority order, and it is not the intuitive one:

1. **Fewer calls** — enters quadratically. Everything else is second.
2. **Less text per call** — enters linearly, but it is also the multiplier on
   lever 1, so the two compound.
3. Output tokens (500 k of 228 M) are a rounding error. Do not optimise here.

## What is irreducible, and what is not

Part of the 4× is real and will not go away. Vercel's agent writes a `.tsx` file
and the work is done. A Mendix change is a model mutation that must be
**validated, applied, built, and rendered** before anyone knows it worked. The
feedback also notes the two sessions were not scope-equivalent — the mxcli one
additionally covered login styling, documentation, Docker and password lockout.

So parity is the wrong target. The target is the **gap between the loop we ship
and the loop mxcli is already capable of**, which is large, because most of the
fast paths below already exist and the session did not take them.

The honest claim: of the reported ~400 extra calls, roughly 250–300 look
addressable. The rest is Mendix being a compiled platform.

---

## Lever 1 — collapse the per-change round trip (attacks N)

The reported loop is **5–8 calls per change**: write script → `mxcli check` →
`mxcli exec` → `mx check` → restart (~35 s) → log in → screenshot.

Four of those steps are already avoidable with today's binary:

- **`check` before `exec` is redundant.** `exec` already runs the full semantic
  check before it writes anything and refuses the script on an error
  (`cmd/mxcli/cmd_exec.go`). Yet `projectGates` in `cmd/mxcli/init_claudemd.go`
  lists `check` and `exec` as two consecutive gates, so the generated CLAUDE.md
  in **every** mxcli project teaches the two-call form. One wasted call per
  change, in every session, by construction.
- **The 35 s restart is opt-in slowness.** `run --local --watch` hot-reloads a
  behavioural change in ~2 s, and `mxcli test --attach` skips the boot entirely
  by driving an app already up.
- **Logging in by hand is solved.** Playwright storage state is captured and
  reused (`screenshot --load-storage`); see `PROPOSAL_playwright_session_reuse.md`.
- **The screenshot is usually the wrong instrument.** See lever 3.

### Proposal: one gate command

```bash
mxcli apply changes.mdl -p app.mpr [--verify tests/admin.spec.ts] [--no-build]
```

One call that runs: semantic check → exec → mxbuild verify → reload the running
app (or restart if the serve build says `restartRequired`) → run the named
Playwright verifications → print **one compact verdict**.

The verdict is the whole point. On success it is two lines. On failure it is the
first failing stage and only that stage's diagnostics — not the output of all
five. This is what turns 5–8 calls into 1, and on the failure path into 2.

Nothing here is new capability. Every stage exists; `apply` is the composition,
and the composition is what the agent is currently doing by hand, one tool call
at a time, paying the full conversation for each.

**Also: fix the gate list.** `projectGates` should teach `exec` (check folded in)
rather than `check` then `exec`, and should name `apply` once it exists. The
gates tests (`init_claudemd_gates_test.go`) already hold three copies of that
list to one definition, so this is a one-line change that propagates.

## Lever 2 — shrink what each call adds (attacks S)

A tool result is written into the conversation once and **re-read by every
subsequent call**. A 200-line `exec` transcript early in a 500-call session is
not 200 lines of cost; it is 200 lines × ~400 remaining calls.

- **Report the delta, not the transcript.** `exec` prints a line per statement.
  The idempotence work already distinguishes `Created` / `Replaced` /
  `Unchanged` per unit (`ExecContext.ReportMutation`), so the information to
  collapse this is in hand: `Created 4, replaced 2, unchanged 196` plus the six
  names that changed. A re-run of a settled script should be **one line**.
- **Default to terse; make verbose opt-in.** `MXCLI_QUIET` exists and the skills
  set it in places. It should be the default posture for non-interactive runs.
- **Prefer a terse agent format over `--json`.** `--json` exists globally, but
  JSON is frequently *more* tokens than good prose for the same facts. The
  target is fewest tokens that stay unambiguous, not machine-readability for its
  own sake.
- **Cap the long tails.** `docker check` error dumps, `show microflows` on a big
  module, catalog listings: add `--top N` and severity filters so a 400-line
  result becomes the 10 lines the agent will act on.

This lever is worth doing carefully rather than aggressively: an output trimmed
past the point of usefulness buys back its savings immediately in re-runs.

## Lever 3 — stop paying image input on a loop

Screenshots are expensive input, they recur, and they persist for the rest of
the session. The feedback names them explicitly.

`mxcli playwright verify <file>` already exists and returns **text** pass/fail
against a running app. That is the correct default instrument for "does this
flow work". A screenshot answers a different and much rarer question — "does
this *look* right" — and should be taken deliberately, once, when appearance is
genuinely the subject.

The rule for the skills: **assert in text; screenshot when the question is
visual, and then once.** `.claude/skills/verify-in-runtime.md` already has the
right shape for this (a table routing a symptom to the cheapest sufficient
proof); it needs the text-vs-pixel row added and `run-local` / `test-app`
pointed at it.

## Lever 4 — keep long investigations out of the main conversation

One self-inflicted bug (a stub script that wiped real microflow bodies) took
**~40 calls** to trace. Those 40 results then sat in context for the rest of the
session, taxing every later call.

A subagent pays for its own 40 round trips once and returns a paragraph. The
skills should say so with a trigger rather than a preference: **a diagnosis
expected to take more than ~5 probes is delegated, not run inline.** The same
applies to log spelunking and "which of these 30 files mentions X".

## Lever 5 — turn each discovered workaround into tool knowledge

Five Mendix limitations each cost an investigation and a rework in that session:

| Discovered the hard way | Where it should live instead |
|---|---|
| inputs inside lists are read-only → admin editing must be pop-ups | `mxcli check` diagnostic on an input widget in a list/gallery context |
| the sidebar went stale after actions | `create-page` / `patterns-crud` skill, refresh guidance |
| pop-up styling breaks (pop-ups sit outside the styled area) | `theme-styling` skill |
| login fields did not register scripted input | a `mxcli playwright login` helper that does it correctly |
| the Docker image had the wrong Java version | `mxcli docker check` preflight |

This is the highest-leverage lever on any horizon longer than one session,
because it converts a cost paid **once per session per user** into one paid
**once, by us**. It is also exactly the repo's existing instinct — findings
files, lint rules, check diagnostics — applied to a class of knowledge that has
so far only been rediscovered.

## Lever 6 — measure it, then claim it

Everything above is a hypothesis until it moves a number, and this proposal
should not be merged on plausibility.

mxcli already logs every invocation as JSON Lines under `~/.mxcli/logs/`
(`diaglog`, wired at `newLoggedExecutor` in `cmd/mxcli/main.go` — so it covers
all commands, not a curated subset). That is the instrument.

**`mxcli diag loop-report`** reads a session's log and prints: invocations by
verb, wall time by verb, `check`-immediately-before-`exec` pairs (pure waste),
restarts vs. hot reloads, and output bytes per command. That converts "the loop
feels expensive" into a ranked list of where the calls actually went — and, run
before and after each lever, into evidence that a lever worked.

**Then a benchmark.** One fixed app-brief, run end to end, recording model calls
and tokens. Without it, every claim here is an argument; with it, each lever
lands or does not. It also guards the result: the gate list drifted in three
places once already, and a loop regression is exactly as invisible.

---

## Sequencing

| | Lever | Effort | Expected effect |
|---|---|---|---|
| 1 | `diag loop-report` + benchmark harness (lever 6) | S | none directly — makes the rest falsifiable |
| 2 | Fix `projectGates` to teach `exec`, not `check`+`exec` (lever 1) | XS | ~1 call per change, every project, immediately |
| 3 | Terse/delta output for `exec` and the noisy listings (lever 2) | M | linear cut on S, compounds with 4 |
| 4 | `mxcli apply` (lever 1) | M | the 5–8 → 1–2 collapse; the main event |
| 5 | Text-first verification rule in the skills (lever 3) | S | removes recurring image input |
| 6 | Subagent trigger in the skills (lever 4) | XS | caps the worst tail |
| 7 | Workarounds → diagnostics and skills (lever 5) | M, ongoing | compounds across all future sessions |

Item 1 first is deliberate. Items 2, 5 and 6 are nearly free and can ship
immediately after it. Item 4 is the one that changes the shape of the loop.

## What this does not fix

- Mendix builds. A model change must be compiled to be trusted, and that is
  seconds of wall time and at least one tool call, per change, forever.
- Scope. The reported sessions did not build the same thing.
- MDL not being in training data (`PROPOSAL_llm_mdl_assistance.md` owns that).
  Every MDL statement the agent gets wrong on the first try is a full loop
  iteration, so that proposal and this one multiply rather than overlap — a
  first-attempt success rate is a call-count lever in disguise.
