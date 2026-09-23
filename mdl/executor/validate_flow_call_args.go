// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// validateFlowCallArguments checks the arguments of every CALL MICROFLOW and
// CALL NANOFLOW in a flow body against the called flow's signature.
//
// Two shapes pass `check --references` and `exec` and fail only at build time:
//
//   - a parameter the call does not map. mxbuild reports CE0115 "the arguments
//     … do not match the expected parameters"; the flow builder writes whatever
//     arguments were given and never compares them with the signature.
//   - a string literal passed to an enumeration-typed parameter
//     (`EventType = 'GuestRemoved'`). mxbuild reports CE0117 "The expression is
//     of type String but should be of type Enumeration …"; the value has to be
//     the qualified enumeration value, `Module.Enum.GuestRemoved`.
//
// Signatures come from the project and from flows this script creates (script
// definitions win, as in validateFlowArguments). A target that resolves to
// neither is skipped: "not found" is validateFlowBodyReferences' report.
func validateFlowCallArguments(ctx *ExecContext, body []ast.MicroflowStatement, sc *scriptContext) []string {
	if !ctx.Connected() || !bodyHasFlowCall(body) {
		return nil
	}
	sigs := buildFlowSignatures(ctx)
	if sc != nil {
		for name, sig := range sc.flowParams {
			sigs[name] = sig
		}
	}
	if len(sigs) == 0 {
		return nil
	}
	return flowCallArgErrors(body, sigs)
}

// forEachFlowStatement is forEachMicroflowStatement plus custom error-handler
// bodies, which hold ordinary activities too.
func forEachFlowStatement(body []ast.MicroflowStatement, fn func(ast.MicroflowStatement)) {
	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		fn(s)
		if eh := getErrorHandlerBody(s); eh != nil {
			forEachFlowStatement(eh, fn)
		}
	})
}

func bodyHasFlowCall(body []ast.MicroflowStatement) bool {
	found := false
	forEachFlowStatement(body, func(s ast.MicroflowStatement) {
		switch s.(type) {
		case *ast.CallMicroflowStmt, *ast.CallNanoflowStmt:
			found = true
		}
	})
	return found
}

// flowCallArgErrors is the rule with the signatures handed in.
func flowCallArgErrors(body []ast.MicroflowStatement, sigs map[string]*flowSignature) []string {
	var errs []string
	forEachFlowStatement(body, func(s ast.MicroflowStatement) {
		var kind string
		var target ast.QualifiedName
		var args []ast.CallArgument
		switch c := s.(type) {
		case *ast.CallMicroflowStmt:
			kind, target, args = "microflow", c.MicroflowName, c.Arguments
		case *ast.CallNanoflowStmt:
			kind, target, args = "nanoflow", c.NanoflowName, c.Arguments
		default:
			return
		}
		sig, ok := sigs[strings.ToLower(target.String())]
		if !ok || sig == nil {
			return
		}
		given := make(map[string]ast.CallArgument, len(args))
		for _, a := range args {
			given[strings.ToLower(strings.TrimPrefix(a.Name, "$"))] = a
		}
		var missing []string
		for _, p := range sig.Params {
			a, ok := given[strings.ToLower(p.Name)]
			if !ok {
				missing = append(missing, "$"+p.Name)
				continue
			}
			if p.Enum == "" {
				continue
			}
			if lit, ok := stringLiteralArg(a.Value); ok {
				errs = append(errs, fmt.Sprintf(
					"call %s %s: parameter $%s is of enumeration type %s, but is passed the string '%s'. "+
						"mxbuild rejects this as CE0117 (\"The expression is of type String but should be of type Enumeration\"). "+
						"Pass the enumeration value instead: %s = %s.%s",
					kind, target.String(), p.Name, p.Enum, lit, a.Name, p.Enum, lit))
			}
		}
		if len(missing) > 0 {
			errs = append(errs, fmt.Sprintf(
				"call %s %s does not pass %s. Every parameter must be mapped; mxbuild rejects the call as CE0115 "+
					"(\"the arguments … do not match the expected parameters\"). %s declares %s",
				kind, target.String(), strings.Join(missing, ", "), target.String(),
				"$"+strings.Join(sig.paramNames(), ", $")))
		}
	})
	return errs
}

// stringLiteralArg reports whether an argument is a bare string literal, and
// returns its value.
func stringLiteralArg(e ast.Expression) (string, bool) {
	for {
		switch v := e.(type) {
		case *ast.SourceExpr:
			e = v.Expression
		case *ast.ParenExpr:
			e = v.Inner
		case *ast.LiteralExpr:
			if v.Kind != ast.LiteralString {
				return "", false
			}
			s, ok := v.Value.(string)
			return s, ok
		default:
			return "", false
		}
	}
}
