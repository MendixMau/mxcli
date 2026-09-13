// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// checkMergeJoinLabels validates `merge <label>` / `join <label>` statically, so
// `mxcli check` reports what exec would refuse. Three rules, each a different
// failure:
//
//	MDL-FLOW02  a `join` with no matching `merge` (and the reverse)
//	MDL-FLOW03  the same label declared twice
//	MDL-FLOW04  a `merge` or `join` inside a LOOP or WHILE body
//
// MDL-FLOW04 is the one that is a Mendix constraint rather than a typo. A
// LoopedActivity owns its own MicroflowObjectCollection, and a SequenceFlow may
// not cross that boundary — so there is no graph a `join` out of a loop could
// build. Refusing beats wiring something Studio Pro cannot load.
func (v *microflowValidator) checkMergeJoinLabels(body []ast.MicroflowStatement) {
	declared := map[string]int{}
	joined := map[string]bool{}

	// Loop bodies are walked separately: the rule for them is not "is the label
	// resolvable" but "there may be no label here at all".
	var walkTop func(stmts []ast.MicroflowStatement)
	var walkInLoop func(stmts []ast.MicroflowStatement)

	inLoopReported := false
	walkInLoop = func(stmts []ast.MicroflowStatement) {
		for _, s := range stmts {
			switch n := s.(type) {
			case *ast.MergeStmt, *ast.JoinStmt:
				if inLoopReported {
					continue
				}
				inLoopReported = true
				v.addViolation("MDL-FLOW04", linter.SeverityError,
					"`merge` / `join` cannot be used inside a LOOP or WHILE body. A Mendix loop is a "+
						"LoopedActivity that owns its own object collection, and a sequence flow cannot "+
						"leave it — so there is no graph this could build.",
					"Use BREAK or CONTINUE to leave the loop, and put the merge on the path outside it.")
			case *ast.IfStmt:
				walkInLoop(n.ThenBody)
				walkInLoop(n.ElseBody)
			case *ast.LoopStmt:
				walkInLoop(n.Body)
			case *ast.WhileStmt:
				walkInLoop(n.Body)
			}
		}
	}

	walkTop = func(stmts []ast.MicroflowStatement) {
		for _, s := range stmts {
			switch n := s.(type) {
			case *ast.MergeStmt:
				declared[n.Label]++
			case *ast.JoinStmt:
				joined[n.Label] = true
			case *ast.IfStmt:
				walkTop(n.ThenBody)
				walkTop(n.ElseBody)
			case *ast.EnumSplitStmt:
				for _, c := range n.Cases {
					walkTop(c.Body)
				}
				walkTop(n.ElseBody)
			case *ast.InheritanceSplitStmt:
				for _, c := range n.Cases {
					walkTop(c.Body)
				}
				walkTop(n.ElseBody)
			case *ast.LoopStmt:
				walkInLoop(n.Body)
			case *ast.WhileStmt:
				walkInLoop(n.Body)
			}
			// An `on error { … }` block is an ordinary body for labels: joining
			// out of it into the main path is the case merge/join exists for.
			if eh := getErrorHandlerBody(s); len(eh) > 0 {
				walkTop(eh)
			}
		}
	}
	walkTop(body)

	if len(declared) == 0 && len(joined) == 0 {
		return
	}

	for _, label := range sortedLabels(joined) {
		if declared[label] == 0 {
			v.addViolation("MDL-FLOW02", linter.SeverityError,
				fmt.Sprintf("join %s: no `merge %s;` in this microflow.", label, label),
				fmt.Sprintf("Declare the join point on the path that owns it: `merge %s;`. "+
					"Forward and backward references both resolve, so it may come before or after.", label))
		}
	}

	declaredNames := make(map[string]bool, len(declared))
	for l := range declared {
		declaredNames[l] = true
	}
	for _, label := range sortedLabels(declaredNames) {
		if declared[label] > 1 {
			v.addViolation("MDL-FLOW03", linter.SeverityError,
				fmt.Sprintf("merge %s: declared %d times — a label names one join point.", label, declared[label]),
				"Give each join point its own label, or delete the duplicate declaration.")
		}
		if !joined[label] {
			v.addViolation("MDL-FLOW02", linter.SeverityError,
				fmt.Sprintf("merge %s: nothing joins it.", label),
				fmt.Sprintf("Every merge needs at least one incoming path — Mendix rejects one with none. "+
					"Either send a path to it with `join %s;` or remove the declaration.", label))
		}
	}
}

func sortedLabels(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for l := range set {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}
