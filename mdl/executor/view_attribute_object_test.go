// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/view-entity-examples FINDINGS §4. Declaring the association column as an
// attribute parsed as an ENUMERATION type naming an entity, and was written as
// one beside the association the column declares. Measured on 11.14.0: the
// two-part form fails the build with CE1613, the three-part form stops mx check
// loading the project. check -p reported "1 columns but 2 attributes" instead.

func parseViewEntity(t *testing.T, attrs string) *ast.CreateViewEntityStmt {
	t.Helper()
	src := `create view entity Trends.ReadingVE (
  ` + attrs + `
) as (
  from Trends.Reading as r
  join r/Trends.Reading_Meter/Trends.Meter as m
  select m.ID as MeterRef, r.Kwh as Kwh
);`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	s, ok := prog.Statements[0].(*ast.CreateViewEntityStmt)
	if !ok {
		t.Fatalf("not a CREATE VIEW ENTITY: %T", prog.Statements[0])
	}
	return s
}

func mdl080(s *ast.CreateViewEntityStmt) []string {
	var out []string
	for _, v := range ValidateViewAttributeDeclarations(s.Query.RawQuery, s.Attributes) {
		if v.RuleID == "MDL080" {
			out = append(out, v.Message)
		}
	}
	return out
}

func TestMDL080_TheAssociationColumnCannotBeDeclared(t *testing.T) {
	for name, attrs := range map[string]string{
		"entity type (reported)":    "MeterRef: Trends.Meter, Kwh: Decimal",
		"entity id type (reported)": "MeterRef: Trends.Meter.ID, Kwh: Decimal",
		"any type at all":           "MeterRef: String, Kwh: Decimal",
		"name differs only in case": "meterref: Trends.Meter, Kwh: Decimal",
	} {
		msgs := mdl080(parseViewEntity(t, attrs))
		if len(msgs) != 1 {
			t.Errorf("%s: MDL080 reported %d time(s), want 1: %q", name, len(msgs), msgs)
			continue
		}
		if !strings.Contains(msgs[0], "m.ID as MeterRef") || !strings.Contains(msgs[0], "Trends.Meter") {
			t.Errorf("%s: the message should name the column and its entity: %s", name, msgs[0])
		}
	}
}

// A three-part name is not an enumeration under any alias, so it needs no
// project to refuse.
func TestMDL080_AThreePartTypeIsNeverAnEnumeration(t *testing.T) {
	s := parseViewEntity(t, "Meter: Trends.Meter.ID, Kwh: Decimal")
	if msgs := mdl080(s); len(msgs) != 1 || !strings.Contains(msgs[0], "Trends.Meter.ID") {
		t.Errorf("want one MDL080 naming Trends.Meter.ID, got %q", msgs)
	}
}

func TestMDL080_LeavesTheWorkingFormsAlone(t *testing.T) {
	for name, attrs := range map[string]string{
		"association column not declared": "Kwh: Decimal",
		"a two-part enumeration type":     "Kwh: Decimal, Status: Trends.Status",
	} {
		if msgs := mdl080(parseViewEntity(t, attrs)); len(msgs) != 0 {
			t.Errorf("%s: spurious MDL080: %q", name, msgs)
		}
	}
}

func objectTypeFixture(t *testing.T) (*ExecContext, *mock.MockBackend) {
	t.Helper()
	mod := mkModule("Trends")
	meter := mkEntity(mod.ID, "Meter")
	dm := mkDomainModel(mod.ID, meter)
	enum := mkEnumeration(mod.ID, "Status", "On", "Off")
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{enum}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx, mb
}

// Under a name that is not an association column, only the project can tell an
// entity from an enumeration.
func TestViewAttributeEntityType_RefusesAnEntityAndKeepsAnEnumeration(t *testing.T) {
	ctx, _ := objectTypeFixture(t)
	attrs := []ast.ViewAttribute{
		{Name: "Meter", Type: ast.DataType{Kind: ast.TypeEnumeration, EnumRef: &ast.QualifiedName{Module: "Trends", Name: "Meter"}}},
		{Name: "Status", Type: ast.DataType{Kind: ast.TypeEnumeration, EnumRef: &ast.QualifiedName{Module: "Trends", Name: "Status"}}},
	}
	errs := viewAttributeEntityTypeErrors(ctx, "", attrs, nil)
	if len(errs) != 1 || !strings.Contains(errs[0], "attribute 'Meter'") || !strings.Contains(errs[0], "CE1613") {
		t.Fatalf("want one refusal for 'Meter' only, got %q", errs)
	}

	// An entity the script itself creates is not in the project yet.
	scriptOnly := []ast.ViewAttribute{
		{Name: "Site", Type: ast.DataType{Kind: ast.TypeEnumeration, EnumRef: &ast.QualifiedName{Module: "Trends", Name: "Site"}}},
	}
	if errs := viewAttributeEntityTypeErrors(ctx, "", scriptOnly, map[string]bool{"Trends.Site": true}); len(errs) != 1 {
		t.Errorf("an entity created earlier in the script must be refused too, got %q", errs)
	}
}

// exec --no-check skips ValidateProgram, so the handler has to refuse on its
// own — and before it writes anything, since exec cannot roll back.
func TestExecCreateViewEntity_RefusesADeclaredAssociationBeforeWriting(t *testing.T) {
	ctx, mb := objectTypeFixture(t)
	wrote := false
	mb.GetDomainModelFunc = func(model.ID) (*domainmodel.DomainModel, error) {
		wrote = true
		return nil, nil
	}
	mb.CreateViewEntitySourceDocumentFunc = func(model.ID, string, string, string, string) (model.ID, error) {
		wrote = true
		return "", nil
	}
	err := execCreateViewEntity(ctx, parseViewEntity(t, "MeterRef: Trends.Meter, Kwh: Decimal"))
	if err == nil || !strings.Contains(err.Error(), "m.ID as MeterRef") {
		t.Fatalf("want a refusal naming the association column, got %v", err)
	}
	if wrote {
		t.Error("the handler reached the domain model before refusing")
	}
	// One attribute, one problem: MDL080 already covers the entity type here.
	if n := strings.Count(err.Error(), "attribute 'MeterRef'"); n != 1 {
		t.Errorf("'MeterRef' refused %d times, want once:\n%v", n, err)
	}
}
