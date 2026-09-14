// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// The legacy engine wrote `Icon: nil` on every action button, unconditionally.
// A button's icon has been authorable since #602 and only the modelsdk engine
// ever wrote one, so under `--engine legacy` the icon was dropped on every
// write — silently, because a null Icon is exactly what an iconless button
// stores and nothing downstream could tell the two apart (mendixlabs/mxcli#1059).
//
// These assert on the encoded document, which is the layer the defect lived in:
// the model carried the icon correctly the whole time.

// iconOf serializes a button and returns its Icon element as a plain map, or
// nil when the icon was written as null.
func iconOf(t *testing.T, icon *pages.Icon) map[string]any {
	t.Helper()
	doc := serializeActionButton(&pages.ActionButton{
		BaseWidget: pages.BaseWidget{Name: "btnEdit"},
		Icon:       icon,
	})
	for _, e := range doc {
		if e.Key != "Icon" {
			continue
		}
		if e.Value == nil {
			return nil
		}
		nested, ok := e.Value.(bson.D)
		if !ok {
			t.Fatalf("Icon is a %T, want bson.D", e.Value)
		}
		out := make(map[string]any, len(nested))
		for _, f := range nested {
			out[f.Key] = f.Value
		}
		return out
	}
	t.Fatal("serialized button has no Icon key at all")
	return nil
}

func TestSerializeActionButton_WritesEachIconElement(t *testing.T) {
	cases := []struct {
		name      string
		icon      *pages.Icon
		wantType  string
		wantImage string
		wantCode  int32
	}{{
		name:      "collection",
		icon:      &pages.Icon{Kind: types.MenuIconCollection, Image: "Atlas_Core.Atlas_Filled.pencil"},
		wantType:  "Forms$IconCollectionIcon",
		wantImage: "Atlas_Core.Atlas_Filled.pencil",
	}, {
		name:      "image",
		icon:      &pages.Icon{Kind: types.MenuIconImage, Image: "DesignSystem.Icons_SVG.edit"},
		wantType:  "Forms$ImageIcon",
		wantImage: "DesignSystem.Icons_SVG.edit",
	}, {
		name:     "glyph",
		icon:     &pages.Icon{Kind: types.MenuIconGlyph, Code: 57377},
		wantType: "Forms$GlyphIcon",
		wantCode: 57377,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := iconOf(t, tc.icon)
			if got == nil {
				t.Fatal("the icon was written as null — the legacy drop")
			}
			if got["$Type"] != tc.wantType {
				t.Errorf("$Type = %v, want %q", got["$Type"], tc.wantType)
			}
			if tc.wantImage != "" && got["Image"] != tc.wantImage {
				t.Errorf("Image = %v, want %q", got["Image"], tc.wantImage)
			}
			if tc.wantCode != 0 && got["Code"] != tc.wantCode {
				t.Errorf("Code = %v (%T), want %d as int32", got["Code"], got["Code"], tc.wantCode)
			}
			// A glyph has no name and a named icon has no code. Writing the
			// other variant's payload alongside is how a reader would then have
			// to guess which one the element really is.
			if tc.wantCode == 0 && got["Code"] != nil {
				t.Errorf("a named icon carries Code = %v", got["Code"])
			}
			if tc.wantImage == "" && got["Image"] != nil {
				t.Errorf("a glyph icon carries Image = %v", got["Image"])
			}
		})
	}
}

// CONTROL: an iconless button must still write a null Icon — that is what
// Studio Pro stores, and it is the TypeDefault the rest of the document expects.
func TestSerializeActionButton_NoIconStaysNull(t *testing.T) {
	if got := iconOf(t, nil); got != nil {
		t.Errorf("an iconless button wrote an icon element: %v", got)
	}
}

// CONTROL: an icon that identifies nothing is written as no icon rather than as
// an element nobody can see. The executor refuses these before they get here, so
// this pins the writer's own behaviour for the paths that build a pages.Icon
// directly.
func TestSerializeActionButton_AnIconIdentifyingNothingIsNotWritten(t *testing.T) {
	for _, icon := range []*pages.Icon{
		{Kind: types.MenuIconGlyph},                    // no code
		{Kind: types.MenuIconImage},                    // no name
		{Kind: types.MenuIconCollection},               // no name
		{Kind: types.MenuIconKind("Forms$SomeFuture")}, // a kind this build does not know
	} {
		if got := iconOf(t, icon); got != nil {
			t.Errorf("%+v was written as %v, want no icon", icon, got)
		}
	}
}
