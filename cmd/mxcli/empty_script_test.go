// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// Text the grammar cannot begin to parse yields ZERO statements and ZERO errors
// — `visitor.Build` cannot tell it from an empty file — so `check` reported
// "Check passed!" and `exec` applied nothing, both at exit 0 (ako/mxcli#618).
//
// A truncated file, a lost heredoc, or a path that resolved to the wrong kind of
// file therefore passes both gates and moves nothing. For a workflow whose
// premise is that mdlsource/ replays, that is the worst available outcome: the
// replay reports success and the model does not change.

func TestGarbageIsNotAnEmptyScript(t *testing.T) {
	line, bad := unparsableInput("this is not valid mdl at all;\n/\n", 0)
	if !bad {
		t.Fatal("a file of pure garbage is accepted as an empty script")
	}
	if line != "this is not valid mdl at all;" {
		t.Errorf("complaint names %q; it should name the first line that did not parse", line)
	}
}

// THE CONTROLS. Each of these legitimately yields zero statements and must stay
// a pass, or the guard would refuse files that are fine.
func TestGenuinelyEmptyInputIsStillAccepted(t *testing.T) {
	for name, src := range map[string]string{
		"empty":            "",
		"whitespace":       "   \n\n\t\n",
		"comments only":    "-- set up the domain model\n-- (nothing yet)\n",
		"comments + blank": "\n-- TODO\n\n",
	} {
		if _, bad := unparsableInput(src, 0); bad {
			t.Errorf("%s: refused, but it is a legitimately empty script", name)
		}
	}
}

// A file that parsed is never the guard's business, whatever it contains.
func TestParsedInputIsNeverFlagged(t *testing.T) {
	if _, bad := unparsableInput("this is not valid mdl at all;", 1); bad {
		t.Error("input that produced statements was flagged; the guard is only for zero")
	}
}
