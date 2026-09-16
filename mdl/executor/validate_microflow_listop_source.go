// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// checkListOperationSource flags a list-operation or aggregate whose list
// operand is not a variable — MDL-LISTOP02, mendixlabs/mxcli#1101.
//
// MDL's expression grammar makes list operations look composable:
//
//	$n = count(filter($reqs, $currentObject/Status = Mod.E.Approved));
//
// Mendix's model is not. Each of those is a separate ACTIVITY, and an activity
// stores its list as a variable reference — Microflows$AggregateAction's
// AggregateVariableName, Microflows$ListOperationsAction's list property. There
// is no slot for a nested computation, so the inner call had nowhere to go and
// was dropped, list and predicate together. The activity was then written with
// an empty list, which is the defect's whole signature: `check` clean, `exec`
// printing "Created microflow", and the failure only at build time.
//
// Measured on mxbuild 11.6.6, one microflow per project:
//
//	count(filter(…))       CE0012 "The 'List' property is required."
//	head(filter(…))        CE0096, the list-operation flavour of the same
//	sum(filter(…), 1)      CE0012 + CE0117
//	sort(filter(…), Name)  mxbuild ABORTS — InvalidOperationException on the
//	                       sort attribute, which resolves against the (now
//	                       absent) list's entity. No error code, no line: the
//	                       document cannot be loaded at all.
//	count('nonsense')      CE0012 — nesting is not required to lose the list
//
// The control for all of them is the reporter's own workaround, which builds
// the same two activities explicitly and passes at 0 errors.
//
// Why refuse rather than materialise an implicit variable: the refusal covers
// every spelling from one rule, including the literal operand, which no amount
// of materialising would fix. And it needs no project — the answer is in the
// statement's own text — so plain `mxcli check` reports it.
func (v *microflowValidator) checkListOperationSource(body []ast.MicroflowStatement) {
	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		switch stmt := s.(type) {
		case *ast.ListOperationStmt:
			op := strings.ToLower(stmt.Operation.String())
			for _, u := range stmt.UnresolvedOperands {
				v.reportUnresolvedListOperand(op, stmt.OutputVariable, u, "CE0096")
			}
		case *ast.AggregateListStmt:
			op := strings.ToLower(stmt.Operation.String())
			for _, u := range stmt.UnresolvedOperands {
				v.reportUnresolvedListOperand(op, stmt.OutputVariable, u, "CE0012")
			}
		}
	})
}

// reportUnresolvedListOperand emits one MDL-LISTOP02 violation, naming what was
// written where a list variable belongs and printing the rewrite that works.
func (v *microflowValidator) reportUnresolvedListOperand(op, outputVar string, u ast.UnresolvedOperand, ce string) {
	which := "list argument"
	if u.Index == 1 {
		which = "second list argument"
	}

	src := microflowExprSource(u.Expr)
	if src == "" {
		v.addViolation("MDL-LISTOP02", linter.SeverityError,
			fmt.Sprintf("%s(…): the %s is missing. A Mendix %s activity stores its list as a "+
				"variable reference, so mxbuild rejects an empty one with %s "+
				"\"The 'List' property is required.\".", op, which, activityNoun(op), ce),
			fmt.Sprintf("Pass a list variable, e.g. $%s = %s($MyList).", displayVar(outputVar), op))
		return
	}

	v.addViolation("MDL-LISTOP02", linter.SeverityError,
		fmt.Sprintf("%s(…): the %s is `%s`, which is not a variable. A Mendix %s activity stores "+
			"its list as a variable reference and has no slot for a nested computation, so the "+
			"argument is dropped and the activity is written with an empty list — mxbuild then "+
			"rejects it with %s \"The 'List' property is required.\".",
			op, which, src, activityNoun(op), ce),
		unresolvedListOperandRemedy(op, outputVar, u, src))
}

// unresolvedListOperandRemedy prints the rewrite. It names the variable to
// introduce and which argument to put it in, rather than reconstructing the whole
// corrected statement: the operand's position differs per operation (filter and
// sort carry a predicate or a sort spec after the list, union carries a second
// list), so a reconstructed example would be wrong for most of them.
func unresolvedListOperandRemedy(op, outputVar string, u ast.UnresolvedOperand, src string) string {
	name := "$" + displayVar(outputVar) + "_source"
	if u.Index == 1 {
		name = "$" + displayVar(outputVar) + "_second"
	}
	which := "list argument"
	if u.Index == 1 {
		which = "second list argument"
	}
	if isListOperationCall(u.Expr) {
		return fmt.Sprintf("Give the inner operation its own statement and pass its variable: "+
			"`%s = %s;` then use `%s` as the %s of %s(…). Each list operation is a separate "+
			"activity in Mendix, so they cannot be nested in one expression.",
			name, src, name, which, op)
	}
	return fmt.Sprintf("Assign a list to a variable and pass the variable as the %s of %s(…), "+
		"e.g. `%s = <a list>;`.", which, op, name)
}

// isListOperationCall reports whether an expression is a call to one of the list
// operations — the case where the remedy can name the exact rewrite.
func isListOperationCall(expr ast.Expression) bool {
	call, ok := expr.(*ast.FunctionCallExpr)
	if !ok {
		return false
	}
	switch strings.ToUpper(call.Name) {
	case "HEAD", "TAIL", "FIND", "FILTER", "SORT", "UNION", "INTERSECT",
		"SUBTRACT", "RANGE", "COUNT", "SUM", "AVERAGE", "MINIMUM", "MAXIMUM":
		return true
	}
	return false
}

// activityNoun names the activity Mendix would build, for the diagnostic.
func activityNoun(op string) string {
	switch op {
	case "count", "sum", "average", "minimum", "maximum", "reduce", "all", "any":
		return "aggregate list"
	default:
		return "list operation"
	}
}

// displayVar keeps the message readable when the statement has no output
// variable (a parse that got far enough to build the activity but not to name
// its result).
func displayVar(outputVar string) string {
	if outputVar == "" {
		return "Result"
	}
	return outputVar
}
