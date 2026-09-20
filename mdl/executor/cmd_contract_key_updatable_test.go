// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// A writable OData entity set: the container declares Insertable=true AND
// Updatable=true, and the entity has one key property plus one ordinary one.
// Deliberately free of complex types — the key-updatability defect is
// independent of the ComplexType flattening fixed in mendixlabs/mxcli#1118 and
// reproduces on a contract with no complex type at all.
const writableSetMetadata = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="App.Model" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EntityType Name="Definition">
        <Key><PropertyRef Name="Id"/></Key>
        <Property Name="Id" Type="Edm.String" Nullable="false" MaxLength="36"/>
        <Property Name="Label" Type="Edm.String" MaxLength="100"/>
      </EntityType>
      <EntityContainer Name="Container">
        <EntitySet Name="Definition" EntityType="App.Model.Definition">
          <Annotation Term="Org.OData.Capabilities.V1.InsertRestrictions">
            <Record><PropertyValue Property="Insertable" Bool="true"/></Record>
          </Annotation>
          <Annotation Term="Org.OData.Capabilities.V1.UpdateRestrictions">
            <Record><PropertyValue Property="Updatable" Bool="true"/></Record>
          </Annotation>
        </EntitySet>
      </EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`

// The same contract with the set insertable but NOT updatable — the second
// direction the report asks about.
const insertOnlySetMetadata = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="App.Model" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EntityType Name="Definition">
        <Key><PropertyRef Name="Id"/></Key>
        <Property Name="Id" Type="Edm.String" Nullable="false" MaxLength="36"/>
        <Property Name="Label" Type="Edm.String" MaxLength="100"/>
      </EntityType>
      <EntityContainer Name="Container">
        <EntitySet Name="Definition" EntityType="App.Model.Definition">
          <Annotation Term="Org.OData.Capabilities.V1.InsertRestrictions">
            <Record><PropertyValue Property="Insertable" Bool="true"/></Record>
          </Annotation>
          <Annotation Term="Org.OData.Capabilities.V1.UpdateRestrictions">
            <Record><PropertyValue Property="Updatable" Bool="false"/></Record>
          </Annotation>
        </EntitySet>
      </EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`

// The reported symptom, on mxbuild 11.12.1:
//
//	[error] [CE6630] "'DefinitionId' is marked Updatable=False in the OData
//	                  service, but True in the app."
//	        at Attribute 'MyFirstModule.Definition.DefinitionId'
//
// Mendix computes a key property as non-updatable whatever the entity set's
// UpdateRestrictions say — a key cannot be changed after creation — so an
// import that lets the key follow the set is exactly one CE6630 per key part.
//
// The control is `Label`: an ordinary attribute of the SAME writable set keeps
// the import's current answer, so the test cannot pass against a fix that
// simply stamped every attribute read-only. It pins behaviour, it is NOT a
// claim that mxbuild wants Updatable=true there — measured on 11.12.1, mxbuild
// says Updatable=False for `Label` too, and for the non-key attribute of all
// SEVEN annotation shapes probed (inline record, typed record, UpdateMethod,
// +NonUpdatableProperties/+DeleteRestrictions, unannotated, external
// <Annotations Target=…>, Core.Permissions/ReadWrite). That is a SECOND,
// separate defect in how this import reads UpdateRestrictions, and it is
// deliberately left alone here: no probe produced a contract mxbuild treats as
// updatable, so there is no positive control for what the right rule is, and
// mxcli-formula1 §48 measured CE6630 firing in BOTH directions — a blanket
// "never updatable" is a guess that can be wrong the other way.
//
// The key rule, by contrast, has a clean two-sided measurement: with the key
// at Updatable=false mxbuild is silent on the key of all seven shapes, and
// with it following the entity set it is exactly one CE6630 per key.
func TestCreateExternalEntities_KeyAttributeIsNeverUpdatable(t *testing.T) {
	ent, _ := importOne(t, writableSetMetadata, "Definition")
	byName := attrByName(ent)

	// `Id` is a Mendix reserved word, so the import renames it — the CE6630
	// in the report names `DefinitionId` for that reason.
	key := byName["DefinitionId"]
	if key == nil {
		t.Fatalf("key attribute DefinitionId missing; got %v", attrNames(ent))
	}
	if key.Updatable {
		t.Error("key attribute is Updatable=true against a service that computes it False — CE6630 " +
			`"'DefinitionId' is marked Updatable=False in the OData service, but True in the app."`)
	}

	label := byName["Label"]
	if label == nil {
		t.Fatalf("attribute Label missing; got %v", attrNames(ent))
	}
	if !label.Updatable {
		t.Error("control failed: the fix must be key-specific — a non-key attribute of the same " +
			"set still follows the entity set, so a blanket read-only stamp is caught here")
	}
}

// A key is NOT read-only: it is written once, at creation. The report's build
// flagged the key Updatable=False *only* — no Creatable error accompanied it —
// so the rule is "a key cannot be changed after creation", not "the entity is
// read-only". Clearing Creatable too would be the obvious over-correction and
// its own CE6630.
func TestCreateExternalEntities_KeyAttributeStaysCreatable(t *testing.T) {
	ent, _ := importOne(t, writableSetMetadata, "Definition")
	byName := attrByName(ent)

	key := byName["DefinitionId"]
	if key == nil {
		t.Fatalf("key attribute DefinitionId missing; got %v", attrNames(ent))
	}
	if !key.Creatable {
		t.Error("key attribute lost Creatable against an Insertable=true set — " +
			"the key is set at creation, so this is CE6630 the other way")
	}
}

// The second direction: a set that is insertable but not updatable. The key
// must be non-updatable here too (it already was, via the set), and Creatable
// must still follow the contract — so the key fix must not be written as
// "clear both capabilities on a key".
func TestCreateExternalEntities_InsertableButNotUpdatableSet(t *testing.T) {
	ent, _ := importOne(t, insertOnlySetMetadata, "Definition")
	byName := attrByName(ent)

	key := byName["DefinitionId"]
	if key == nil {
		t.Fatalf("key attribute DefinitionId missing; got %v", attrNames(ent))
	}
	if key.Updatable {
		t.Error("key attribute Updatable=true against a non-updatable set — CE6630")
	}
	if !key.Creatable {
		t.Error("key attribute Creatable=false against an Insertable=true set — CE6630")
	}

	label := byName["Label"]
	if label == nil {
		t.Fatalf("attribute Label missing; got %v", attrNames(ent))
	}
	if label.Updatable {
		t.Error("non-key attribute Updatable=true against a non-updatable set — CE6630")
	}
	if !label.Creatable {
		t.Error("control failed: a non-key attribute of an insertable set still follows the set's Insertable=true")
	}
}
