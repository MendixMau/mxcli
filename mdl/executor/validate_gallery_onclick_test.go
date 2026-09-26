// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A gallery's OnClick fires on a single click, which is also what selects an
// item. Mendix rejects the pair (mx check: "The item click action is
// ambiguous"), and Selection defaults to Single — so an OnClick with no
// `selection: none` is refused at check time instead of failing the build.
func TestValidateGalleryOnClickSelection(t *testing.T) {
	act := &ast.ActionV3{Type: "microflow", Target: "M.Open"}
	cases := []struct {
		name  string
		props map[string]any
		want  int
	}{
		{"default selection", map[string]any{"Action": act}, 1},
		{"single", map[string]any{"Action": act, "Selection": "Single"}, 1},
		{"multiple", map[string]any{"Action": act, "Selection": "Multiple"}, 1},
		{"none", map[string]any{"Action": act, "Selection": "none"}, 0},
		{"no action", map[string]any{"Selection": "Single"}, 0},
	}
	for _, c := range cases {
		vs := validateGalleryOnClickSelection(&ast.WidgetV3{Type: "GALLERY", Name: "g", Properties: c.props}, "page P")
		if len(vs) != c.want {
			t.Errorf("%s: got %v, want %d violation(s)", c.name, vs, c.want)
			continue
		}
		if c.want == 1 && vs[0].RuleID != "MDL-WIDGET35" {
			t.Errorf("%s: rule %s, want MDL-WIDGET35", c.name, vs[0].RuleID)
		}
	}
	// A clickable container is not a gallery; its action has no selection to clash with.
	if vs := validateGalleryOnClickSelection(&ast.WidgetV3{Type: "CONTAINER", Name: "c",
		Properties: map[string]any{"Action": act}}, "page P"); len(vs) != 0 {
		t.Errorf("container: got %v, want none", vs)
	}
}

// DESCRIBE must print a gallery's stored `Selection: None`. The definition's
// default is Single, so an omitted None came back as Single on describe → exec,
// and a gallery with OnClick — which needs None — was then refused as
// MDL-WIDGET35 (measured on Mendix 11.14.0: describe, re-exec, check error).
// Every property value carries a Selection field (None on all but
// itemSelection), so the lookup must go by key.
func TestCustomWidgetPropertySelection_ByKey(t *testing.T) {
	gallery := func(sel string) map[string]any {
		return map[string]any{
			"Type": map[string]any{"ObjectType": map[string]any{"PropertyTypes": []any{
				map[string]any{"$ID": "pt-sel", "PropertyKey": "itemSelection"},
				map[string]any{"$ID": "pt-page", "PropertyKey": "pageSize"},
			}}},
			"Object": map[string]any{"Properties": []any{
				map[string]any{"TypePointer": "pt-page", "Value": map[string]any{"Selection": "None", "PrimitiveValue": "20"}},
				map[string]any{"TypePointer": "pt-sel", "Value": map[string]any{"Selection": sel}},
			}},
		}
	}
	for _, sel := range []string{"None", "Single", "Multiple"} {
		if got := customWidgetPropertySelection(gallery(sel), "itemSelection"); got != sel {
			t.Errorf("stored %s: got %q", sel, got)
		}
	}
	if got := customWidgetPropertySelection(gallery("Single"), "noSuchKey"); got != "" {
		t.Errorf("unknown key: got %q, want empty", got)
	}
}
