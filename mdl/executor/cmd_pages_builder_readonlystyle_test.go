// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#490 — the builder half: `ReadOnlyStyle:` on a checkbox has to reach
// the semantic model, or the codec has nothing to write. The property parsed,
// `mxcli check` accepted it (it is in the known-property allowlist as
// "vocabulary describe page emits") and every layer below dropped it.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

func TestBuildCheckBox_ReadOnlyStyle(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"unset stays unset (writer keeps the stored default)", nil, ""},
		{"control", "Control", "Control"},
		{"text", "Text", "Text"},
		{"inherit", "Inherit", "Inherit"},
		// MDL property values are matched case-insensitively everywhere else;
		// the enum value stored must still be Mendix's own casing, since an
		// unknown member is a document Studio Pro cannot load.
		{"lowercase is canonicalised", "control", "Control"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pb := &pageBuilder{widgetScope: map[string]model.ID{}}
			w := &ast.WidgetV3{Name: "cbActive", Type: "checkbox", Properties: map[string]any{}}
			if tc.value != nil {
				w.Properties["ReadOnlyStyle"] = tc.value
			}
			cb, err := pb.buildCheckBoxV3(w)
			if err != nil {
				t.Fatalf("buildCheckBoxV3: %v", err)
			}
			if cb.ReadOnlyStyle != tc.want {
				t.Errorf("ReadOnlyStyle = %q, want %q (ako/mxcli#490)", cb.ReadOnlyStyle, tc.want)
			}
		})
	}
}

// An unknown member must be refused rather than written: a value outside
// Inherit/Control/Text is a property Studio Pro cannot resolve, and mxbuild
// tolerates it — so the build stays green and the project will not open.
func TestBuildCheckBox_ReadOnlyStyleRejectsUnknownValue(t *testing.T) {
	pb := &pageBuilder{widgetScope: map[string]model.ID{}}
	w := &ast.WidgetV3{
		Name:       "cbActive",
		Type:       "checkbox",
		Properties: map[string]any{"ReadOnlyStyle": "ReadOnly"},
	}
	_, err := pb.buildCheckBoxV3(w)
	if err == nil {
		t.Fatal("an unknown ReadOnlyStyle was accepted — it would be written into the document")
	}
	if !strings.Contains(err.Error(), "Control") {
		t.Errorf("the error should name the accepted values, got: %v", err)
	}
}
