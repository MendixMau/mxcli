// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// retrieveSingleRule is the rule ID for "a LIMIT 1 retrieve used as a list".
const retrieveSingleRule = "MDL-RETRIEVE01"

// checkRetrieveLimitOneAsList flags a variable that `RETRIEVE … LIMIT 1` bound to
// a single OBJECT and a later statement uses as a LIST.
//
// `limit 1` is not a one-element list here. The executor maps it to Mendix's
// "First object" range (RangeTypeFirst, cmd_microflows_builder_actions.go), which
// makes the output variable an object — and that is deliberate and documented
// (MDL_QUICK_REFERENCE.md), not something to change under anyone's feet.
//
// What was missing is any sign of it before the build. Nothing in the MDL says
// the variable changed shape: `mxcli check --references` passed, and DESCRIBE
// re-emits `limit 1`, so the source of an object retrieve and a list retrieve are
// identical text. The first thing the author saw was CE0097 "The selected 'x'
// variable must be of type List" from mxbuild — and inside a .test.mdl file, not
// even that: the injected test simply failed to build (mendixlabs/mxcli#1103).
//
// The clause is also spelled the other way round elsewhere in the same language —
// `import from mapping … first` binds an object and `… limit 1` a one-element
// list — so reading it as a list is a reasonable mistake rather than a careless
// one. The message therefore names the working spelling instead of only refusing.
//
// Keyed on exactly the condition the writer uses (limit "1", no offset), because
// a check that disagrees with the writer it describes is worse than no check.
func (v *microflowValidator) checkRetrieveLimitOneAsList(body []ast.MicroflowStatement) {
	// single holds the variables currently bound to one object by a LIMIT 1
	// retrieve. Maintained in statement order so a rebinding clears it: a name
	// reused for a real list further down is not this rule's business.
	single := map[string]bool{}

	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		if name, op := listUseOf(s); name != "" && single[name] {
			v.addViolation(retrieveSingleRule, linter.SeverityError,
				fmt.Sprintf("$%s was retrieved with LIMIT 1, which binds a single object rather than a "+
					"one-element list, so %s cannot take it — mxbuild rejects this with CE0097 "+
					"\"The selected '%s' variable must be of type List\".", name, op, name),
				fmt.Sprintf("Drop the LIMIT to retrieve a list and keep %s, or keep LIMIT 1 and use "+
					"$%s as the object it already is.", op, name))
		}

		// Rebinding first, so a statement that both consumes and produces the
		// name is judged on what it consumed.
		for _, p := range statementProducedVars(s) {
			delete(single, p.name)
		}
		if r, ok := s.(*ast.RetrieveStmt); ok && r.Limit == "1" && r.Offset == "" && r.Variable != "" {
			single[r.Variable] = true
		}
	})
}

// listUseOf reports the list variable a statement consumes, and a phrase naming
// what consumes it. ("", "") when the statement takes no list.
func listUseOf(s ast.MicroflowStatement) (string, string) {
	switch st := s.(type) {
	case *ast.ListOperationStmt:
		if st.InputVariable != "" {
			return st.InputVariable, st.Operation.String() + "()"
		}
		if st.SecondVariable != "" {
			return st.SecondVariable, st.Operation.String() + "()"
		}
	case *ast.AggregateListStmt:
		if st.InputVariable != "" {
			return st.InputVariable, st.Operation.String() + "()"
		}
	case *ast.LoopStmt:
		if st.ListVariable != "" {
			return st.ListVariable, "a loop"
		}
	case *ast.AddToListStmt:
		if st.List != "" {
			return st.List, "ADD … TO"
		}
	case *ast.RemoveFromListStmt:
		if st.List != "" {
			return st.List, "REMOVE … FROM"
		}
	}
	return "", ""
}
