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
- **The 35 s restart is NOT avoidable on Mendix 11.14** — see the section below.
  On 11.13 and earlier, `run --local --watch` hot-reloads a behavioural change
  in ~3 s, so the restart is opt-in slowness there and forced here.
- **Logging in by hand is solved.** Playwright storage state is captured and
  reused (`screenshot --load-storage`); see `PROPOSAL_playwright_session_reuse.md`.
- **The screenshot is usually the wrong instrument.** See lever 3.

### First: the agent can already chain this, and mostly should

The obvious objection to a new command is that `&&` exists:

```bash
mxcli exec changes.mdl -p app.mpr && mxcli docker check -p app.mpr
```

That is **one tool call**, needs nothing built, and captures most of lever 1's
value today. It works because the commands are exit-code-honest — `exec` exits
non-zero if any statement failed, and `docker check` propagates `mx check`'s
status through `cmd.Run()` rather than printing errors and exiting 0. Worth
stating explicitly, because a chain built on a command that reports failure only
in stdout would pass silently, and that is the failure mode that would make
chaining unsafe. It is not present here.

So the call-count win does **not** require a new command. What `&&` does not
give is the *token* win: it concatenates the stdout of every stage that ran, so
a five-stage chain puts five stages of output into the conversation forever —
the opposite of the compact verdict this proposal wants. The agent can paper
over that with per-invocation `tail`/`grep`, but then it is writing fragile
filters that encode each command's output shape, and getting them wrong is
silent.

**That reorders the proposal.** Lever 2 (output discipline) is the more
fundamental of the two, not the junior partner: with terse, delta-shaped output
from each command, `&&` chaining gets nearly all of `apply`'s value at zero new
surface area — and the improvement lands on every *other* invocation too, not
just the ones inside the chain.

### So what, if anything, is left for a command?

Two things, and both are weaker than the first draft claimed:

- **The reload/restart decision.** Mapping the serve build's `restartRequired`
  to reload-vs-restart is not expressible in `&&`. But when `run --local --watch`
  works it already does this in the background, and the agent orchestrates
  nothing; when it does not (11.14, below) the answer is a restart, which *is*
  `&&`-able. So this is thin.
- **Consistency.** An agent composing the chain fresh each session composes it
  differently, and sometimes wrongly — the cost report is the evidence, having
  run the redundant `check`, restarted when it need not have, and logged in by
  hand. But that is cured by **stating the chain**, not by shipping a wrapper
  around it.

**Revised recommendation: publish the one-liner, do not build the command.** Put
the canonical chain in `projectGates` and the skills, fix the outputs it
concatenates, and build `mxcli apply` only if `diag loop-report` (lever 6) shows
agents still composing it wrong after that. This is strictly cheaper, ships
sooner, and does not add a surface that has to stay in sync with the commands
underneath it.

### The chain is tiered, not fixed — most changes stop at the first gate

The first draft put `--verify <playwright>` in the default chain. That is a
mistake of the same kind the cost report is complaining about: **an always-on
gate chain trains maximal verification.** If a browser run is in the default
path, every change pays browser cost, and the report's "I tested every admin
flow ... most of those checks included screenshots" stops being a choice the
agent made and becomes a property of the tool. Hard-wiring it would
institutionalise the expensive failure mode.

Verification tier is a **per-change decision**, and the routing rule already
exists in `.claude/skills/verify-in-runtime.md` — a table from symptom to
cheapest sufficient proof, with explicit counter-examples where the browser
would be waste. The chain should express those tiers and stop at the first one
that is sufficient:

| What changed | Sufficient gate | Cost |
|---|---|---|
| any MDL edit | `mxcli exec` (check folded in) | ~2 s, no build |
| structure a build can reject (pages, widgets, settings) | `+ docker check` / the serve build | ~25 s |
| microflow *behaviour* | `+ mxcli test` — no browser | ~2 s warm |
| what the app **renders or looks like** | `+` browser, once | expensive, rare |

Most changes stop at row 1 or 2. The browser row is the rare one, and it is the
only row that pays image input. Making the tier explicit is itself a lever: it
converts "verify everything, to be safe" into a decision with a stated default.

### The 35 s restart is blocked by mxbuild on 11.14, not by our defaults

An earlier draft of this proposal called the restart-per-change "opt-in
slowness". That is wrong on the version a new project most likely lands on, and
the correction matters because it changes what is actionable.

**On Mendix 11.14 the first build in an `mxbuild --serve` process succeeds and
every subsequent build in that process fails.** The first build does not leave
the deployment in a state its own incremental build can continue from: the
bundler config (`web/rollup.config.mjs` / `web/rspack.config.mjs`) and the
per-document client (`web/pages/`, `web/layouts/`) are both absent, and which
one the build dies on is only how far it gets before it needs one.

This is measured in `cmd/mxcli/docker/webclient_legacy_paths.go`, against
mxbuild 11.14.0 driven **directly over its HTTP API with mxcli removed from the
picture** — the same `/build` request POSTed twice with the model untouched:

```text
build 1   Success
build 2   Failure — ERR_MODULE_NOT_FOUND for web/rollup.config.mjs,
                    imported from mxbuild's own tools/node/rollup-runner.mjs
```

Three controls place it in mxbuild rather than here: a one-shot
`mxbuild --target=deploy` run twice into the same directory succeeds both times
(so it is the serve process's state, not the 11.14 deployment shape); flipping
to Rspack gives an identical failure naming the other config (so it is not the
bundler choice); and 11.13.0 hot-reloads normally — measured, build #2 applied
via reload in 3.4 s. Restoring the deleted config rescues only the
model-unchanged case, which is the case nobody needs. `rm -rf deployment/` costs
a cold build and changes nothing.

mxcli cannot fix this from outside, and does not pretend to: it recognises the
failure **by its own shape** rather than by version, so a fixed mxbuild goes
quiet on its own. The docs say plainly that `--watch` is not usable on 11.14.

Three consequences for this proposal:

1. **The cost report's 35 s-per-change was very likely forced, not chosen.** The
   project that first reported this "routed around it with a restart per
   change", which is exactly the pattern in the session log.
2. **It does not weaken lever 1.** Wall time and call count are separate axes.
   The mxbuild defect taxes *time*; the 228 M bill is *calls*. `mxcli apply`
   collapses 5–8 calls into 1 whether the build underneath it is warm or cold —
   so on 11.14 it is the only lever left on that axis, and therefore more
   important, not less.
3. **`mxcli test --attach` is probably blocked too, and this is unmeasured.**
   `--attach` must rebuild to pick up its test microflows, and it rebuilds
   through the attached app's own serve process (`runner_attach.go`) — whose
   first build happened at boot. That makes the attach rebuild a *second* serve
   build, which is precisely the failing one. This is read off the code, not
   measured; it needs one run on 11.14 to confirm or kill. If it holds, the warm
   *test* loop is blocked on 11.14 as well, and the skills that recommend
   `--attach` need the same version caveat `--watch` already carries.

**The version default makes this worse than it needs to be.** `bootstrap-app`
chooses the newest version on the CDN when the environment has nothing cached —
11.14.0 at the time of writing — so a freshly bootstrapped project lands on
exactly the version where the warm loop does not work, without anyone choosing
it. Until mxbuild is fixed, the default should prefer the newest version whose
warm loop is known to work (11.13.0), and say in one line why. A user who asks
for 11.14 still gets it, with the caveat.

**This is also a standing strategic risk worth recording.** `--serve`, `--host`
and `--port` appear in `mxbuild --help` but not in the reference guide at
docs.mendix.com/refguide/mxbuild/, which documents only the four `--target`
modes. The entire warm loop is built on an undocumented interface carrying no
compatibility promise. That argues for (a) reporting this defect to Mendix
rather than only routing around it, and (b) keeping the cold-build path a
first-class supported mode rather than a fallback.

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
| 2b | Measure `test --attach` on 11.14; pin the bootstrap default off 11.14 | XS | removes a forced 35 s/change from new projects |
| 3 | Publish the canonical `&&` chain in `projectGates` + skills (lever 1) | XS | the 5–8 → 1–2 collapse, with nothing built |
| 4 | Terse/delta output for `exec` and the noisy listings (lever 2) | M | the token half of the chain win; helps every call |
| 5 | Tiered verification rule in the skills (lever 3) | S | stops the default path at the cheapest sufficient gate |
| 6 | Subagent trigger in the skills (lever 4) | XS | caps the worst tail |
| 7 | Workarounds → diagnostics and skills (lever 5) | M, ongoing | compounds across all future sessions |

Item 1 first is deliberate. Items 2, 3, 5 and 6 are all XS-to-S and can ship
immediately after it — item 3 is now the one that changes the shape of the loop,
and it is a documentation change. Item 4 is the only substantial build, and it
is what makes item 3 pay in tokens rather than only in call count.

`mxcli apply` is deliberately **not** in this table. It is contingent on item 1
showing that the published chain is still being composed wrong.

## What this does not fix

- Mendix builds. A model change must be compiled to be trusted, and that is
  seconds of wall time and at least one tool call, per change, forever.
- The 11.14 serve-rebuild defect. It is mxbuild's, the controls are conclusive,
  and nothing mxcli does from outside repairs it. It should be reported upstream;
  meanwhile the version default is the only lever we hold.
- Scope. The reported sessions did not build the same thing.
- MDL not being in training data (`PROPOSAL_llm_mdl_assistance.md` owns that).
  Every MDL statement the agent gets wrong on the first try is a full loop
  iteration, so that proposal and this one multiply rather than overlap — a
  first-attempt success rate is a call-count lever in disguise.
