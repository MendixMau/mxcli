// SPDX-License-Identifier: Apache-2.0

package exprcheck

import (
	"testing"
	"time"
)

// XPath spells the boolean literals as calls. Before the fix the parser took
// `true` as a literal and stopped at `(`, so `check -p` reported E014
// "trailing tokens" with advice about `empty` — while a check without a
// project accepted the expression, since MDL044 only saw CallExprs. mxbuild
// rejects it (CE0117), so the right report is MDL044 via UnknownFunctionCalls.
func TestXPathBooleanCallParsesAndIsReported(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{"$currentObject/Flag = true()", []string{"true"}},
		{"false()", []string{"false"}},
		{"true() and not(false())", []string{"true", "false"}},
		{"$currentObject/Flag = true", nil},
		{"true", nil},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			_, hints := (&parserImpl{}).Parse(tc.src, Context{})
			for _, d := range hints {
				if d.Code == "E014" {
					t.Errorf("unexpected E014 for %q: %s", tc.src, d.Problem)
				}
			}
			var got []string
			for _, u := range UnknownFunctionCalls(tc.src) {
				got = append(got, u.Name)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("UnknownFunctionCalls(%q) = %v, want %v", tc.src, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("UnknownFunctionCalls(%q) = %v, want %v", tc.src, got, tc.want)
				}
			}
		})
	}
}

// Malformed boolean spellings must terminate: a parser that loops on an
// unconsumed `(` is the hang a user reported (it turned out to be project
// load time, but the guarantee is worth pinning).
func TestXPathBooleanCallParseTerminates(t *testing.T) {
	srcs := []string{
		"true(", "true())", "true((", "true(1)", "true()()", "(true()", "true() =",
		"$x/Flag = true(", "not(true(", "false(true())", "true(false()",
	}
	for _, src := range srcs {
		done := make(chan struct{})
		go func() {
			defer close(done)
			(&parserImpl{}).Parse(src, Context{})
			UnknownFunctionCalls(src)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("parse of %q did not terminate", src)
		}
	}
}
