// SPDX-License-Identifier: Apache-2.0

// A pluggable widget property of widget.xml `type="file"` — the Document
// Viewer's `file` is the one that surfaced it — could not be written from MDL.
// GenerateDefJSON emitted no mapping for the type, so `file:` was reported
// MDL-WIDGET06 ("recognized but not persisted") and dropped, and a required one
// failed mxbuild with:
//
//	CE0642 Property 'Document' is required.
//
// The stored shape, from Mendix.Modeler (WidgetValueDocumentEntityPathConfig)
// and confirmed by `mx check` at 0 errors on a Mendix 11.14 model, is an entity
// path from the enclosing data container, in the WidgetValue's EntityRef:
//
//	EntityRef: DirectEntityRef{ Entity: "Mod.File" }             // $currentObject
//	EntityRef: IndirectEntityRef{ Steps: [ EntityRefStep{
//	    Association: "Mod.File_Owner", DestinationEntity: "Mod.File" } ] }
//
// These tests cover the executor half: the def mapping, the registry, the MDL
// value forms, the path resolution and the built widget.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
	"go.mongodb.org/mongo-driver/bson"
)

// A `type="file"` property gets a `file` mapping of its own, and so is no
// longer listed as known-but-unmapped (which is what made the checker warn
// MDL-WIDGET06 and the writer drop it).
func TestGenerateDefJSON_FilePropertyMapped(t *testing.T) {
	mpkDef := &mpk.WidgetDefinition{
		ID:   "com.mendix.shared.widgets.documentviewer.DocumentViewer",
		Name: "Document Viewer",
		Properties: []mpk.PropertyDef{
			{Key: "file", Type: "file"},
			{Key: "width", Type: "integer"},
		},
	}
	def := GenerateDefJSON(mpkDef, "DOCUMENTVIEWER")
	m := findMapping(def.PropertyMappings, "file")
	if m == nil {
		t.Fatal("file (type=file) has no mapping — `file:` would be dropped and a required one fails CE0642")
	}
	if m.Operation != "file" || m.Source != "File" {
		t.Errorf("file mapping = {op=%s src=%s}, want {file File}", m.Operation, m.Source)
	}
	for _, k := range def.KnownProperties {
		if k == "file" {
			t.Error("KnownProperties lists the mapped `file` property — the checker would still warn it is dropped")
		}
	}
}

// The registry validates every project def's operations. Without "file" in the
// known set, the first def carrying one stops the load and every project widget
// is reported MDL-WIDGET25 "not a widget in this project".
func TestKnownOperationsSet_IncludesFile(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("NewWidgetRegistry() error: %v", err)
	}
	if !reg.knownOperations["file"] {
		t.Error(`knownOperations["file"] = false — a regenerated def with a file mapping would be rejected`)
	}
}

// fileWidgetProps parses a real snippet and returns the widget's properties, so
// the value forms are the ones the parser actually produces, not imagined ones.
func fileWidgetProps(t *testing.T, value string) map[string]any {
	t.Helper()
	src := `create or modify snippet Rules.S (Params: { $A: Rules.RuleAction }) {
  dataview dv (DataSource: $A) {
    documentviewer viewer (file: ` + value + `)
  }
}`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("source does not parse: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateSnippetStmtV3)
	if !ok {
		t.Fatalf("parsed %T, want *ast.CreateSnippetStmtV3", prog.Statements[0])
	}
	dv := stmt.Widgets[0]
	if len(dv.Children) != 1 {
		t.Fatalf("dataview has %d children, want 1", len(dv.Children))
	}
	return dv.Children[0].Properties
}

// `$currentObject` and `$currentObject/Mod.Assoc` come out of the parser as
// data-source expressions, not strings. Read as a string they are "", which is
// how the value was dropped silently even once a mapping existed.
func TestFilePropertyPath_ParsedForms(t *testing.T) {
	mapping := PropertyMapping{PropertyKey: "file", Source: "File", Operation: "file"}
	for _, tc := range []struct{ mdl, want string }{
		{"$currentObject", "$currentObject"},
		{"$currentObject/Rules.RuleAction_BusinessRule", "$currentObject/Rules.RuleAction_BusinessRule"},
		{"Rules.RuleAction_BusinessRule", "Rules.RuleAction_BusinessRule"},
	} {
		t.Run(tc.mdl, func(t *testing.T) {
			w := &ast.WidgetV3{Name: "viewer", Properties: fileWidgetProps(t, tc.mdl)}
			got, err := filePropertyPath(mapping, w)
			if err != nil {
				t.Fatalf("filePropertyPath: %v", err)
			}
			if got != tc.want {
				t.Errorf("path = %q, want %q", got, tc.want)
			}
		})
	}
}

// A source that is not a path from the container (a database retrieve here) is
// refused with a message naming the property, not written as nothing.
func TestFilePropertyPath_RefusesOtherSources(t *testing.T) {
	mapping := PropertyMapping{PropertyKey: "file", Source: "File", Operation: "file"}
	w := &ast.WidgetV3{Name: "viewer", Properties: map[string]any{
		"file": &ast.DataSourceV3{Type: "database", Reference: "Rules.BusinessRule"},
	}}
	_, err := filePropertyPath(mapping, w)
	if err == nil || !strings.Contains(err.Error(), "file") {
		t.Fatalf("err = %v, want a refusal naming the file property", err)
	}
}

func TestResolveFileEntityPath(t *testing.T) {
	pb := ruleActionPB(t)

	t.Run("$currentObject is the container's own entity, no steps", func(t *testing.T) {
		entity, steps, err := pb.resolveFileEntityPath("$currentObject", "Rules.RuleAction")
		if err != nil || entity != "Rules.RuleAction" || len(steps) != 0 {
			t.Errorf("got (%q, %v, %v), want (Rules.RuleAction, no steps, nil)", entity, steps, err)
		}
	})

	t.Run("an association path is one step per hop", func(t *testing.T) {
		for _, in := range []string{
			"$currentObject/Rules.RuleAction_BusinessRule",
			"Rules.RuleAction_BusinessRule",
			"RuleAction_BusinessRule", // qualified with the context's module
		} {
			entity, steps, err := pb.resolveFileEntityPath(in, "Rules.RuleAction")
			if err != nil {
				t.Fatalf("%s: %v", in, err)
			}
			if entity != "Rules.BusinessRule" || len(steps) != 1 ||
				steps[0].Association != "Rules.RuleAction_BusinessRule" ||
				steps[0].DestinationEntity != "Rules.BusinessRule" {
				t.Errorf("%s: got (%q, %+v), want (Rules.BusinessRule, [RuleAction_BusinessRule → BusinessRule])", in, entity, steps)
			}
		}
	})

	t.Run("the association is followed in whichever direction the context is on", func(t *testing.T) {
		entity, steps, err := pb.resolveFileEntityPath("Rules.RuleAction_BusinessRule", "Rules.BusinessRule")
		if err != nil || entity != "Rules.RuleAction" || len(steps) != 1 || steps[0].DestinationEntity != "Rules.RuleAction" {
			t.Errorf("got (%q, %+v, %v), want Rules.RuleAction over one step", entity, steps, err)
		}
	})

	t.Run("a name that is not an association of the context is refused", func(t *testing.T) {
		if _, _, err := pb.resolveFileEntityPath("Rules.NoSuchAssoc", "Rules.RuleAction"); err == nil {
			t.Error("expected an error for an unknown association")
		}
	})

	t.Run("no enclosing data container is refused", func(t *testing.T) {
		_, _, err := pb.resolveFileEntityPath("$currentObject", "")
		if err == nil || !strings.Contains(err.Error(), "data container") {
			t.Errorf("err = %v, want a refusal naming the missing data container", err)
		}
	})
}

// fileBuildEngine is ruleActionPB behind a widget engine that loads real
// embedded templates. No embedded widget has a `type="file"` property, so the
// Image widget's `imageObject` value node stands in: SetFile writes the
// EntityRef key every WidgetValue carries, whatever the property's type.
func fileBuildEngine(t *testing.T) *PluggableWidgetEngine {
	t.Helper()
	real := &modelsdkbackend.Backend{}
	b := &mock.MockBackend{LoadWidgetTemplateFunc: real.LoadWidgetTemplate}
	pb := ruleActionPB(t)
	pb.backend = b
	pb.paramEntityNames = map[string]string{}
	e := NewPluggableWidgetEngine(b, pb)
	pb.pluggableEngine = e
	return e
}

func fileDef() *WidgetDefinition {
	return &WidgetDefinition{
		WidgetID:     "com.mendix.widget.web.image.Image",
		MDLName:      "IMAGE",
		TemplateFile: "image.json",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "imageObject", Source: "File", Operation: "file"},
		},
	}
}

// findEntityRefs collects every non-nil EntityRef document in the built widget.
func findEntityRefs(v any, out *[]bson.D) {
	switch x := v.(type) {
	case bson.D:
		for _, e := range x {
			if e.Key == "EntityRef" {
				if d, ok := e.Value.(bson.D); ok {
					*out = append(*out, d)
				}
			}
			findEntityRefs(e.Value, out)
		}
	case bson.A:
		for _, e := range x {
			findEntityRefs(e, out)
		}
	case []any:
		for _, e := range x {
			findEntityRefs(e, out)
		}
	}
}

func builtRawObject(t *testing.T, e *PluggableWidgetEngine, w *ast.WidgetV3) bson.D {
	t.Helper()
	widget, err := e.Build(fileDef(), w)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if widget == nil || widget.RawObject == nil {
		t.Fatal("Build returned no raw widget object")
	}
	return widget.RawObject
}

func bsonField(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

// primitiveValues collects every non-empty PrimitiveValue in the built widget.
func primitiveValues(v any, out *[]string) {
	switch x := v.(type) {
	case bson.D:
		for _, e := range x {
			if s, ok := e.Value.(string); ok && e.Key == "PrimitiveValue" && s != "" {
				*out = append(*out, s)
			}
			primitiveValues(e.Value, out)
		}
	case bson.A:
		for _, e := range x {
			primitiveValues(e, out)
		}
	case []any:
		for _, e := range x {
			primitiveValues(e, out)
		}
	}
}

// The built widget carries an IndirectEntityRef over the association, and the
// path's string form is NOT also written into a PrimitiveValue by the explicit
// property pass (which it was before `file:` counted as a mapped key).
func TestBuild_FilePropertyWritesEntityRef(t *testing.T) {
	e := fileBuildEngine(t)
	w := &ast.WidgetV3{
		Name: "viewer",
		Type: "pluggablewidget",
		Properties: map[string]any{
			"WidgetType":  "com.mendix.widget.web.image.Image",
			"imageObject": "Rules.RuleAction_BusinessRule",
		},
	}
	obj := builtRawObject(t, e, w)

	var refs []bson.D
	findEntityRefs(obj, &refs)
	if len(refs) != 1 {
		t.Fatalf("found %d EntityRefs, want exactly 1 (the file property's)", len(refs))
	}
	if typ := bsonField(refs[0], "$Type"); typ != "DomainModels$IndirectEntityRef" {
		t.Fatalf("EntityRef $Type = %v, want DomainModels$IndirectEntityRef", typ)
	}
	steps := getBsonArrayElements(bsonField(refs[0], "Steps"))
	if len(steps) != 1 {
		t.Fatalf("IndirectEntityRef has %d steps, want 1", len(steps))
	}
	step, _ := steps[0].(bson.D)
	if a, d := bsonField(step, "Association"), bsonField(step, "DestinationEntity"); a != "Rules.RuleAction_BusinessRule" || d != "Rules.BusinessRule" {
		t.Errorf("step = {%v → %v}, want {Rules.RuleAction_BusinessRule → Rules.BusinessRule}", a, d)
	}
	var pvs []string
	primitiveValues(obj, &pvs)
	for _, pv := range pvs {
		if strings.Contains(pv, "RuleAction_BusinessRule") {
			t.Errorf("the path was also written as PrimitiveValue %q — a stray value beside the EntityRef", pv)
		}
	}
}

func TestBuild_FilePropertyCurrentObjectIsDirect(t *testing.T) {
	e := fileBuildEngine(t)
	w := &ast.WidgetV3{
		Name: "viewer",
		Type: "pluggablewidget",
		Properties: map[string]any{
			"WidgetType":  "com.mendix.widget.web.image.Image",
			"imageObject": &ast.DataSourceV3{Type: "parameter", Reference: "$currentObject"},
		},
	}
	obj := builtRawObject(t, e, w)

	var refs []bson.D
	findEntityRefs(obj, &refs)
	if len(refs) != 1 {
		t.Fatalf("found %d EntityRefs, want 1 — `file: $currentObject` was dropped", len(refs))
	}
	if typ, ent := bsonField(refs[0], "$Type"), bsonField(refs[0], "Entity"); typ != "DomainModels$DirectEntityRef" || ent != "Rules.RuleAction" {
		t.Errorf("EntityRef = {%v %v}, want {DomainModels$DirectEntityRef Rules.RuleAction}", typ, ent)
	}
}

// DESCRIBE reads the EntityRef back into the form the writer takes, so a
// describe → exec round-trip keeps the binding.
func TestFileEntityRefPath(t *testing.T) {
	direct := map[string]any{"$Type": "DomainModels$DirectEntityRef", "Entity": "Rules.RuleAction"}
	if got := fileEntityRefPath(direct); got != "$currentObject" {
		t.Errorf("direct = %q, want $currentObject", got)
	}
	indirect := map[string]any{
		"$Type": "DomainModels$IndirectEntityRef",
		"Steps": []any{int32(2),
			map[string]any{"$Type": "DomainModels$EntityRefStep", "Association": "M.A_B", "DestinationEntity": "M.B"},
			map[string]any{"$Type": "DomainModels$EntityRefStep", "Association": "M.B_C", "DestinationEntity": "M.C"},
		},
	}
	if got := fileEntityRefPath(indirect); got != "$currentObject/M.A_B/M.B_C" {
		t.Errorf("indirect = %q, want $currentObject/M.A_B/M.B_C", got)
	}
	if got := fileEntityRefPath(nil); got != "" {
		t.Errorf("unset = %q, want empty", got)
	}
}
