// SPDX-License-Identifier: Apache-2.0

package adapters

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
)

func qn(mod, name string) ast.QualifiedName { return ast.QualifiedName{Module: mod, Name: name} }

// assocStub answers one association, in one direction.
type assocStub struct{ assoc, from, to string }

func (a assocStub) AssociationTarget(assocQN, fromEntityQN string) (string, bool) {
	if assocQN == a.assoc && fromEntityQN == a.from {
		return a.to, true
	}
	return "", false
}

// TestBuildFlowScopeTypesLoopVariable is the unit-level statement of
// mendixlabs/mxcli#1100: the iterator takes the element type of the list it
// walks, or nothing downstream of it can be typed.
func TestBuildFlowScopeTypesLoopVariable(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.RetrieveStmt{Variable: "reqs", Source: qn("Probe", "Request")},
		&ast.LoopStmt{LoopVariable: "r", ListVariable: "reqs"},
	}
	s := buildFlowScope(body, nil, nil)
	if got := s.entities["r"]; got != "Probe.Request" {
		t.Errorf("loop variable typed %q, want Probe.Request", got)
	}
}

// TestBuildFlowScopeSeedsParametersFirst pins an ordering that is easy to get
// wrong and silent when it is: parameters must be in scope before the body
// walk, because a body statement can be typed FROM one. Appending them
// afterwards left every association retrieve off a parameter untyped, and with
// it every loop over the result.
func TestBuildFlowScopeSeedsParametersFirst(t *testing.T) {
	params := []ast.MicroflowParam{
		{Name: "T", Type: ast.DataType{Kind: ast.TypeEntity, EntityRef: &ast.QualifiedName{Module: "Probe", Name: "Request"}}},
	}
	body := []ast.MicroflowStatement{
		&ast.RetrieveStmt{Variable: "reps", Source: qn("Probe", "Request_Reporter"), StartVariable: "T"},
		&ast.LoopStmt{LoopVariable: "rep", ListVariable: "reps"},
	}
	s := buildFlowScope(body, params, assocStub{"Probe.Request_Reporter", "Probe.Request", "Probe.Reporter"})
	if got := s.entities["reps"]; got != "Probe.Reporter" {
		t.Errorf("association retrieve typed %q, want Probe.Reporter", got)
	}
	if got := s.entities["rep"]; got != "Probe.Reporter" {
		t.Errorf("loop over an association retrieve typed %q, want Probe.Reporter", got)
	}
}

// TestBuildFlowScopeTypesDeclaredPrimitives pins the other half of #1100: a
// rule that needs both operands typed (E004) was silent on `$out + <enum>`
// because a locally declared String was Unknown.
func TestBuildFlowScopeTypesDeclaredPrimitives(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.DeclareStmt{Variable: "out", Type: ast.DataType{Kind: ast.TypeString}},
		&ast.DeclareStmt{Variable: "n", Type: ast.DataType{Kind: ast.TypeInteger}},
		&ast.DeclareStmt{Variable: "obj", Type: ast.DataType{Kind: ast.TypeEntity, EntityRef: &ast.QualifiedName{Module: "Probe", Name: "Request"}}},
	}
	s := buildFlowScope(body, nil, nil)
	if got := s.kinds["out"]; got != exprcheck.KindString {
		t.Errorf("declared String typed %v, want KindString", got)
	}
	if got := s.kinds["n"]; got != exprcheck.KindInteger {
		t.Errorf("declared Integer typed %v, want KindInteger", got)
	}
	// An entity-typed DECLARE belongs to the entity half, not the kind half.
	if got := s.entities["obj"]; got != "Probe.Request" {
		t.Errorf("declared entity typed %q, want Probe.Request", got)
	}
	if _, ok := s.kinds["obj"]; ok {
		t.Errorf("an entity-typed DECLARE should have no primitive kind")
	}
}

// TestBuildFlowScopeCarriesElementTypeThroughListOperations pins which
// operations preserve the element type and which answer a Boolean instead. A
// loop is only as typed as the list it walks, so a FILTER that lost the entity
// would leave the fix covering one spelling of the same loop.
func TestBuildFlowScopeCarriesElementTypeThroughListOperations(t *testing.T) {
	for _, tc := range []struct {
		op        ast.ListOperationType
		wantQN    string
		wantKind  exprcheck.TypeKind
		hasEntity bool
	}{
		{ast.ListOpFilter, "Probe.Request", exprcheck.KindUnknown, true},
		{ast.ListOpSort, "Probe.Request", exprcheck.KindUnknown, true},
		{ast.ListOpHead, "Probe.Request", exprcheck.KindUnknown, true},
		{ast.ListOpRange, "Probe.Request", exprcheck.KindUnknown, true},
		{ast.ListOpContains, "", exprcheck.KindBoolean, false},
		{ast.ListOpEquals, "", exprcheck.KindBoolean, false},
	} {
		body := []ast.MicroflowStatement{
			&ast.RetrieveStmt{Variable: "reqs", Source: qn("Probe", "Request")},
			&ast.ListOperationStmt{OutputVariable: "out", Operation: tc.op, InputVariable: "reqs"},
		}
		s := buildFlowScope(body, nil, nil)
		if got := s.entities["out"]; got != tc.wantQN {
			t.Errorf("%v: entity %q, want %q", tc.op, got, tc.wantQN)
		}
		if tc.wantKind != exprcheck.KindUnknown {
			if got := s.kinds["out"]; got != tc.wantKind {
				t.Errorf("%v: kind %v, want %v", tc.op, got, tc.wantKind)
			}
		}
	}
}

// TestBuildFlowScopeWalksErrorHandlerBodies pins that a variable introduced in
// an ON ERROR handler is typed like any other. The handler's body is ordinary
// statements; leaving it out made moving a statement into one an exemption.
func TestBuildFlowScopeWalksErrorHandlerBodies(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.RetrieveStmt{
			Variable: "reqs", Source: qn("Probe", "Request"),
			ErrorHandling: &ast.ErrorHandlingClause{Body: []ast.MicroflowStatement{
				&ast.CreateObjectStmt{Variable: "fallback", EntityType: qn("Probe", "Request")},
			}},
		},
	}
	s := buildFlowScope(body, nil, nil)
	if got := s.entities["fallback"]; got != "Probe.Request" {
		t.Errorf("a variable created in an ON ERROR body typed %q, want Probe.Request", got)
	}
}

// TestBuildFlowScopeLeavesAnUnresolvableLoopAlone is the failure direction. A
// loop over a list nothing typed, or over an association path the AST does not
// record (`LOOP $r IN $T/Mod.Assoc` leaves ListVariable empty), must produce no
// entry — a guess here is a false positive on code that builds.
func TestBuildFlowScopeLeavesAnUnresolvableLoopAlone(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.LoopStmt{LoopVariable: "r", ListVariable: "unknown"},
		&ast.LoopStmt{LoopVariable: "p", ListVariable: ""},
	}
	s := buildFlowScope(body, nil, nil)
	for _, v := range []string{"r", "p"} {
		if qn, ok := s.entities[v]; ok {
			t.Errorf("an unresolvable loop variable %q was typed %q", v, qn)
		}
	}
}

// TestBuildFlowScopeDoesNotCallAnAmbiguousNameAnEnumeration pins the one place
// a guess would be wrong in a way that shows. `DECLARE $o Mod.Person` parses as
// TypeEnumeration with EnumRef set — the visitor cannot tell it from an
// enumeration (see CLAUDE.md) — so the ENTITY guess is recorded (it resolves no
// attributes if wrong, costing nothing) but the KIND is not, or a rule would
// report an object as an "Enumeration". `ENUM Mod.Status` sets ExplicitEnum and
// is unambiguous, so it does get a kind.
func TestBuildFlowScopeDoesNotCallAnAmbiguousNameAnEnumeration(t *testing.T) {
	ambiguous := ast.DataType{Kind: ast.TypeEnumeration, EnumRef: &ast.QualifiedName{Module: "Probe", Name: "Person"}}
	explicit := ast.DataType{Kind: ast.TypeEnumeration, EnumRef: &ast.QualifiedName{Module: "Probe", Name: "Status"}, ExplicitEnum: true}

	s := buildFlowScope([]ast.MicroflowStatement{
		&ast.DeclareStmt{Variable: "maybe", Type: ambiguous},
		&ast.DeclareStmt{Variable: "sure", Type: explicit},
	}, nil, nil)

	if got := s.entities["maybe"]; got != "Probe.Person" {
		t.Errorf("the entity guess was dropped: %q", got)
	}
	if k, ok := s.kinds["maybe"]; ok {
		t.Errorf("an ambiguous bare name was typed %v; it must have no kind", k)
	}
	if got := s.kinds["sure"]; got != exprcheck.KindEnumeration {
		t.Errorf("an explicit ENUM typed %v, want KindEnumeration", got)
	}
}
