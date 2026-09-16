// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// listopSourceViolations parses real MDL, validates the microflow it declares
// and returns only the MDL-LISTOP02 messages.
//
// It goes through the visitor rather than hand-building the AST on purpose: the
// defect in mendixlabs/mxcli#1101 IS the conversion, so a test that writes the
// AST it wants would assert against the wrong side of the bug.
func listopSourceViolations(t *testing.T, src string) []string {
	t.Helper()
	prog := parseMDL(t, src)
	var out []string
	for _, stmt := range prog.Statements {
		mf, ok := stmt.(*ast.CreateMicroflowStmt)
		if !ok {
			continue
		}
		for _, v := range ValidateMicroflow(mf) {
			if v.RuleID == "MDL-LISTOP02" {
				out = append(out, v.Message)
			}
		}
	}
	return out
}

// microflowSrc wraps a body in a microflow that retrieves two lists, so each
// case below is only the statement under test.
func microflowSrc(name, body string) string {
	return "create or replace microflow Shop." + name + `()
begin
  retrieve $reqs from Shop.Request;
  retrieve $others from Shop.Request;
  ` + body + `
end;`
}

// TestNestedListOperandIsReported is mendixlabs/mxcli#1101 exactly as reported:
// COUNT over an inline FILTER. Measured before the fix — `check` clean, `exec`
// printed "Created microflow", and mxbuild 11.6.6 then failed the build with
// CE0012 "The 'List' property is required." at Aggregate list activity 'Count'.
func TestNestedListOperandIsReported(t *testing.T) {
	got := listopSourceViolations(t, microflowSrc("CountFilter",
		`$n = COUNT(FILTER($reqs, $currentObject/Status = Shop.ENUM_Status.Approved));`))
	if len(got) != 1 {
		t.Fatalf("expected 1 MDL-LISTOP02 violation, got %d: %v", len(got), got)
	}
	for _, want := range []string{"count", "filter($reqs", "CE0012"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("message %q does not mention %q", got[0], want)
		}
	}
}

// TestNestedListOperandInEveryShape covers the spellings measured against
// mxbuild 11.6.6 while investigating #1101. The list operand is dropped by the
// same line in every one of them, so a fix that only handles COUNT leaves the
// rest silent.
func TestNestedListOperandInEveryShape(t *testing.T) {
	cases := []struct {
		name string
		body string
		// buildSymptom is what mxbuild did with the document before the fix.
		buildSymptom string
	}{
		{"CountOfFilter", `$n = COUNT(FILTER($reqs, $currentObject/Name != ''));`, "CE0012"},
		{"HeadOfFilter", `$h = HEAD(FILTER($reqs, $currentObject/Name != ''));`, "CE0096"},
		{"SumOfFilter", `$n = SUM(FILTER($reqs, $currentObject/Name != ''), 1);`, "CE0012 + CE0117"},
		{"SortOfFilter", `$s = SORT(FILTER($reqs, $currentObject/Name != ''), Name);`, "mxbuild abort"},
		{"FilterOfSort", `$f = FILTER(SORT($reqs, Name), $currentObject/Name != '');`, "CE0096"},
		{"TailOfFilter", `$t = TAIL(FILTER($reqs, $currentObject/Name != ''));`, "CE0096"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := listopSourceViolations(t, microflowSrc(tc.name, tc.body))
			if len(got) != 1 {
				t.Fatalf("%s (mxbuild: %s): expected 1 MDL-LISTOP02 violation, got %d: %v",
					tc.body, tc.buildSymptom, len(got), got)
			}
		})
	}
}

// TestLiteralListOperandIsReported: nesting is not required to lose the list.
// `COUNT('nonsense')` reached mxbuild as an aggregate with an empty List and the
// same CE0012 — so the rule keys on "did not reduce to a variable", not on the
// operand being a call.
func TestLiteralListOperandIsReported(t *testing.T) {
	got := listopSourceViolations(t, microflowSrc("CountLiteral", `$n = COUNT('nonsense');`))
	if len(got) != 1 {
		t.Fatalf("expected 1 MDL-LISTOP02 violation, got %d: %v", len(got), got)
	}
}

// TestSecondListOperandIsReported covers the two-list operations, where the
// dropped operand is the SECOND one — `union($others, filter(…))` stored
// $others and an empty second list.
func TestSecondListOperandIsReported(t *testing.T) {
	got := listopSourceViolations(t, microflowSrc("UnionFilter",
		`$u = UNION($others, FILTER($reqs, $currentObject/Name != ''));`))
	if len(got) != 1 {
		t.Fatalf("expected 1 MDL-LISTOP02 violation, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "second") {
		t.Errorf("message %q does not say which operand was dropped", got[0])
	}
}

// TestResolvedListOperandsAreNotReported is the control. Every one of these is a
// spelling mxbuild accepts at 0 errors (the two-variable form is the workaround
// the reporter found), so a rule that fires here would be worse than the bug.
func TestResolvedListOperandsAreNotReported(t *testing.T) {
	bodies := []string{
		// The reporter's own workaround.
		`$approved = FILTER($reqs, $currentObject/Name != '');
		 $n = COUNT($approved);`,
		`$n = COUNT($reqs);`,
		`$h = HEAD($reqs);`,
		`$s = SORT($reqs, Name);`,
		`$u = UNION($reqs, $others);`,
		`$i = INTERSECT($reqs, $others);`,
		`$r = RANGE($reqs, 0, 10);`,
		`$f = FILTER($reqs, $currentObject/Name != '');`,
		// Aggregates over an attribute path and over a per-item expression:
		// buildSetAggregate resolves both, so neither is a dropped operand.
		`$n = SUM($reqs.Amount);`,
		`$n = SUM($reqs, $currentObject/Amount * 2);`,
		// String functions that share a name with a list operation must stay
		// value expressions — they never become a list activity at all.
		`$p = find('haystack', 'needle');`,
		`$b = contains($reqs/Name, 'x');`,
	}
	for i, body := range bodies {
		got := listopSourceViolations(t, microflowSrc("Control", body))
		if len(got) != 0 {
			t.Errorf("case %d %q: expected no MDL-LISTOP02 violation, got %v", i, body, got)
		}
	}
}
