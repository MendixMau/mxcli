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

// reverseRefCtx is module M with M.Child_Parent, a Reference from M.Child to
// M.Parent whose owner is given — the shape of UserGroups.GuestGroup_App
// (GuestGroup → App, owner Default) in the marketplace-rnd report.
func reverseRefCtx(t *testing.T, typ domainmodel.AssociationType, owner domainmodel.AssociationOwner) *ExecContext {
	t.Helper()
	mod := mkModule("M")
	child := mkEntity(mod.ID, "Child")
	parent := mkEntity(mod.ID, "Parent")
	assoc := mkAssociation(mod.ID, "Child_Parent", child.ID, parent.ID)
	assoc.Type = typ
	assoc.Owner = owner
	dm := mkDomainModel(mod.ID, child, parent)
	dm.Associations = []*domainmodel.Association{assoc}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == "M" {
				return mod, nil
			}
			return nil, nil
		},
		GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx
}

func TestValidateReverseReferenceRetrieves(t *testing.T) {
	const (
		ref  = domainmodel.AssociationTypeReference
		set  = domainmodel.AssociationTypeReferenceSet
		dflt = domainmodel.AssociationOwnerDefault
		both = domainmodel.AssociationOwnerBoth
	)
	cases := []struct {
		name    string
		typ     domainmodel.AssociationType
		owner   domainmodel.AssociationOwner
		body    string
		wantErr bool
	}{
		{"reverse reference used as object (the report)", ref, dflt, `
  retrieve $C from $P/M.Child_Parent;
  declare $S String = $C/Name;`, true},
		{"reverse reference changed as object", ref, dflt, `
  retrieve $C from $P/M.Child_Parent;
  change $C (Name = 'x');`, true},
		{"reverse reference used as a list", ref, dflt, `
  retrieve $C from $P/M.Child_Parent;
  declare $N Integer = count($C);`, false},
		{"forward reference used as object", ref, dflt, `
  $K = create M.Child (Name = 'k');
  retrieve $Q from $K/M.Child_Parent;
  declare $S String = $Q/Name;`, false},
		{"owner Both reverse is a single object", ref, both, `
  retrieve $C from $P/M.Child_Parent;
  declare $S String = $C/Name;`, false},
		{"reference set reverse used as object is not this rule", set, dflt, `
  retrieve $C from $P/M.Child_Parent;
  declare $N Integer = count($C);`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			script := "create microflow M.MF ($P: M.Parent) begin" + c.body + "\nend;\n/\n"
			prog, errs := visitor.Build(script)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			mf, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
			if !ok {
				t.Fatalf("got %T", prog.Statements[0])
			}
			got := validateReverseReferenceRetrieves(reverseRefCtx(t, c.typ, c.owner), mf, nil)
			if (len(got) > 0) != c.wantErr {
				t.Fatalf("errors = %v, want error: %v", got, c.wantErr)
			}
			if c.wantErr {
				msg := strings.Join(got, "\n")
				for _, w := range []string{"$C", "List of M.Child", "CE0117", "head("} {
					if !strings.Contains(msg, w) {
						t.Errorf("message lacks %q:\n%s", w, msg)
					}
				}
			}
		})
	}
}
