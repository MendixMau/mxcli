// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// rec builds a session_start line for verb-resolution and segmentation tests.
func startAt(sec int, mode string, args ...string) logRecord {
	return logRecord{
		Time: time.Date(2026, 9, 22, 12, 0, sec, 0, time.UTC),
		Msg:  "session_start",
		Mode: mode,
		Args: append([]string{"./mxcli"}, args...),
	}
}

func endAt(sec, cmds, errs int) logRecord {
	return logRecord{
		Time:             time.Date(2026, 9, 22, 12, 0, sec, 0, time.UTC),
		Msg:              "session_end",
		CommandsExecuted: cmds,
		ErrorsCount:      errs,
	}
}

// The verb comes from cobra rather than a hand-kept list, so a renamed or added
// command is picked up with no change to the report. A hardcoded table is the
// failure this avoids: it goes stale silently, reporting a real command as
// "(unknown)" — and the whole point is to rank commands by how often they run.
func TestInvocationVerbResolvesThroughCobra(t *testing.T) {
	for _, tc := range []struct {
		name, mode, want string
		args             []string
	}{
		{"subcommand", "subcommand", "exec", []string{"exec", "s.mdl", "-p", "a.mpr"}},
		{"nested subcommand", "subcommand", "docker check", []string{"docker", "check", "-p", "a.mpr"}},
		{"one-shot -c", "batch", "-c (one-shot)", []string{"-p", "a.mpr", "-c", "show entities;"}},
		{"repl", "repl", "REPL", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := invocationVerb(append([]string{"./mxcli"}, tc.args...), tc.mode)
			if got != tc.want {
				t.Errorf("verb = %q, want %q", got, tc.want)
			}
		})
	}

	// Guard the guard: if cobra resolution silently stopped working, every verb
	// above would fall back to its mode and the table would still look sane for
	// the flag-only cases. This one can only pass through cobra.
	if got := invocationVerb([]string{"./mxcli", "docker", "check"}, "subcommand"); got != "docker check" {
		t.Fatalf("cobra resolution is not running: got %q", got)
	}
}

// An invocation that did not write session_end is how mxcli reports almost every
// failure: it exits through os.Exit, and deferred Close() does not run then.
//
// This is MEASURED, not inferred. Against a real log of 10 invocations, the 3
// without a session_end were exactly the 3 runs that exited non-zero (two
// refusals and a parse error); the 7 that closed were the 7 that succeeded.
func TestUnclosedInvocationsAreCountedSeparately(t *testing.T) {
	rep := analyzeLoop([]logRecord{
		startAt(0, "subcommand", "exec", "a.mdl", "-p", "x.mpr"),
		endAt(2, 3, 0), // closed, clean
		startAt(3, "subcommand", "exec", "b.mdl", "-p", "x.mpr"),
		// no end: exited non-zero
		startAt(5, "subcommand", "exec", "c.mdl", "-p", "x.mpr"),
		endAt(6, 1, 2), // closed, but reported errors
	})

	if rep.Invocations != 3 {
		t.Errorf("Invocations = %d, want 3", rep.Invocations)
	}
	if rep.Unclosed != 1 {
		t.Errorf("Unclosed = %d, want 1 — the run with no session_end", rep.Unclosed)
	}
	if rep.StatementErrors != 1 {
		t.Errorf("StatementErrors = %d, want 1 — the closed run whose summary reported errors", rep.StatementErrors)
	}
	// Wall time counts only the runs that closed: an unclosed run has no knowable
	// end, and guessing one (say, the next start) would silently inflate the
	// figure the report exists to make trustworthy.
	if rep.WallSeconds != 3 {
		t.Errorf("WallSeconds = %v, want 3 (2s + 1s; the unclosed run contributes nothing)", rep.WallSeconds)
	}
}

// The pair this counts is `check` immediately followed by `exec` of the SAME
// script. Both halves of that are load-bearing, so both have a control.
func TestCheckThenExecPairNeedsSameScriptAndOrder(t *testing.T) {
	pair := []logRecord{
		startAt(0, "subcommand", "check", "a.mdl", "-p", "x.mpr"),
		endAt(1, 1, 0),
		startAt(2, "subcommand", "exec", "a.mdl", "-p", "x.mpr"),
		endAt(3, 1, 0),
	}
	if got := analyzeLoop(pair).CheckExecDup; got != 1 {
		t.Errorf("check then exec of the same script counted %d, want 1", got)
	}

	// CONTROL 1: different scripts are not a pair — checking one file and
	// applying another is two pieces of work, not a doubled call.
	diff := []logRecord{
		startAt(0, "subcommand", "check", "a.mdl", "-p", "x.mpr"),
		endAt(1, 1, 0),
		startAt(2, "subcommand", "exec", "b.mdl", "-p", "x.mpr"),
		endAt(3, 1, 0),
	}
	if got := analyzeLoop(diff).CheckExecDup; got != 0 {
		t.Errorf("different scripts counted %d, want 0", got)
	}

	// CONTROL 2: order matters. exec-then-check is re-validating after applying,
	// which is a different (and defensible) habit.
	rev := []logRecord{
		startAt(0, "subcommand", "exec", "a.mdl", "-p", "x.mpr"),
		endAt(1, 1, 0),
		startAt(2, "subcommand", "check", "a.mdl", "-p", "x.mpr"),
		endAt(3, 1, 0),
	}
	if got := analyzeLoop(rev).CheckExecDup; got != 0 {
		t.Errorf("exec then check counted %d, want 0", got)
	}
}

// A report that counted its own invocations would climb every time it was read.
// `diag` does not write session records today, so this cannot be caught by
// running the binary — it guards the filter that makes the report correct even
// if diag later starts logging.
func TestLoopReportNeverCountsItself(t *testing.T) {
	rep := analyzeLoop([]logRecord{
		startAt(0, "subcommand", "exec", "a.mdl", "-p", "x.mpr"),
		endAt(1, 1, 0),
		startAt(2, "subcommand", "diag", "loop-report"),
		endAt(3, 0, 0),
	})
	if rep.Invocations != 1 {
		t.Errorf("Invocations = %d, want 1 — the diag run must not be counted", rep.Invocations)
	}
	for _, s := range rep.ByVerb {
		if strings.HasPrefix(s.Verb, "diag") {
			t.Errorf("the report lists its own command %q", s.Verb)
		}
	}
}

// A truncated final line is normal: a process killed mid-write leaves one. It
// must not take the whole report down, because the logs are read precisely when
// something went wrong.
func TestParseLogRecordsSkipsUnparseableLines(t *testing.T) {
	lines := []string{
		`{"time":"2026-09-22T12:00:00Z","msg":"session_start","mode":"repl","args":["./mxcli"]}`,
		`{"time":"2026-09-22T12:00:01Z","msg":"sessio`, // truncated
		``,
		`{"time":"2026-09-22T12:00:02Z","msg":"session_end","commands_executed":1,"errors_count":0}`,
	}
	got := parseLogRecords(lines)
	if len(got) != 2 {
		t.Fatalf("parsed %d records, want 2 (the truncated and empty lines skipped)", len(got))
	}
	if rep := analyzeLoop(got); rep.Invocations != 1 {
		t.Errorf("Invocations = %d, want 1", rep.Invocations)
	}
}

// The two error populations are disjoint, and conflating them is the defect
// this test exists to prevent: the field was called `failed` and read 0 across
// a real 442-invocation log while runs were exiting non-zero, because a
// non-zero exit skips the summary record entirely (ako/mxcli#620).
//
// A run that exits reaches Unclosed and must NOT reach StatementErrors; a run
// that finishes while reporting failed statements is the reverse.
func TestStatementErrorsIsDisjointFromUnclosed(t *testing.T) {
	// A run that exited: no session_end, so nothing reports its errors.
	exited := analyzeLoop([]logRecord{
		startAt(0, "subcommand", "exec", "a.mdl", "-p", "x.mpr"),
	})
	if exited.Unclosed != 1 || exited.StatementErrors != 0 {
		t.Errorf("run that exited: Unclosed=%d StatementErrors=%d, want 1 and 0",
			exited.Unclosed, exited.StatementErrors)
	}

	// CONTROL: a run that finished while reporting failed statements is the
	// other population — it closed, so it is not Unclosed.
	continued := analyzeLoop([]logRecord{
		startAt(0, "subcommand", "exec", "a.mdl", "-p", "x.mpr", "--continue-on-error"),
		endAt(2, 5, 3),
	})
	if continued.Unclosed != 0 || continued.StatementErrors != 1 {
		t.Errorf("run that continued past errors: Unclosed=%d StatementErrors=%d, want 0 and 1",
			continued.Unclosed, continued.StatementErrors)
	}
}

// The JSON key is the interface the finding was read through, so it is asserted
// literally. `failed` must be gone, not merely renamed in Go.
func TestJSONKeyNamesWhatItMeasures(t *testing.T) {
	out, err := json.Marshal(loopReport{StatementErrors: 2, Unclosed: 7})
	if err != nil {
		t.Fatal(err)
	}
	js := string(out)
	if !strings.Contains(js, `"runs_with_statement_errors":2`) {
		t.Errorf("JSON lacks runs_with_statement_errors: %s", js)
	}
	if strings.Contains(js, `"failed"`) {
		t.Errorf("JSON still carries the misleading `failed` key: %s", js)
	}
}

// Zero is not printed. A "0" beside a non-zero unclosed count reads as "nothing
// failed", which is the opposite of what the pair means.
func TestStatementErrorLineIsPrintedOnlyWhenNonZero(t *testing.T) {
	var quiet bytes.Buffer
	renderLoopReport(loopReport{Invocations: 3, Unclosed: 2}, &quiet)
	if strings.Contains(quiet.String(), "failed statements") {
		t.Errorf("printed the statement-error line at zero:\n%s", quiet.String())
	}
	if !strings.Contains(quiet.String(), "Did not close: 2") {
		t.Errorf("control failed — the unclosed line is missing:\n%s", quiet.String())
	}

	var loud bytes.Buffer
	renderLoopReport(loopReport{Invocations: 3, StatementErrors: 1}, &loud)
	if !strings.Contains(loud.String(), "Finished with failed statements: 1") {
		t.Errorf("did not print the statement-error line at 1:\n%s", loud.String())
	}
}
