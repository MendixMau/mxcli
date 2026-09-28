// SPDX-License-Identifier: Apache-2.0

package widgetobj

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A widget property of widget.xml `type="file"` (the Document Viewer's `file`)
// stores an entity path from the enclosing data container in the WidgetValue's
// EntityRef — established from Mendix.Modeler (WidgetValueDocumentEntityPathConfig,
// mxbuild 11.14.0) and confirmed by `mx check` at 0 errors:
//
//	EntityRef: {$Type: DomainModels$DirectEntityRef, Entity: "Mod.File"}
//	EntityRef: {$Type: DomainModels$IndirectEntityRef, Steps: [2,
//	    {$Type: DomainModels$EntityRefStep, Association, DestinationEntity}]}
//
// SourceVariable stays null, which is "the enclosing container". Before SetFile
// the value was never written and mxbuild refused the widget with CE0642.

const fileID = "33333333-3333-3333-3333-333333333333"

func fileBuilder() *Builder {
	return &Builder{
		widgetID: "com.mendix.shared.widgets.documentviewer.DocumentViewer",
		object: bson.D{{Key: "Properties", Value: bson.A{
			int32(2),
			bson.D{
				{Key: "$Type", Value: "CustomWidgets$WidgetProperty"},
				{Key: "TypePointer", Value: types.UUIDToBlob(fileID)},
				{Key: "Value", Value: bson.D{
					{Key: "$Type", Value: "CustomWidgets$WidgetValue"},
					{Key: "EntityRef", Value: nil},
					{Key: "PrimitiveValue", Value: ""},
					{Key: "SourceVariable", Value: nil},
				}},
			},
		}}},
		propertyTypeIDs: map[string]pages.PropertyTypeIDEntry{
			"file": {PropertyTypeID: fileID, ValueType: "File"},
		},
	}
}

func fileValue(t *testing.T, ob *Builder) bson.D {
	t.Helper()
	props := findField(t, ob.object, "Properties").(bson.A)
	return findField(t, props[1].(bson.D), "Value").(bson.D)
}

func TestSetFile_CurrentObjectIsADirectEntityRef(t *testing.T) {
	ob := fileBuilder()
	ob.SetFile("file", "Mod.InvoiceDocument", nil)

	v := fileValue(t, ob)
	ref, ok := findField(t, v, "EntityRef").(bson.D)
	if !ok {
		t.Fatal("EntityRef is still unset — mxbuild reports CE0642 'Property ... is required'")
	}
	if typ := findField(t, ref, "$Type"); typ != "DomainModels$DirectEntityRef" {
		t.Errorf("$Type = %v, want DomainModels$DirectEntityRef", typ)
	}
	if ent := findField(t, ref, "Entity"); ent != "Mod.InvoiceDocument" {
		t.Errorf("Entity = %v, want Mod.InvoiceDocument", ent)
	}
	if findField(t, ref, "$ID") == nil {
		t.Error("EntityRef has no $ID")
	}
	if sv := findField(t, v, "SourceVariable"); sv != nil {
		t.Errorf("SourceVariable = %v, want nil (the enclosing data container)", sv)
	}
	if pv := findField(t, v, "PrimitiveValue"); pv != "" {
		t.Errorf("PrimitiveValue = %v, want untouched", pv)
	}
}

func TestSetFile_AssociationPathIsAnIndirectEntityRef(t *testing.T) {
	ob := fileBuilder()
	ob.SetFile("file", "Mod.InvoiceDocument", []pages.AttributeRefStep{
		{Association: "Mod.InvoiceDocument_Invoice", DestinationEntity: "Mod.InvoiceDocument"},
	})

	ref, ok := findField(t, fileValue(t, ob), "EntityRef").(bson.D)
	if !ok {
		t.Fatal("EntityRef is still unset")
	}
	if typ := findField(t, ref, "$Type"); typ != "DomainModels$IndirectEntityRef" {
		t.Fatalf("$Type = %v, want DomainModels$IndirectEntityRef", typ)
	}
	steps := findField(t, ref, "Steps").(bson.A)
	if len(steps) != 2 || steps[0] != int32(2) {
		t.Fatalf("Steps = %v, want [2, one step]", steps)
	}
	step := steps[1].(bson.D)
	if typ := findField(t, step, "$Type"); typ != "DomainModels$EntityRefStep" {
		t.Errorf("step $Type = %v, want DomainModels$EntityRefStep", typ)
	}
	if a, d := findField(t, step, "Association"), findField(t, step, "DestinationEntity"); a != "Mod.InvoiceDocument_Invoice" || d != "Mod.InvoiceDocument" {
		t.Errorf("step = {%v → %v}, want {Mod.InvoiceDocument_Invoice → Mod.InvoiceDocument}", a, d)
	}
}

// "The script did not say" leaves the template's value alone, like every other
// Set* on this builder.
func TestSetFile_EmptyIsANoOp(t *testing.T) {
	ob := fileBuilder()
	ob.SetFile("file", "", nil)
	if ref := findField(t, fileValue(t, ob), "EntityRef"); ref != nil {
		t.Errorf("EntityRef = %v, want nil", ref)
	}
}
