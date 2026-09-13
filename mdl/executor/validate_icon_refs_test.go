// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
)

func testIconIndex() *iconIndex {
	return &iconIndex{
		collections: map[string]map[string]bool{
			"Atlas_Core.Atlas":        {"home": true, "pencil": true, "pencil-write-paper": true},
			"Atlas_Core.Atlas_Filled": {"home": true, "pencil": true},
		},
		images: map[string]map[string]bool{
			"MyModule.Images":        {"logo": true},
			"DesignSystem.Icons_SVG": {"edit": true},
		},
		order:      []string{"Atlas_Core.Atlas", "Atlas_Core.Atlas_Filled"},
		imageOrder: []string{"DesignSystem.Icons_SVG", "MyModule.Images"},
	}
}

// TestIconIndexCheck covers the resolution rules. An icon reference was written
// straight through to BSON with nothing resolving it, so a typo first surfaced
// as CE1613 from MxBuild — long after `mxcli check` had passed.
func TestIconIndexCheck(t *testing.T) {
	idx := testIconIndex()
	tests := []struct {
		name      string
		value     string
		wantErr   bool
		wantParts []string
	}{
		{
			name:  "valid reference",
			value: "Atlas_Core.Atlas_Filled.pencil",
		},
		{
			name:  "valid reference in the other collection",
			value: "Atlas_Core.Atlas.home",
		},
		{
			name:      "unknown icon in a known collection",
			value:     "Atlas_Core.Atlas_Filled.no-such-icon",
			wantErr:   true,
			wantParts: []string{"no-such-icon", "Atlas_Core.Atlas_Filled", "CE1613", "describe icon collection"},
		},
		{
			name:    "unknown collection names the ones that exist",
			value:   "Atlas_Core.Nope.pencil",
			wantErr: true,
			// Listing the real collections is the actionable half: the typo is
			// usually in the collection, not the icon.
			wantParts: []string{"unknown icon collection", "Atlas_Core.Nope", "Atlas_Core.Atlas_Filled"},
		},
		{
			name:      "not a qualified reference",
			value:     "pencil",
			wantErr:   true,
			wantParts: []string{"not a qualified reference", "Module.Collection.Name"},
		},
		{
			name:      "trailing dot",
			value:     "Atlas_Core.Atlas.",
			wantErr:   true,
			wantParts: []string{"not a qualified reference"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := idx.check(iconRef{value: tt.value, kind: types.MenuIconCollection, where: "actionbutton 'btn'"})
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error for %q: %v", tt.value, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error for %q", tt.value)
			}
			for _, want := range tt.wantParts {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err.Error(), want)
				}
			}
		})
	}
}

// TestNearestIcons pins the suggestion behaviour: a near miss gets a hint, a
// name with nothing in common does not get a misleading one.
func TestNearestIcons(t *testing.T) {
	icons := map[string]bool{"pencil": true, "pencil-write-paper": true, "home": true}

	got := nearestIcons(icons, "penci")
	if len(got) != 2 || got[0] != "pencil" || got[1] != "pencil-write-paper" {
		t.Errorf("nearestIcons(penci) = %v, want [pencil pencil-write-paper]", got)
	}
	if got := nearestIcons(icons, "zzzzz"); len(got) != 0 {
		t.Errorf("nearestIcons(zzzzz) = %v, want none — a wrong hint is worse than no hint", got)
	}
	// An exact match is not a suggestion for itself.
	if got := nearestIcons(icons, "home"); len(got) != 0 {
		t.Errorf("nearestIcons(home) = %v, want none", got)
	}
}

// TestIconRefsInStatement covers collection from every statement shape that can
// carry an icon — a reference the walker misses is a reference nothing checks.
func TestIconRefsInStatement(t *testing.T) {
	btn := func(name, icon string) *ast.WidgetV3 {
		return &ast.WidgetV3{Type: "ACTIONBUTTON", Name: name, Properties: map[string]any{"Icon": icon}}
	}

	tests := []struct {
		name string
		stmt ast.Statement
		want []string
	}{
		{
			name: "create page, nested widgets",
			stmt: &ast.CreatePageStmtV3{Widgets: []*ast.WidgetV3{
				{Type: "CONTAINER", Name: "c1", Children: []*ast.WidgetV3{
					btn("btnA", "Atlas_Core.Atlas.home"),
					btn("btnB", "Atlas_Core.Atlas.pencil"),
				}},
			}},
			want: []string{"Atlas_Core.Atlas.home", "Atlas_Core.Atlas.pencil"},
		},
		{
			name: "alter page set",
			stmt: &ast.AlterPageStmt{Operations: []ast.AlterPageOperation{
				&ast.SetPropertyOp{
					Target:     ast.WidgetRef{Widget: "btnSave"},
					Properties: map[string]any{"Icon": "Atlas_Core.Atlas.home"},
				},
			}},
			want: []string{"Atlas_Core.Atlas.home"},
		},
		{
			name: "alter page insert and replace",
			stmt: &ast.AlterPageStmt{Operations: []ast.AlterPageOperation{
				&ast.InsertWidgetOp{Widgets: []*ast.WidgetV3{btn("btnI", "Atlas_Core.Atlas.a")}},
				&ast.ReplaceWidgetOp{NewWidgets: []*ast.WidgetV3{btn("btnR", "Atlas_Core.Atlas.b")}},
			}},
			want: []string{"Atlas_Core.Atlas.a", "Atlas_Core.Atlas.b"},
		},
		{
			name: "navigation menu, including sub-items",
			stmt: &ast.AlterNavigationStmt{MenuItems: []ast.NavMenuItemDef{
				{Caption: "Home", Icon: "Atlas_Core.Atlas.home", Items: []ast.NavMenuItemDef{
					{Caption: "Nested", Icon: "Atlas_Core.Atlas.pencil"},
				}},
			}},
			want: []string{"Atlas_Core.Atlas.home", "Atlas_Core.Atlas.pencil"},
		},
		// The four shapes below all reached MxBuild as CE1613 with `mxcli check
		// --references` reporting nothing (mendixlabs/mxcli#1008). Each holds its
		// widgets in a field the walk did not visit, so the icon was never seen.
		{
			// A page's Widgets field is only the bare body. Content bound to a
			// named layout placeholder lives in Placeholders.
			name: "create page, placeholder block",
			stmt: &ast.CreatePageStmtV3{Placeholders: []*ast.PagePlaceholderV3{
				{Name: "Main", Widgets: []*ast.WidgetV3{btn("btnP", "Atlas_Core.Atlas.home")}},
			}},
			want: []string{"Atlas_Core.Atlas.home"},
		},
		{
			// Both halves of a page, so fixing one does not silently drop the other.
			name: "create page, body and placeholder together",
			stmt: &ast.CreatePageStmtV3{
				Widgets: []*ast.WidgetV3{btn("btnBody", "Atlas_Core.Atlas.home")},
				Placeholders: []*ast.PagePlaceholderV3{
					{Name: "Topbar", Widgets: []*ast.WidgetV3{btn("btnPh", "Atlas_Core.Atlas.pencil")}},
				},
			},
			want: []string{"Atlas_Core.Atlas.home", "Atlas_Core.Atlas.pencil"},
		},
		{
			name: "create snippet",
			stmt: &ast.CreateSnippetStmtV3{Widgets: []*ast.WidgetV3{
				{Type: "CONTAINER", Name: "c1", Children: []*ast.WidgetV3{
					btn("btnS", "Atlas_Core.Atlas.home"),
				}},
			}},
			want: []string{"Atlas_Core.Atlas.home"},
		},
		{
			// The costliest of the four: a layout's topbar is shared, so one bad
			// icon there is an error on every page that uses the layout.
			name: "create layout",
			stmt: &ast.CreateLayoutStmt{Widgets: []*ast.WidgetV3{
				{Type: "SCROLLCONTAINER", Name: "layoutContainer", Children: []*ast.WidgetV3{
					btn("btnL", "Atlas_Core.Atlas.home"),
				}},
			}},
			want: []string{"Atlas_Core.Atlas.home"},
		},
		{
			// A menu document carries the same NavMenuItemDef as a profile menu,
			// so it needs the same recursion into sub-items.
			name: "menu document, including sub-items",
			stmt: &ast.CreateMenuStmt{Items: []ast.NavMenuItemDef{
				{Caption: "Home", Icon: "Atlas_Core.Atlas.home", Items: []ast.NavMenuItemDef{
					{Caption: "Nested", Icon: "Atlas_Core.Atlas.pencil"},
				}},
			}},
			want: []string{"Atlas_Core.Atlas.home", "Atlas_Core.Atlas.pencil"},
		},
		{
			name: "no icons at all",
			stmt: &ast.CreatePageStmtV3{Widgets: []*ast.WidgetV3{
				{Type: "CONTAINER", Name: "c1"},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := iconRefsInStatement(tt.stmt)
			if len(refs) != len(tt.want) {
				t.Fatalf("got %d refs %v, want %d %v", len(refs), refs, len(tt.want), tt.want)
			}
			for i, w := range tt.want {
				if refs[i].value != w {
					t.Errorf("ref[%d] = %q, want %q", i, refs[i].value, w)
				}
				if refs[i].where == "" {
					t.Errorf("ref[%d] has no location for the message", i)
				}
			}
		})
	}
}

// TestIconPropRef covers the quoting and casing MDL allows — `Icon:` and `icon:`
// are the same property, and the value may arrive quoted — and, since #1059, the
// two SHAPES the property can hold.
//
// The plain-string rows are not legacy trivia. `Icon:` carried a string before
// the kinds existed and several callers still build a WidgetV3 that way, so a
// reader handling only the typed shape stops resolving icons entirely — and says
// nothing about it, because "no reference found" and "no icon" look identical
// from here.
func TestIconPropRef(t *testing.T) {
	tests := []struct {
		name     string
		props    map[string]any
		wantOK   bool
		wantName string
		wantKind types.MenuIconKind
	}{
		{"string is the collection kind", map[string]any{"Icon": "Atlas_Core.Atlas.home"}, true, "Atlas_Core.Atlas.home", types.MenuIconCollection},
		{"lowercase key", map[string]any{"icon": "Atlas_Core.Atlas.home"}, true, "Atlas_Core.Atlas.home", types.MenuIconCollection},
		{"quoted value", map[string]any{"ICON": "'Atlas_Core.Atlas.home'"}, true, "Atlas_Core.Atlas.home", types.MenuIconCollection},
		{"padded value", map[string]any{"Icon": "  Atlas_Core.Atlas.home  "}, true, "Atlas_Core.Atlas.home", types.MenuIconCollection},
		{"typed collection", map[string]any{"Icon": &ast.WidgetIcon{Kind: types.MenuIconCollection, Name: "Atlas_Core.Atlas.home"}}, true, "Atlas_Core.Atlas.home", types.MenuIconCollection},
		{"typed image keeps its kind", map[string]any{"Icon": &ast.WidgetIcon{Kind: types.MenuIconImage, Name: "MyModule.Images.logo"}}, true, "MyModule.Images.logo", types.MenuIconImage},
		// A glyph names no document, so there is no name to resolve — but it IS
		// returned, carrying its code, because MDL078 reads glyphs off this same
		// walk. A private second walk is how that rule came to know about menu
		// items and not about widgets.
		{"glyph is walked, with a code and no name", map[string]any{"Icon": &ast.WidgetIcon{Kind: types.MenuIconGlyph, Code: 57377}}, true, "", types.MenuIconGlyph},
		{"no icon property", map[string]any{"Caption": "not an icon"}, false, "", ""},
		{"non-string must not panic", map[string]any{"Icon": 42}, false, "", ""},
		{"nil properties", nil, false, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := iconPropRef(tt.props)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (ref %+v)", ok, tt.wantOK, got)
			}
			if !ok {
				return
			}
			if got.value != tt.wantName || got.kind != tt.wantKind {
				t.Errorf("got (%q, %q), want (%q, %q)", got.value, got.kind, tt.wantName, tt.wantKind)
			}
		})
	}
}

// TestIconIndexCheck_ResolvesEachKindAgainstItsOwnDocument is the rule the kinds
// exist for. The two named kinds are spelled identically and live in different
// documents, so resolving one against the other's listing reports a correct
// reference as a typo — the same conflation as the original bug, pointed the
// other way.
func TestIconIndexCheck_ResolvesEachKindAgainstItsOwnDocument(t *testing.T) {
	idx := testIconIndex()

	if err := idx.check(iconRef{value: "DesignSystem.Icons_SVG.edit", kind: types.MenuIconImage, where: "actionbutton 'btn'"}); err != nil {
		t.Errorf("a valid image reference was rejected: %v", err)
	}
	if err := idx.check(iconRef{value: "Atlas_Core.Atlas.home", kind: types.MenuIconCollection, where: "actionbutton 'btn'"}); err != nil {
		t.Errorf("a valid collection reference was rejected: %v", err)
	}

	// Written without `image`, an image collection is not an icon collection —
	// and this is exactly the mistake #1059 reported, so the message has to name
	// the remedy rather than leave it looking like a typo.
	err := idx.check(iconRef{value: "DesignSystem.Icons_SVG.edit", kind: types.MenuIconCollection, where: "actionbutton 'btn'"})
	if err == nil {
		t.Fatal("an image reference written as a collection reference was accepted — that is CE1613")
	}
	for _, want := range []string{"IS an image collection", "Icon: image", "CE1613"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}

	// And the reverse.
	err = idx.check(iconRef{value: "Atlas_Core.Atlas.home", kind: types.MenuIconImage, where: "actionbutton 'btn'"})
	if err == nil {
		t.Fatal("a collection reference written as an image reference was accepted")
	}
	if !strings.Contains(err.Error(), "IS an icon collection") {
		t.Errorf("error %q does not name the remedy", err)
	}
}

// CONTROL: an empty listing means the backend could not answer, not that the
// project has none. Reporting every reference as unknown on that basis is a
// false error blocking a script that builds cleanly — the third state
// ("could not establish") that the check-vs-mxbuild class turns on.
func TestIconIndexCheck_EmptyListingResolvesNothing(t *testing.T) {
	idx := &iconIndex{collections: map[string]map[string]bool{}, images: map[string]map[string]bool{}}
	if err := idx.check(iconRef{value: "Whatever.Coll.name", kind: types.MenuIconCollection, where: "actionbutton 'btn'"}); err != nil {
		t.Errorf("an unresolvable listing reported an error: %v", err)
	}
	if err := idx.check(iconRef{value: "Whatever.Coll.name", kind: types.MenuIconImage, where: "actionbutton 'btn'"}); err != nil {
		t.Errorf("an unresolvable image listing reported an error: %v", err)
	}
}
