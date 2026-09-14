// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// A widget's `Icon:` has to say WHICH of Mendix's three icon elements it means
// (mendixlabs/mxcli#1059). The two named kinds are spelled identically — both
// are a Module.Collection.Name — so the keyword is the only thing that
// distinguishes an icon-collection reference from an image-collection one, and
// getting it wrong is a document that builds as CE1613.
//
// The invariant these assert is the parse, not the wording: `image` and `glyph`
// must reach the AST as KINDS, and the bare form must still mean what it has
// meant since #602.
func TestWidgetIconV3_ParsesEveryVariant(t *testing.T) {
	cases := []struct {
		name string
		mdl  string
		want ast.WidgetIcon
	}{{
		// The form every existing script and every DESCRIBE output uses.
		name: "quoted bare is the collection icon",
		mdl:  `Icon: 'Atlas_Core.Atlas_Filled.pencil'`,
		want: ast.WidgetIcon{Kind: types.MenuIconCollection, Name: "Atlas_Core.Atlas_Filled.pencil"},
	}, {
		// A reference into the model is spelled as a qualified name everywhere
		// else (ADR-0003), so the unquoted form is accepted too.
		name: "unquoted bare is the collection icon",
		mdl:  `Icon: Atlas_Core.Atlas_Filled.pencil`,
		want: ast.WidgetIcon{Kind: types.MenuIconCollection, Name: "Atlas_Core.Atlas_Filled.pencil"},
	}, {
		name: "image names an image collection",
		mdl:  `Icon: image MyModule.Images.logo`,
		want: ast.WidgetIcon{Kind: types.MenuIconImage, Name: "MyModule.Images.logo"},
	}, {
		name: "image accepts the quoted spelling too",
		mdl:  `Icon: image 'MyModule.Images.logo'`,
		want: ast.WidgetIcon{Kind: types.MenuIconImage, Name: "MyModule.Images.logo"},
	}, {
		// A glyph has a code and NO name — the whole reason one string could
		// never carry all three.
		name: "glyph carries a code",
		mdl:  `Icon: glyph 57377`,
		want: ast.WidgetIcon{Kind: types.MenuIconGlyph, Code: 57377},
	}, {
		// Atlas icon names are hyphenated and IDENTIFIER cannot lex a hyphen, so
		// that segment is double-quoted — per segment, as qualifiedName allows.
		// The quotes are SPELLING and do not survive into the AST: what reaches
		// BSON is the plain name, and re-quoting on the way out is the emitter's
		// job (quoteQualifiedName). A Name still carrying quotes here would be
		// written into the document verbatim and resolve to nothing.
		name: "a hyphenated segment is quoted on its own",
		mdl:  `Icon: Atlas_Core.Atlas."align-center"`,
		want: ast.WidgetIcon{Kind: types.MenuIconCollection, Name: "Atlas_Core.Atlas.align-center"},
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseButtonIcon(t, tc.mdl)
			if got == nil {
				t.Fatal("Icon: produced no WidgetIcon")
			}
			if *got != tc.want {
				t.Errorf("got %+v, want %+v", *got, tc.want)
			}
		})
	}
}

// TestWidgetIconV3_ImageIsAKeywordNotANameSegment pins the ANTLR ordering trap
// that navMenuIcon documents: qualifiedName accepts a keyword as a name segment,
// so `image Mod.Images.logo` ALSO matches the bare alternative with `image` read
// as the first segment. Listing the keyword-led alternatives first is what
// settles it — and if that ordering is ever lost, this is the test that says so
// rather than a mis-typed icon reaching a project.
func TestWidgetIconV3_ImageIsAKeywordNotANameSegment(t *testing.T) {
	got := parseButtonIcon(t, `Icon: image MyModule.Images.logo`)
	if got == nil {
		t.Fatal("Icon: produced no WidgetIcon")
	}
	if got.Kind != types.MenuIconImage {
		t.Fatalf("kind = %q, want image — `image` was read as a name segment", got.Kind)
	}
	if got.Name == "image.MyModule.Images.logo" {
		t.Errorf("the keyword was swallowed into the name: %q", got.Name)
	}
}

// parseButtonIcon runs the real parser over a button carrying one Icon property.
func parseButtonIcon(t *testing.T, iconProp string) *ast.WidgetIcon {
	t.Helper()
	src := "create page Mod.P (Title: 'T') {\n  actionbutton btn (Caption: 'X', " + iconProp + ")\n}"
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", iconProp, errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("got %d statements, want 1", len(prog.Statements))
	}
	page, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("got %T, want *ast.CreatePageStmtV3", prog.Statements[0])
	}
	if len(page.Widgets) != 1 {
		t.Fatalf("got %d widgets, want 1", len(page.Widgets))
	}
	icon, _ := page.Widgets[0].Properties["Icon"].(*ast.WidgetIcon)
	return icon
}
