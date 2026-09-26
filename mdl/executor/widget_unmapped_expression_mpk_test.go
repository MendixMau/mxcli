// SPDX-License-Identifier: Apache-2.0

// An expression-typed property the widget's INSTALLED .mpk declares but mxcli's
// embedded template does not — File Uploader 2.5.0's `maxFilesPerUpload`
// ("Maximum number of files"). The explicit-property pass writes it (measured:
// `maxFilesPerUpload: '1'` stores Expression "1"), yet MDL-WIDGET06 said it
// "will be dropped" because persistedByExplicitPass looked the type up in the
// embedded template only, found nothing, and kept the warning. A project took
// the warning at its word and left the setting for Studio Pro.
package executor

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeProjectMPK(t *testing.T, projectDir, widgetXML string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(projectDir, "widgets"), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(projectDir, "widgets", "Probe.mpk"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	add := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	add("package.xml", `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.mendix.com/package/1.0/">
  <clientModule name="Probe" version="1.0.0" xmlns="http://www.mendix.com/clientModule/1.0/">
    <widgetFiles><widgetFile path="Probe.xml"/></widgetFiles>
  </clientModule>
</package>`)
	add("Probe.xml", widgetXML)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPersistedByExplicitPass_ReadsTheInstalledPackage(t *testing.T) {
	dir := t.TempDir()
	writeProjectMPK(t, dir, `<?xml version="1.0" encoding="utf-8"?>
<widget id="com.example.probe.Probe" pluginWidget="true" xmlns="http://www.mendix.com/widget/1.0/">
  <name>Probe</name>
  <properties>
    <propertyGroup caption="General">
      <property key="maxFilesPerUpload" type="expression" defaultValue="10" required="false">
        <caption>Maximum number of files</caption>
        <description />
        <returnType type="Integer" />
      </property>
      <property key="sources" type="datasource" isList="true" required="false">
        <caption>Sources</caption>
        <description />
      </property>
    </propertyGroup>
  </properties>
</widget>`)
	mprPath := filepath.Join(dir, "App.mpr")

	if !persistedByExplicitPass("com.example.probe.Probe", "maxfilesperupload", mprPath) {
		t.Error("an expression property declared by the installed .mpk is written by the explicit pass; MDL-WIDGET06 must not call it dropped")
	}
	// Control: a type the explicit pass has no route for keeps the warning.
	if persistedByExplicitPass("com.example.probe.Probe", "sources", mprPath) {
		t.Error("a datasource property is not written by the explicit pass; the warning must stay")
	}
	// Control: no project, no package — unknown type keeps the warning.
	if persistedByExplicitPass("com.example.probe.Probe", "maxfilesperupload", "") {
		t.Error("without a project the type is unknown; the warning must stay")
	}
}

// The read half: describe re-emits a stored expression property, so replaying
// its output does not reset it to the default. Without a project the default is
// unknown and the stored value is emitted; with the package's default equal to
// the stored value it is left out.
func TestExplicitPropertiesReadExpressionValues(t *testing.T) {
	const id = "type-id-maxfiles"
	w := map[string]any{
		"Type": map[string]any{
			"WidgetId": "com.example.probe.Probe",
			"ObjectType": map[string]any{"PropertyTypes": []any{
				map[string]any{"$ID": id, "PropertyKey": "maxFilesPerUpload",
					"ValueType": map[string]any{"Type": "Expression"}},
			}},
		},
		"Object": map[string]any{"Properties": []any{
			map[string]any{"TypePointer": id, "Value": map[string]any{"Expression": "1", "PrimitiveValue": ""}},
		}},
	}
	props := extractExplicitProperties(nil, w)
	if len(props) != 1 || props[0].Key != "maxFilesPerUpload" || props[0].Value != "1" {
		t.Fatalf("got %+v, want maxFilesPerUpload = 1", props)
	}
	if got := explicitPropValue(props[0]); got != "'1'" {
		t.Errorf("rendered %s, want '1' — the builder reads an expression property from a string", got)
	}

	// With the installed package declaring default "10", a stored "10" is noise.
	dir := t.TempDir()
	writeProjectMPK(t, dir, `<?xml version="1.0" encoding="utf-8"?>
<widget id="com.example.probe.Probe" pluginWidget="true" xmlns="http://www.mendix.com/widget/1.0/">
  <name>Probe</name>
  <properties><propertyGroup caption="General">
    <property key="maxFilesPerUpload" type="expression" defaultValue="10" required="false">
      <caption>Max</caption><description /><returnType type="Integer" />
    </property>
  </propertyGroup></properties>
</widget>`)
	ctx := &ExecContext{MprPath: filepath.Join(dir, "App.mpr")}
	w["Object"] = map[string]any{"Properties": []any{
		map[string]any{"TypePointer": id, "Value": map[string]any{"Expression": "10"}},
	}}
	if props := extractExplicitProperties(ctx, w); len(props) != 0 {
		t.Errorf("a stored default was emitted: %+v", props)
	}
}
