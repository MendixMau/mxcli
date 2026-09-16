// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func retrieveSingleViolations(t *testing.T, stmt *ast.CreateMicroflowStmt) []string {
	t.Helper()
	var out []string
	for _, v := range ValidateMicroflow(stmt) {
		if v.RuleID == retrieveSingleRule {
			out = append(out, v.Message)
		}
	}
	return out
}

func mfWith(body ...ast.MicroflowStatement) *ast.CreateMicroflowStmt {
	return &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "Probe", Name: "M"},
		Body: body,
	}
}

func retrieveLimit(variable, limit, offset string) *ast.RetrieveStmt {
	return &ast.RetrieveStmt{
		Variable: variable,
		Source:   ast.QualifiedName{Module: "Probe", Name: "Request"},
		Limit:    limit,
		Offset:   offset,
	}
}

// TestRetrieveLimitOneUsedAsList is mendixlabs/mxcli#1103's real defect.
//
// `RETRIEVE $x … LIMIT 1` is compiled to Mendix's "First object" range, so $x is
// an OBJECT, not a one-element list. Nothing between the author and mxbuild said
// so: `mxcli check --references` passed, and `describe` re-emits `limit 1`, so
// the source looks identical to a list retrieve. The first sign was CE0097 at
// the far end of a build — and, in a .test.mdl file, a test that failed to
// inject with no explanation at all.
func TestRetrieveLimitOneUsedAsList(t *testing.T) {
	t.Run("HEAD of a LIMIT 1 retrieve is flagged", func(t *testing.T) {
		got := retrieveSingleViolations(t, mfWith(
			retrieveLimit("reqs", "1", ""),
			&ast.ListOperationStmt{Operation: ast.ListOpHead, InputVariable: "reqs", OutputVariable: "req"},
		))
		if len(got) != 1 {
			t.Fatalf("got %d violations %q, want 1", len(got), got)
		}
		for _, want := range []string{"$reqs", "LIMIT 1", "CE0097"} {
			if !strings.Contains(got[0], want) {
				t.Errorf("message %q does not mention %q", got[0], want)
			}
		}
	})

	t.Run("looping over a LIMIT 1 retrieve is flagged", func(t *testing.T) {
		got := retrieveSingleViolations(t, mfWith(
			retrieveLimit("reqs", "1", ""),
			&ast.LoopStmt{LoopVariable: "r", ListVariable: "reqs"},
		))
		if len(got) != 1 {
			t.Fatalf("got %d violations %q, want 1", len(got), got)
		}
	})

	t.Run("aggregating a LIMIT 1 retrieve is flagged", func(t *testing.T) {
		got := retrieveSingleViolations(t, mfWith(
			retrieveLimit("reqs", "1", ""),
			&ast.AggregateListStmt{Operation: ast.AggregateCount, InputVariable: "reqs", OutputVariable: "n"},
		))
		if len(got) != 1 {
			t.Fatalf("got %d violations %q, want 1", len(got), got)
		}
	})
}

// TestRetrieveLimitOneControls are the cases that must NOT be flagged. Each is a
// shape the rule would swallow if it keyed on the wrong thing.
func TestRetrieveLimitOneControls(t *testing.T) {
	cases := map[string]*ast.CreateMicroflowStmt{
		// The reporter's own control: without LIMIT the retrieve is a list, and
		// this is the single most common shape in every test suite.
		"no limit": mfWith(
			retrieveLimit("reqs", "", ""),
			&ast.ListOperationStmt{Operation: ast.ListOpHead, InputVariable: "reqs", OutputVariable: "req"},
		),
		// LIMIT 2 is a CustomRange, which is a list however small.
		"limit 2": mfWith(
			retrieveLimit("reqs", "2", ""),
			&ast.ListOperationStmt{Operation: ast.ListOpHead, InputVariable: "reqs", OutputVariable: "req"},
		),
		// An offset forces CustomRange even at limit 1 — the executor's own
		// condition, so the rule must use the same one or it will disagree with
		// the writer it is describing.
		"limit 1 with offset": mfWith(
			retrieveLimit("reqs", "1", "5"),
			&ast.ListOperationStmt{Operation: ast.ListOpHead, InputVariable: "reqs", OutputVariable: "req"},
		),
		// Using it as an object is exactly right and must stay silent.
		"used as an object": mfWith(
			retrieveLimit("req", "1", ""),
			&ast.MfCommitStmt{Variable: "req"},
		),
		// Rebound to a real list before the list use.
		"rebound to a list": mfWith(
			retrieveLimit("reqs", "1", ""),
			&ast.CreateListStmt{Variable: "reqs", EntityType: ast.QualifiedName{Module: "Probe", Name: "Request"}},
			&ast.ListOperationStmt{Operation: ast.ListOpHead, InputVariable: "reqs", OutputVariable: "req"},
		),
	}
	for name, stmt := range cases {
		t.Run(name, func(t *testing.T) {
			if got := retrieveSingleViolations(t, stmt); len(got) != 0 {
				t.Errorf("flagged a valid microflow: %q", got)
			}
		})
	}
}
