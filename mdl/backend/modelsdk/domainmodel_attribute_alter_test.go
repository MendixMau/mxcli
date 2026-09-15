// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

// AddAttribute / UpdateAttribute were the two FullBackend methods the codec
// engine left to the stub. They were unreachable — and so unimplemented — only
// because api/ was their sole caller and api/ held a concrete sdk/mpr writer
// instead of a backend. Routing api/ through the abstraction is what makes them
// reachable, so they are implemented and pinned here.
//
// The properties worth pinning are the ones a rebuild gets wrong by default:
// the attribute keeps its stored $ID (the runtime keys on identity, see
// CLAUDE.md), and the list keeps its order (the generated list offers only
// Append and Remove, so a naive replace moves the edited attribute to the end).

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"

	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// entityWithAttributes creates a scratch entity and returns the ids needed to
// mutate it, so each test starts from a known shape rather than from whatever
// the fixture happens to carry.
func entityWithAttributes(t *testing.T, b *Backend, names ...string) (dmID, entID string) {
	t.Helper()
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	dms, err := b.ListDomainModels()
	if err != nil {
		t.Fatalf("ListDomainModels: %v", err)
	}
	for _, dm := range dms {
		if dm.ContainerID == mod.ID {
			dmID = string(dm.ID)
			break
		}
	}
	if dmID == "" {
		t.Fatal("no domain model in MyFirstModule")
	}

	ent := &domainmodel.Entity{Name: "ZzAttrHost", Persistable: true}
	for _, n := range names {
		ent.Attributes = append(ent.Attributes, &domainmodel.Attribute{
			Name: n, Type: &domainmodel.StringAttributeType{Length: 200},
		})
	}
	if err := b.CreateEntity(model.ID(dmID), ent); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	for _, e := range dm.Entities {
		if e.Name == "ZzAttrHost" {
			return dmID, string(e.ID)
		}
	}
	t.Fatal("created entity not found on read-back")
	return "", ""
}

func attrNames(t *testing.T, b *Backend, entID string) []string {
	t.Helper()
	mod, _ := b.GetModuleByName("MyFirstModule")
	dm, err := b.GetDomainModel(mod.ID)
	if err != nil {
		t.Fatalf("GetDomainModel: %v", err)
	}
	for _, e := range dm.Entities {
		if string(e.ID) == entID {
			var out []string
			for _, a := range e.Attributes {
				out = append(out, a.Name)
			}
			return out
		}
	}
	t.Fatalf("entity %s not found", entID)
	return nil
}

func TestAddAttribute_AppendsAndPersists(t *testing.T) {
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	dmID, entID := entityWithAttributes(t, b, "Alpha", "Beta")
	err := b.AddAttribute(model.ID(dmID), model.ID(entID), &domainmodel.Attribute{
		Name: "Gamma", Type: &domainmodel.IntegerAttributeType{},
	})
	if err != nil {
		t.Fatalf("AddAttribute: %v", err)
	}
	if got, want := attrNames(t, b, entID), []string{"Alpha", "Beta", "Gamma"}; !sameStrings(got, want) {
		t.Errorf("attributes = %v, want %v", got, want)
	}
}

// A duplicate name is refused rather than written. Two attributes of one name is
// a model Studio Pro cannot show and mxbuild rejects late, so the backend is the
// cheaper place to say no.
func TestAddAttribute_RefusesDuplicateName(t *testing.T) {
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	dmID, entID := entityWithAttributes(t, b, "Alpha")
	err := b.AddAttribute(model.ID(dmID), model.ID(entID), &domainmodel.Attribute{
		Name: "Alpha", Type: &domainmodel.IntegerAttributeType{},
	})
	if err == nil {
		t.Fatal("adding a second attribute named Alpha was accepted")
	}
	if got := attrNames(t, b, entID); len(got) != 1 {
		t.Errorf("a refused add still changed the entity: %v", got)
	}
}

// THE identity property. An attribute's $ID is what the runtime keys on; minting
// a fresh one on edit makes it a different attribute and churns every reference.
func TestUpdateAttribute_KeepsStoredID(t *testing.T) {
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	dmID, entID := entityWithAttributes(t, b, "Alpha", "Beta")
	mod, _ := b.GetModuleByName("MyFirstModule")
	dm, _ := b.GetDomainModel(mod.ID)
	var before *domainmodel.Attribute
	for _, e := range dm.Entities {
		if string(e.ID) == entID {
			before = e.Attributes[0]
		}
	}
	if before == nil {
		t.Fatal("fixture entity has no attributes")
	}

	edited := *before
	edited.Type = &domainmodel.IntegerAttributeType{}
	if err := b.UpdateAttribute(model.ID(dmID), model.ID(entID), &edited); err != nil {
		t.Fatalf("UpdateAttribute: %v", err)
	}

	dm2, _ := b.GetDomainModel(mod.ID)
	for _, e := range dm2.Entities {
		if string(e.ID) != entID {
			continue
		}
		if string(e.Attributes[0].ID) != string(before.ID) {
			t.Errorf("attribute $ID changed on update: %s -> %s — the runtime keys on this",
				before.ID, e.Attributes[0].ID)
		}
		if got := e.Attributes[0].Type.GetTypeName(); got != (&domainmodel.IntegerAttributeType{}).GetTypeName() {
			t.Errorf("the edit did not land: type = %s", got)
		}
	}
}

// The generated list has only Append and Remove, so the implementation rebuilds
// it. Without that, an edited attribute jumps to the bottom of the entity — a
// diff on every edit and a reordered entity in Studio Pro.
func TestUpdateAttribute_PreservesOrder(t *testing.T) {
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	dmID, entID := entityWithAttributes(t, b, "Alpha", "Beta", "Gamma")
	mod, _ := b.GetModuleByName("MyFirstModule")
	dm, _ := b.GetDomainModel(mod.ID)
	var middle *domainmodel.Attribute
	for _, e := range dm.Entities {
		if string(e.ID) == entID {
			middle = e.Attributes[1] // Beta
		}
	}
	edited := *middle
	edited.Type = &domainmodel.BooleanAttributeType{}
	if err := b.UpdateAttribute(model.ID(dmID), model.ID(entID), &edited); err != nil {
		t.Fatalf("UpdateAttribute: %v", err)
	}

	if got, want := attrNames(t, b, entID), []string{"Alpha", "Beta", "Gamma"}; !sameStrings(got, want) {
		t.Errorf("attribute order = %v, want %v — the edited one moved", got, want)
	}
}

func TestUpdateAttribute_UnknownAttributeIsAnError(t *testing.T) {
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	dmID, entID := entityWithAttributes(t, b, "Alpha")
	err := b.UpdateAttribute(model.ID(dmID), model.ID(entID), &domainmodel.Attribute{
		Name: "Nope", Type: &domainmodel.StringAttributeType{Length: 200},
	})
	if err == nil {
		t.Fatal("updating an attribute that does not exist was accepted")
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
