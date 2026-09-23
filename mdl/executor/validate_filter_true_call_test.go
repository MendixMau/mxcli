// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestValidateMicroflow_XPathBooleanCall covers `true()` / `false()` in a
// microflow expression. That is XPath's spelling of the Boolean constants — it is
// valid inside a retrieve's `where [...]`, and CE0117 "Error(s) in expression"
// anywhere a microflow expression is stored (measured with mx check 11.12.2 on a
// filter condition and on a declare). Before the fix `mxcli check` with no project
// passed it, and with -p reported E014 with advice about `empty`, which sent the
// reader looking for a keyword they had not written.
//
// The filter/find condition is covered explicitly: list operations were not
// among the expression sites MDL044 walked at all.
func TestValidateMicroflow_XPathBooleanCall(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantMDL bool
	}{
		{"true() in filter", "$R = filter($L, $currentObject/Flag = true());", true},
		{"false() in find", "$R = find($L, $currentObject/Flag = false());", true},
		{"bare true() as filter condition", "$R = filter($L, true());", true},
		{"true() in declare", "declare $b Boolean = true();", true},
		{"true() inside and", "$R = filter($L, $currentObject/Flag = true() and $currentObject != $G);", true},
		// Controls: the literal spelling is valid and stays silent.
		{"true literal in filter", "$R = filter($L, $currentObject/Flag = true);", false},
		{"object compare in filter", "$R = filter($L, $currentObject != $G);", false},
		{"true literal in declare", "declare $b Boolean = true;", false},
		// An unknown call inside a filter condition is caught too, now that the
		// condition is walked.
		{"unknown func in filter", "$R = filter($L, isBlank($currentObject/Name));", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "create microflow M.F ($L: list of M.E, $G: M.E)\nbegin\n  " + tc.body + "\nend;"
			ids := mdlIDs(buildMF(t, src))
			msg, got := ids["MDL044"]
			if got != tc.wantMDL {
				t.Fatalf("MDL044 fired=%v, want %v (body %q) all=%v", got, tc.wantMDL, tc.body, ids)
			}
			if got && strings.Contains(tc.body, "true()") && !strings.Contains(msg, "write `true`") {
				t.Errorf("suggestion should name the literal spelling, got %q", msg)
			}
		})
	}
}

// TestFilterPredicateParsingTerminates guards the report that one `filter(...,
// true…)` variant hung `mxcli check`. Every malformed shape of the statement
// must come back — with errors or without — well inside the deadline, through
// both the MDL visitor and the validator. A parser must never hang.
func TestFilterPredicateParsingTerminates(t *testing.T) {
	bodies := []string{
		"$R = filter($L, true;",
		"$R = filter($L, true));",
		"$R = filter($L, true true);",
		"$R = filter($L, );",
		"$R = filter($L, = true);",
		"$R = filter($L, $currentObject != );",
		"$R = filter($L, $currentObject/Flag = true()",
		"$R = filter($L, true) and true;",
		"$R = filter($L, true or);",
		"$R = filter($L, (true);",
		"$R = filter($L, true() = true());",
		"$R = filter($L, true()()());",
		"$R = filter($L, true(true));",
		"$R = filter($L, if true() then true() else false());",
		"$R = filter($L, [Flag = true()]);",
		"$R = filter($L, true",
		"$R = filter($L, true(",
	}
	for _, b := range bodies {
		src := "create microflow M.F ($L: list of M.E, $G: M.E)\nbegin\n  " + b + "\nend;"
		done := make(chan struct{})
		go func() {
			defer close(done)
			prog, _ := visitor.Build(src)
			if prog == nil {
				return
			}
			for _, s := range prog.Statements {
				if mf, ok := s.(*ast.CreateMicroflowStmt); ok {
					_ = ValidateMicroflow(mf)
				}
			}
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("parsing/validating did not terminate within 5s: %q", b)
		}
	}
}
