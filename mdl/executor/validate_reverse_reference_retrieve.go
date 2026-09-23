// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// validateReverseReferenceRetrieves reports a retrieve over a Reference
// association from its TO end whose result is then used as an object.
//
// `retrieve $G from $App/UserGroups.GuestGroup_App`, where GuestGroup_App runs
// from GuestGroup to App, starts at the end that does not hold the reference.
// Mendix returns every GuestGroup pointing at $App — a LIST — unless the owner
// is Both (then it is one-to-one and returns an object). The flow builder types
// the output as an object when the body uses it as one, so `exec` writes the
// microflow; mxbuild then rejects `$G/UUID` as CE0117. Nothing short of the
// build said so (marketplace-rnd).
//
// The rule needs the association's ends and the start variable's entity, so it
// runs only against a project. Associations the script itself creates are not
// resolved here (lookupAssociation reads the project); such a retrieve is left
// unjudged rather than guessed at.
func validateReverseReferenceRetrieves(ctx *ExecContext, s *ast.CreateMicroflowStmt, sc *scriptContext) []string {
	if ctx == nil || !ctx.Connected() || ctx.Backend == nil || s == nil || len(s.Body) == 0 {
		return nil
	}
	fb := &flowBuilder{backend: ctx.Backend}

	varEntity := make(map[string]string)
	for _, p := range s.Parameters {
		if p.Type.Kind == ast.TypeListOf {
			continue
		}
		if qn := astDataTypeEntity(p.Type); qn != "" {
			varEntity[p.Name] = qn
		}
	}
	type reverse struct {
		stmt   *ast.RetrieveStmt
		assoc  *assocLookupResult
		start  string
		result string
	}
	var candidates []reverse
	forEachFlowStatement(s.Body, func(stmt ast.MicroflowStatement) {
		switch st := stmt.(type) {
		case *ast.CreateObjectStmt:
			varEntity[st.Variable] = st.EntityType.String()
		case *ast.RetrieveStmt:
			if st.StartVariable == "" {
				if st.Limit == "1" && st.Offset == "" {
					varEntity[st.Variable] = st.Source.String()
				}
				return
			}
			start := varEntity[st.StartVariable]
			if start == "" {
				return
			}
			info := fb.lookupAssociation(st.Source.Module, st.Source.Name)
			if info == nil || info.Type != domainmodel.AssociationTypeReference ||
				info.childEntityQN == "" || info.parentEntityQN == "" {
				return
			}
			fromTo := fb.entityIsSubtypeOf(start, info.childEntityQN)
			fromFrom := fb.entityIsSubtypeOf(start, info.parentEntityQN)
			switch {
			case fromFrom:
				// Forward (or a self-reference, which is ambiguous): one object.
				varEntity[st.Variable] = info.childEntityQN
			case fromTo && info.Owner != domainmodel.AssociationOwnerBoth:
				candidates = append(candidates, reverse{st, info, start, info.parentEntityQN})
			case fromTo:
				varEntity[st.Variable] = info.parentEntityQN
			}
		}
	})
	if len(candidates) == 0 {
		return nil
	}

	usedAsObject := collectObjectInputVariables(s.Body)
	// collectObjectInputVariables does not read DECLARE initial values (the
	// builder's typing never needed them); a member access there is just as
	// much an object use.
	forEachFlowStatement(s.Body, func(stmt ast.MicroflowStatement) {
		if d, ok := stmt.(*ast.DeclareStmt); ok && d.InitialValue != nil {
			for v := range collectObjectInputVariables([]ast.MicroflowStatement{&ast.ReturnStmt{Value: d.InitialValue}}) {
				usedAsObject[v] = true
			}
		}
	})
	var errs []string
	for _, c := range candidates {
		v := c.stmt.Variable
		if !usedAsObject[v] {
			continue
		}
		assocQN := c.stmt.Source.String()
		errs = append(errs, fmt.Sprintf(
			"retrieve $%s from $%s/%s starts at %s, the TO end of reference %s (%s → %s, owner %s), "+
				"so it returns a List of %s — but $%s is then used as an object (member access or change). "+
				"mxbuild rejects that as CE0117. Keep the list and take one element "+
				"(`$One = head($%s)`), or retrieve the object directly: "+
				"`retrieve $%s from %s where [%s = $%s] limit 1`",
			v, c.stmt.StartVariable, assocQN, c.start, assocQN, c.assoc.parentEntityQN, c.assoc.childEntityQN,
			c.assoc.Owner, c.result, v, v, v, c.result, assocQN, c.stmt.StartVariable))
	}
	return errs
}
