// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1059, second half: authoring the two icon elements MDL could
// not spell.
//
// Making the loss visible (the `-- NOT re-executable` note) stopped describe ->
// exec from destroying an icon, but it did not let anyone WRITE one. These pin
// the builder: the kind the author wrote must reach the element `$Type`, since
// the two named kinds are spelled identically and only the kind separates a
// custom-icon reference from an image one.

func buildIconButton(t *testing.T, icon any) (*pages.ActionButton, error) {
	t.Helper()
	pb := &pageBuilder{widgetScope: map[string]model.ID{}}
	props := map[string]any{"Caption": "Edit"}
	if icon != nil {
		props["Icon"] = icon
	}
	widget, err := pb.buildWidgetV3(&ast.WidgetV3{Type: "actionbutton", Name: "btnEdit", Properties: props})
	if err != nil {
		return nil, err
	}
	return widget.(*pages.ActionButton), nil
}

func TestBuildWidgetIcon_EachKindBuildsItsOwnElement(t *testing.T) {
	cases := []struct {
		name      string
		icon      *ast.WidgetIcon
		wantType  string
		wantImage string
		wantCode  int
	}{{
		name:      "collection",
		icon:      &ast.WidgetIcon{Kind: types.MenuIconCollection, Name: "Atlas_Core.Atlas_Filled.pencil"},
		wantType:  "Forms$IconCollectionIcon",
		wantImage: "Atlas_Core.Atlas_Filled.pencil",
	}, {
		// The one the report hit. Same spelling as the line above, different
		// document, different element — CE1613 if it lands as a collection icon.
		name:      "image",
		icon:      &ast.WidgetIcon{Kind: types.MenuIconImage, Name: "DesignSystem.Icons_SVG.edit"},
		wantType:  "Forms$ImageIcon",
		wantImage: "DesignSystem.Icons_SVG.edit",
	}, {
		name:     "glyph",
		icon:     &ast.WidgetIcon{Kind: types.MenuIconGlyph, Code: 57377},
		wantType: "Forms$GlyphIcon",
		wantCode: 57377,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			btn, err := buildIconButton(t, tc.icon)
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			if btn.Icon == nil {
				t.Fatal("icon dropped")
			}
			if btn.Icon.TypeName != tc.wantType {
				t.Errorf("$Type = %q, want %q", btn.Icon.TypeName, tc.wantType)
			}
			if btn.Icon.Image != tc.wantImage {
				t.Errorf("Image = %q, want %q", btn.Icon.Image, tc.wantImage)
			}
			if btn.Icon.Code != tc.wantCode {
				t.Errorf("Code = %d, want %d", btn.Icon.Code, tc.wantCode)
			}
		})
	}
}

// TestBuildWidgetIcon_LegacyStringIsStillTheCollectionIcon is the compatibility
// CONTROL. `Icon:` has carried a plain quoted string since #602, in scripts and
// in callers that build a WidgetV3 directly, and that string has only ever meant
// an icon-collection icon. If this stops holding, every existing page script
// silently changes meaning.
func TestBuildWidgetIcon_LegacyStringIsStillTheCollectionIcon(t *testing.T) {
	for _, raw := range []string{"Atlas_Core.Atlas_Filled.pencil", "'Atlas_Core.Atlas_Filled.pencil'"} {
		btn, err := buildIconButton(t, raw)
		if err != nil {
			t.Fatalf("build %q: %v", raw, err)
		}
		if btn.Icon == nil {
			t.Fatalf("%q: icon dropped", raw)
		}
		if btn.Icon.TypeName != "Forms$IconCollectionIcon" || btn.Icon.Image != "Atlas_Core.Atlas_Filled.pencil" {
			t.Errorf("%q built %s{%s}, want Forms$IconCollectionIcon{Atlas_Core.Atlas_Filled.pencil}",
				raw, btn.Icon.TypeName, btn.Icon.Image)
		}
	}
}

// TestBuildWidgetIcon_NoIconIsNoElement is the second CONTROL: a button without
// an `Icon:` must carry no icon element at all, not an empty one.
func TestBuildWidgetIcon_NoIconIsNoElement(t *testing.T) {
	btn, err := buildIconButton(t, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if btn.Icon != nil {
		t.Errorf("an iconless button got an icon element: %+v", btn.Icon)
	}
}

// TestBuildWidgetIcon_RefusesAnIconThatIdentifiesNothing covers the failure that
// would otherwise be invisible. A bare Forms$GlyphIcon with no Code, or a named
// icon with no name, builds at 0 errors and renders nothing — so the author sees
// a button with no icon and no reason for it. A glyph's code is not a detail: it
// is the ONLY thing that identifies a glyph icon.
func TestBuildWidgetIcon_RefusesAnIconThatIdentifiesNothing(t *testing.T) {
	cases := []struct {
		name string
		icon *ast.WidgetIcon
		want string
	}{
		{"glyph with no code", &ast.WidgetIcon{Kind: types.MenuIconGlyph}, "character code"},
		{"image with no name", &ast.WidgetIcon{Kind: types.MenuIconImage}, "qualified name"},
		{"collection with no name", &ast.WidgetIcon{Kind: types.MenuIconCollection}, "qualified name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildIconButton(t, tc.icon)
			if err == nil {
				t.Fatal("an icon identifying nothing was accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not say what to write (want %q in it)", err, tc.want)
			}
		})
	}
}
