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
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The fixture is the reported model: GuestGroup —GuestGroup_Guests→ Guest,
// a reference set owned (Default) by GuestGroup, the FROM end.
const guestGroupsAssocMDL = `
create persistent entity UserGroups.GuestGroup (Name: String(100));
create persistent entity UserGroups.Guest (Email: String(200));
create association UserGroups.GuestGroup_Guests from UserGroups.GuestGroup to UserGroups.Guest type ReferenceSet owner Default;
`

func parseProgram(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	return prog
}

func firstMicroflow(t *testing.T, prog *ast.Program) *ast.CreateMicroflowStmt {
	t.Helper()
	for _, s := range prog.Statements {
		if mf, ok := s.(*ast.CreateMicroflowStmt); ok {
			return mf
		}
	}
	t.Fatal("no microflow in program")
	return nil
}

// --- (a) add/remove parse, build and describe -----------------------------

func TestMemberChange_AddRemoveParse(t *testing.T) {
	prog := parseProgram(t, `
create microflow UserGroups.M ($G: UserGroups.GuestGroup, $X: UserGroups.Guest)
begin
  change $G (add $X to GuestGroup_Guests, remove $X from UserGroups.GuestGroup_Guests, Name = 'n');
end;`)
	mf := firstMicroflow(t, prog)
	ch := mf.Body[0].(*ast.ChangeObjectStmt).Changes
	if len(ch) != 3 {
		t.Fatalf("want 3 changes, got %d", len(ch))
	}
	want := []struct {
		attr string
		kind ast.MemberChangeKind
	}{
		{"GuestGroup_Guests", ast.MemberChangeAdd},
		{"UserGroups.GuestGroup_Guests", ast.MemberChangeRemove},
		{"Name", ast.MemberChangeSet},
	}
	for i, w := range want {
		if ch[i].Attribute != w.attr || ch[i].Kind != w.kind {
			t.Errorf("change %d = (%q, %v), want (%q, %v)", i, ch[i].Attribute, ch[i].Kind, w.attr, w.kind)
		}
	}
}

func guestGroupBackend() *mock.MockBackend {
	modID := model.ID("usergroups-module")
	return &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == "UserGroups" {
				return &model.Module{BaseElement: model.BaseElement{ID: modID}, Name: name}, nil
			}
			return nil, nil
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			if id != modID {
				return nil, nil
			}
			group := &domainmodel.Entity{BaseElement: model.BaseElement{ID: "e-group"}, Name: "GuestGroup"}
			guest := &domainmodel.Entity{BaseElement: model.BaseElement{ID: "e-guest"}, Name: "Guest"}
			return &domainmodel.DomainModel{
				ContainerID: modID,
				Entities:    []*domainmodel.Entity{group, guest},
				Associations: []*domainmodel.Association{{
					Name:     "GuestGroup_Guests",
					ParentID: group.ID, // FROM = owner
					ChildID:  guest.ID,
					Type:     domainmodel.AssociationTypeReferenceSet,
					Owner:    domainmodel.AssociationOwnerDefault,
				}},
			}, nil
		},
	}
}

func TestMemberChange_BuilderWritesAddRemoveType(t *testing.T) {
	prog := parseProgram(t, `
create microflow UserGroups.M ($G: UserGroups.GuestGroup, $X: UserGroups.Guest)
begin
  change $G (add $X to GuestGroup_Guests, remove $X from GuestGroup_Guests, Name = 'n');
  $G2 = create UserGroups.GuestGroup (add $X to GuestGroup_Guests);
end;`)
	mf := firstMicroflow(t, prog)
	fb := &flowBuilder{backend: guestGroupBackend(), varTypes: map[string]string{
		"G": "UserGroups.GuestGroup", "X": "UserGroups.Guest"}}

	fb.addChangeObjectAction(mf.Body[0].(*ast.ChangeObjectStmt))
	fb.addCreateObjectAction(mf.Body[1].(*ast.CreateObjectStmt))
	if errs := fb.GetErrors(); len(errs) > 0 {
		t.Fatalf("unexpected builder errors: %v", errs)
	}
	change := fb.objects[0].(*microflows.ActionActivity).Action.(*microflows.ChangeObjectAction)
	wantTypes := []microflows.MemberChangeType{
		microflows.MemberChangeTypeAdd, microflows.MemberChangeTypeRemove, microflows.MemberChangeTypeSet}
	for i, w := range wantTypes {
		if got := change.Changes[i].Type; got != w {
			t.Errorf("change member %d Type = %q, want %q", i, got, w)
		}
	}
	if change.Changes[0].AssociationQualifiedName != "UserGroups.GuestGroup_Guests" {
		t.Errorf("add member should resolve to the association, got %+v", change.Changes[0])
	}
	create := fb.objects[1].(*microflows.ActionActivity).Action.(*microflows.CreateObjectAction)
	if got := create.InitialMembers[0].Type; got != microflows.MemberChangeTypeAdd {
		t.Errorf("create initial member Type = %q, want Add", got)
	}
}

func TestMemberChange_AddOnAttributeIsRefused(t *testing.T) {
	prog := parseProgram(t, `
create microflow UserGroups.M ($G: UserGroups.GuestGroup)
begin
  change $G (add 'x' to Name);
end;`)
	mf := firstMicroflow(t, prog)
	fb := &flowBuilder{backend: guestGroupBackend(), varTypes: map[string]string{"G": "UserGroups.GuestGroup"}}
	fb.addChangeObjectAction(mf.Body[0].(*ast.ChangeObjectStmt))
	errs := strings.Join(fb.GetErrors(), "\n")
	if !strings.Contains(errs, "not an association") {
		t.Fatalf("add on an attribute must be refused, got errors: %q", errs)
	}
}

func TestMemberChange_DescribeRendersAddRemove(t *testing.T) {
	e := newTestExecutor()
	action := &microflows.ChangeObjectAction{
		ChangeVariable: "G",
		Changes: []*microflows.MemberChange{
			{Type: microflows.MemberChangeTypeAdd, AssociationQualifiedName: "UserGroups.GuestGroup_Guests", Value: "$X"},
			{Type: microflows.MemberChangeTypeRemove, AssociationQualifiedName: "UserGroups.GuestGroup_Guests", Value: "$Y"},
			{Type: microflows.MemberChangeTypeSet, AttributeQualifiedName: "UserGroups.GuestGroup.Name", Value: "'n'"},
		},
	}
	got := e.formatAction(action, nil, nil)
	for _, want := range []string{
		"add $X to UserGroups.GuestGroup_Guests",
		"remove $Y from UserGroups.GuestGroup_Guests",
		"Name = 'n'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("describe output %q is missing %q", got, want)
		}
	}
	// The describe output must re-parse as the same kinds, or describe → exec
	// turns an Add back into a Set.
	src := "create microflow UserGroups.M ($G: UserGroups.GuestGroup, $X: UserGroups.Guest, $Y: UserGroups.Guest)\nbegin\n  " +
		got + "\nend;"
	mf := firstMicroflow(t, parseProgram(t, src))
	ch := mf.Body[0].(*ast.ChangeObjectStmt).Changes
	if ch[0].Kind != ast.MemberChangeAdd || ch[1].Kind != ast.MemberChangeRemove || ch[2].Kind != ast.MemberChangeSet {
		t.Errorf("describe output did not round-trip kinds: %+v", ch)
	}
}

// --- (b) CE0854: writing an association from its non-owner end -----------

func assocViolations(t *testing.T, src string) []string {
	t.Helper()
	var out []string
	for _, v := range ValidateProgram(parseProgram(t, src), "") {
		if v.RuleID == "MDL-ASSOC01" {
			out = append(out, v.Message)
		}
	}
	return out
}

func TestAssociationWrites_NonOwnerSideIsCE0854(t *testing.T) {
	cases := map[string]string{
		"create from the TO end": `
create microflow UserGroups.AddGuest ($GuestGroup: UserGroups.GuestGroup, $Email: String)
begin
  $NewGuest = create UserGroups.Guest (Email = $Email, GuestGroup_Guests = $GuestGroup);
end;`,
		"change from the TO end": `
create microflow UserGroups.Move ($Guest: UserGroups.Guest, $Remaining: UserGroups.GuestGroup)
begin
  change $Guest (GuestGroup_Guests = $Remaining);
end;`,
	}
	for name, mf := range cases {
		t.Run(name, func(t *testing.T) {
			got := assocViolations(t, guestGroupsAssocMDL+mf)
			if len(got) != 1 {
				t.Fatalf("want one MDL-ASSOC01 violation, got %d: %v", len(got), got)
			}
			for _, want := range []string{"CE0854", "not reachable from entity UserGroups.Guest", "add $", "to UserGroups.GuestGroup_Guests"} {
				if !strings.Contains(got[0], want) {
					t.Errorf("message should contain %q (the fix names the owner side), got: %s", want, got[0])
				}
			}
		})
	}
}

// The control: the same association written from its owner is fine, so the
// test above cannot be passing on the mere presence of an association write.
func TestAssociationWrites_OwnerSideIsAccepted(t *testing.T) {
	got := assocViolations(t, guestGroupsAssocMDL+`
create microflow UserGroups.AddGuest ($GuestGroup: UserGroups.GuestGroup, $Email: String)
begin
  $NewGuest = create UserGroups.Guest (Email = $Email);
  change $GuestGroup (add $NewGuest to GuestGroup_Guests);
  change $GuestGroup (remove $NewGuest from UserGroups.GuestGroup_Guests);
  $G2 = create UserGroups.GuestGroup (GuestGroup_Guests = $NewGuest);
end;`)
	if len(got) != 0 {
		t.Fatalf("owner-side writes must not be reported, got: %v", got)
	}
}

func TestAssociationWrites_OwnerBothAcceptsEitherEnd(t *testing.T) {
	src := strings.Replace(guestGroupsAssocMDL, "owner Default", "owner Both", 1) + `
create microflow UserGroups.Move ($Guest: UserGroups.Guest, $G: UserGroups.GuestGroup)
begin
  change $Guest (GuestGroup_Guests = $G);
end;`
	if got := assocViolations(t, src); len(got) != 0 {
		t.Fatalf("owner Both is writable from either end, got: %v", got)
	}
}

func TestAssociationWrites_AddOnReferenceIsRefused(t *testing.T) {
	src := strings.Replace(guestGroupsAssocMDL, "type ReferenceSet", "type Reference", 1) + `
create microflow UserGroups.M ($G: UserGroups.GuestGroup, $X: UserGroups.Guest)
begin
  change $G (add $X to GuestGroup_Guests);
end;`
	got := assocViolations(t, src)
	if len(got) != 1 || !strings.Contains(got[0], "is a reference, not a reference set") {
		t.Fatalf("add on a plain reference must be refused, got: %v", got)
	}
}

// A specialisation of the owner inherits its associations, so writing from it
// is fine; an entity whose inheritance leaves the script is not guessed at.
func TestAssociationWrites_Specialisation(t *testing.T) {
	src := guestGroupsAssocMDL + `
create persistent entity UserGroups.VipGroup extends UserGroups.GuestGroup (Level: Integer);
create microflow UserGroups.M ($G: UserGroups.VipGroup, $X: UserGroups.Guest)
begin
  change $G (add $X to GuestGroup_Guests);
end;`
	if got := assocViolations(t, src); len(got) != 0 {
		t.Fatalf("a specialisation of the owner may write the association, got: %v", got)
	}
}

// --- (b) project half: association stored in the project ------------------

func TestAssociationWrites_ProjectStoredAssociation(t *testing.T) {
	ctx, _ := newMockCtx(t, withBackend(guestGroupBackend()))
	sc := newScriptContext()
	sc.modules["UserGroups"] = true
	// The entities exist; only the association lookup is under test here.
	sc.entities["UserGroups.Guest"] = true
	sc.entities["UserGroups.GuestGroup"] = true

	bad := firstMicroflow(t, parseProgram(t, `
create microflow UserGroups.AddGuest ($GuestGroup: UserGroups.GuestGroup, $Email: String)
begin
  $NewGuest = create UserGroups.Guest (Email = $Email, GuestGroup_Guests = $GuestGroup);
end;`))
	err := validateWithContext(ctx, bad, sc)
	if err == nil || !strings.Contains(err.Error(), "CE0854") {
		t.Fatalf("--references must report the non-owner write against the stored association, got: %v", err)
	}

	good := firstMicroflow(t, parseProgram(t, `
create microflow UserGroups.AddGuest ($GuestGroup: UserGroups.GuestGroup, $Email: String)
begin
  $NewGuest = create UserGroups.Guest (Email = $Email);
  change $GuestGroup (add $NewGuest to GuestGroup_Guests);
end;`))
	if err := validateWithContext(ctx, good, sc); err != nil {
		t.Fatalf("owner-side write must pass --references, got: %v", err)
	}
}
