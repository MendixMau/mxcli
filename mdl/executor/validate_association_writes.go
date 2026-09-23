// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// Association writes from the wrong side (CE0854), and add/remove on a plain
// reference.
//
// A CREATE or CHANGE member that names an association writes the reference on
// the object being created or changed. Mendix stores an association on its
// OWNER — the FROM entity (`ParentPointer`) under `owner Default`, either end
// under `owner Both` — so writing it from the other end is a model mxbuild
// rejects:
//
//	CE0854 Association 'UserGroups.GuestGroup_Guests' is not reachable from
//	entity 'UserGroups.Guest'.
//
// Nothing before mxbuild said so: `mxcli check --references` resolves the
// association NAME, which exists, and `exec` wrote the member as authored. The
// member-change syntax cannot express "write it from the other side", so the
// fix is always the same: change the owner object instead —
// `change $GuestGroup (add $Guest to UserGroups.GuestGroup_Guests)`.
//
// The same walk refuses `add … to` / `remove … from` against a reference
// (non-set) association, whose MemberChange type Mendix only accepts as Set.
//
// Two callers, split so a violation is reported once:
//   - ValidateAssociationWrites (no project): associations the SCRIPT declares.
//     It only reports what it can prove — an entity whose inheritance chain
//     leaves the script might specialise the owner, so it is left alone.
//   - validateFlowAssociationWrites (--references and exec): associations the
//     PROJECT stores and the script does not redeclare.

// assocWriteEnds is what the check needs to know about one association.
type assocWriteEnds struct {
	QN        string // Module.Association
	From      string // owner under `owner Default` (the stored ParentPointer)
	To        string
	IsSet     bool // reference set
	OwnerBoth bool
}

// inheritanceAnswer is a tri-state "is child a (specialisation of) ancestor".
type inheritanceAnswer int

const (
	inheritsNo inheritanceAnswer = iota
	inheritsYes
	inheritsUnknown
)

// associationWriteProblem returns the error for one member write, or "" when
// the write is fine or cannot be decided. isA answers "entity a is, or
// specialises, entity b".
func associationWriteProblem(varName, entityQN string, ch ast.ChangeItem, a *assocWriteEnds,
	isA func(a, b string) inheritanceAnswer) string {
	if a == nil {
		return ""
	}
	obj := "$" + varName
	if varName == "" {
		obj = "the new " + entityQN
	}
	if ch.Kind != ast.MemberChangeSet && !a.IsSet {
		return fmt.Sprintf("%s on %s: %s is a reference, not a reference set — "+
			"add/remove applies only to a reference set; write '%s = <object>' instead",
			memberForm(ch), obj, a.QN, ch.Attribute)
	}

	fromSide := isA(entityQN, a.From)
	if fromSide == inheritsYes {
		return ""
	}
	toSide := isA(entityQN, a.To)
	if a.OwnerBoth && toSide == inheritsYes {
		return ""
	}
	if fromSide == inheritsUnknown || toSide == inheritsUnknown {
		return ""
	}

	if toSide != inheritsYes {
		return fmt.Sprintf("association %s is not reachable from entity %s (%s writes it on %s) — "+
			"it runs from %s to %s; mxbuild rejects this with CE0854",
			a.QN, entityQN, memberForm(ch), obj, a.From, a.To)
	}
	// The object is on the TO end of a Default-owned association: the right
	// statement changes the FROM object instead.
	hint := fmt.Sprintf("change $<%s> (%s = %s)", shortEntity(a.From), a.QN, "$"+nonEmpty(varName, "<"+shortEntity(entityQN)+">"))
	if a.IsSet {
		switch ch.Kind {
		case ast.MemberChangeRemove:
			hint = fmt.Sprintf("change $<%s> (remove %s from %s)", shortEntity(a.From), "$"+nonEmpty(varName, "<"+shortEntity(entityQN)+">"), a.QN)
		default:
			hint = fmt.Sprintf("change $<%s> (add %s to %s)", shortEntity(a.From), "$"+nonEmpty(varName, "<"+shortEntity(entityQN)+">"), a.QN)
		}
	}
	return fmt.Sprintf("association %s is not reachable from entity %s (%s writes it on %s) — "+
		"it is owned by %s (owner Default), so only a %s object can write it; mxbuild rejects this with CE0854. "+
		"Write it from the owner side: %s — or declare the association 'owner Both'",
		a.QN, entityQN, memberForm(ch), obj, a.From, shortEntity(a.From), hint)
}

func memberForm(ch ast.ChangeItem) string {
	switch ch.Kind {
	case ast.MemberChangeAdd:
		return "'add ... to " + ch.Attribute + "'"
	case ast.MemberChangeRemove:
		return "'remove ... from " + ch.Attribute + "'"
	}
	return "'" + ch.Attribute + " = ...'"
}

func shortEntity(qn string) string {
	if i := strings.LastIndex(qn, "."); i >= 0 {
		return qn[i+1:]
	}
	return qn
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// memberAssociationKey returns the lower-cased Module.Association a member
// name would resolve to on an entity — the member's own qualifier when it has
// exactly one, the entity's module when it is bare — and false for a member
// that cannot be an association (Module.Entity.Attribute).
func memberAssociationKey(entityQN, member string) (module, name string, ok bool) {
	switch strings.Count(member, ".") {
	case 0:
		dot := strings.Index(entityQN, ".")
		if dot < 0 {
			return "", "", false
		}
		return entityQN[:dot], member, true
	case 1:
		dot := strings.Index(member, ".")
		return member[:dot], member[dot+1:], true
	}
	return "", "", false
}

// flowAssociationWriteErrors walks one flow body and reports every member
// write resolve() can place on an association.
func flowAssociationWriteErrors(params []ast.MicroflowParam, body []ast.MicroflowStatement,
	resolve func(entityQN, member string) *assocWriteEnds,
	isA func(a, b string) inheritanceAnswer) []string {
	varTypes := make(map[string]string)
	declaredVars := make(map[string]string)
	for _, p := range params {
		if p.Type.EntityRef != nil && p.Type.EntityRef.Module != "" {
			qn := p.Type.EntityRef.Module + "." + p.Type.EntityRef.Name
			if p.Type.Kind == ast.TypeListOf {
				qn = "List of " + qn
			}
			varTypes[p.Name] = qn
		} else {
			declaredVars[p.Name] = p.Type.Kind.String()
		}
	}
	var out []string
	seen := map[string]bool{}
	fb := &flowBuilder{varTypes: varTypes, declaredVars: declaredVars}
	fb.memberWrite = func(varName, entityQN string, ch ast.ChangeItem) {
		msg := associationWriteProblem(varName, entityQN, ch, resolve(entityQN, ch.Attribute), isA)
		if msg != "" && !seen[msg] {
			seen[msg] = true
			out = append(out, msg)
		}
	}
	fb.validateStatements(body)
	return out
}

// flowParts returns the name, parameters and body of a flow-defining
// statement, and false for anything else or an excluded flow.
func flowParts(stmt ast.Statement) (string, string, []ast.MicroflowParam, []ast.MicroflowStatement, bool) {
	switch s := stmt.(type) {
	case *ast.CreateMicroflowStmt:
		return "microflow", s.Name.String(), s.Parameters, s.Body, !s.Excluded
	case *ast.CreateNanoflowStmt:
		return "nanoflow", s.Name.String(), s.Parameters, s.Body, !s.Excluded
	case *ast.CreateRuleStmt:
		return "rule", s.Name.String(), s.Parameters, s.Body, !s.Excluded
	}
	return "", "", nil, nil, false
}

// ValidateAssociationWrites is the no-project half: association writes checked
// against the associations the script itself declares (MDL-ASSOC01).
func ValidateAssociationWrites(prog *ast.Program) []linter.Violation {
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	if len(sc.assocEnds) == 0 {
		return nil
	}
	resolve := func(entityQN, member string) *assocWriteEnds {
		mod, name, ok := memberAssociationKey(entityQN, member)
		if !ok {
			return nil
		}
		return sc.assocEnds[strings.ToLower(mod+"."+name)]
	}
	isA := func(a, b string) inheritanceAnswer {
		return scriptInherits(sc, a, b, nil)
	}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		kind, name, params, body, ok := flowParts(stmt)
		if !ok {
			continue
		}
		loc := linter.Location{DocumentType: kind, DocumentName: name}
		if dot := strings.Index(name, "."); dot >= 0 {
			loc.Module, loc.DocumentName = name[:dot], name[dot+1:]
		}
		for _, msg := range flowAssociationWriteErrors(params, body, resolve, isA) {
			out = append(out, linter.Violation{
				RuleID:   "MDL-ASSOC01",
				Severity: linter.SeverityError,
				Location: loc,
				Message:  fmt.Sprintf("%s '%s': %s", kind, name, msg),
				Suggestion: "An association is written on its owner. Change the FROM-side object " +
					"(`change $Owner (add $Member to Module.Assoc)`), or declare the association `owner Both`.",
			})
		}
	}
	return out
}

// scriptInherits answers "a is, or specialises, b" from the script's own
// entity declarations, falling back to project() (when given) for an entity the
// script does not declare. Without a fallback, a chain that leaves the script
// is unknown.
func scriptInherits(sc *scriptContext, a, b string, project func(qn string) (string, bool)) inheritanceAnswer {
	seen := map[string]bool{}
	for qn := a; qn != ""; {
		key := strings.ToLower(qn)
		if strings.EqualFold(qn, b) {
			return inheritsYes
		}
		if seen[key] {
			return inheritsNo
		}
		seen[key] = true
		parent, ok := sc.entityGeneralizations[key]
		if !ok {
			if project == nil {
				return inheritsUnknown
			}
			if parent, ok = project(qn); !ok {
				return inheritsUnknown
			}
		}
		qn = parent
	}
	return inheritsNo
}

// validateFlowAssociationWrites is the project half: association writes
// checked against the associations the project stores. Associations the script
// declares are left to ValidateAssociationWrites, so nothing is reported twice.
func validateFlowAssociationWrites(ctx *ExecContext, params []ast.MicroflowParam,
	body []ast.MicroflowStatement, sc *scriptContext) []string {
	if !ctx.Connected() || ctx.Backend == nil || len(body) == 0 {
		return nil
	}
	cache := map[string]*domainmodel.DomainModel{}
	domainModel := func(module string) *domainmodel.DomainModel {
		key := strings.ToLower(module)
		if dm, ok := cache[key]; ok {
			return dm
		}
		var dm *domainmodel.DomainModel
		if mod, err := ctx.Backend.GetModuleByName(module); err == nil && mod != nil {
			if d, err := ctx.Backend.GetDomainModel(mod.ID); err == nil {
				dm = d
			}
		}
		cache[key] = dm
		return dm
	}
	resolve := func(entityQN, member string) *assocWriteEnds {
		mod, name, ok := memberAssociationKey(entityQN, member)
		if !ok {
			return nil
		}
		if sc != nil && sc.assocEnds[strings.ToLower(mod+"."+name)] != nil {
			return nil // script-declared: ValidateAssociationWrites owns it
		}
		dm := domainModel(mod)
		if dm == nil {
			return nil
		}
		entityName := func(id string) string {
			for _, e := range dm.Entities {
				if string(e.ID) == id {
					return mod + "." + e.Name
				}
			}
			return ""
		}
		for _, a := range dm.Associations {
			if a.Name != name {
				continue
			}
			from, to := entityName(string(a.ParentID)), entityName(string(a.ChildID))
			if from == "" || to == "" {
				return nil
			}
			return &assocWriteEnds{QN: mod + "." + a.Name, From: from, To: to,
				IsSet: a.Type == domainmodel.AssociationTypeReferenceSet, OwnerBoth: a.Owner == domainmodel.AssociationOwnerBoth}
		}
		for _, a := range dm.CrossAssociations {
			if a.Name != name {
				continue
			}
			from := entityName(string(a.ParentID))
			if from == "" || a.ChildRef == "" {
				return nil
			}
			return &assocWriteEnds{QN: mod + "." + a.Name, From: from, To: a.ChildRef,
				IsSet: a.Type == domainmodel.AssociationTypeReferenceSet, OwnerBoth: a.Owner == domainmodel.AssociationOwnerBoth}
		}
		return nil
	}
	project := func(qn string) (string, bool) {
		e, ok := findEntityByQN(ctx.Backend, qn)
		if !ok || e == nil {
			return "", false
		}
		return e.GeneralizationRef, true
	}
	if sc == nil {
		sc = newScriptContext()
	}
	isA := func(a, b string) inheritanceAnswer {
		return scriptInherits(sc, a, b, project)
	}
	return flowAssociationWriteErrors(params, body, resolve, isA)
}
