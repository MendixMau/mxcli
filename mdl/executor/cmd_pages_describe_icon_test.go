// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1059: `describe page` -> `create or replace page` turned a
// button's icon into a broken reference, and the build was the first thing that
// said so:
//
//	[error] [CE1613] "The selected custom icon 'DesignSystem.Icons_SVG.edit'
//	no longer exists."
//
// Mendix stores THREE icon elements and two of them put a qualified name under
// the same BSON key:
//
//	Forms$IconCollectionIcon{Image} -> CustomIcons$CustomIcon
//	Forms$ImageIcon{Image}          -> Images$Image        (a DIFFERENT document)
//	Forms$GlyphIcon{Code}           -> a font code point, no name at all
//
// DESCRIBE read `Image` off whichever element was there and the page builder
// wrote Forms$IconCollectionIcon unconditionally, so an image icon came back as
// a custom-icon reference — the silent variant swap. The same three-way split
// was already understood one layer over, in navigation
// (types.MenuIconKindOf, cmd_navigation_icon_roundtrip_test.go); the page path
// never received it.
//
// These tests assert on the emitted MDL rather than on the extractor, because
// the defect was that read, emit and write disagreed about what an icon IS.

// iconButton is one stored Forms$ActionButton carrying one icon element.
func iconButton(iconType, image string, code int) map[string]any {
	icon := map[string]any{"$Type": iconType}
	if image != "" {
		icon["Image"] = image
	}
	if code != 0 {
		icon["Code"] = code
	}
	return map[string]any{
		"$Type": "Forms$ActionButton",
		"Name":  "btnEdit",
		"Icon":  icon,
	}
}

// describeButton runs the real describe path over a stored widget map.
func describeButton(t *testing.T, w map[string]any) string {
	t.Helper()
	ctx, _ := newMockCtx(t)
	var buf bytes.Buffer
	ctx.Output = &buf
	for _, rw := range parseRawWidget(ctx, w) {
		outputWidgetMDLV3(ctx, rw, 0)
	}
	return buf.String()
}

// TestDescribeButtonIcon_RoundTripsEveryVariant is the whole point: DESCRIBE's
// output, re-executed, has to rebuild the SAME element — not merely something
// that parses.
//
// That is the assertion the reported bug would have failed. An image icon came
// out spelled exactly like an icon-collection reference, so it parsed, executed
// and produced a document whose build said `[CE1613] "The selected custom icon
// 'DesignSystem.Icons_SVG.edit' no longer exists."` A glyph icon came out as
// nothing at all and replay deleted it.
//
// The round trip runs the real parser and the real page builder, because the
// defect was that read, emit and write each had a different idea of what an icon
// is — asserting on any one of them alone is what let them drift.
func TestDescribeButtonIcon_RoundTripsEveryVariant(t *testing.T) {
	cases := []struct {
		name     string
		stored   map[string]any
		wantMDL  string
		wantType string
		wantName string
		wantCode int
	}{{
		name:     "collection",
		stored:   iconButton("Forms$IconCollectionIcon", "Atlas_Core.Atlas_Filled.pencil", 0),
		wantMDL:  "Icon: 'Atlas_Core.Atlas_Filled.pencil'",
		wantType: "Forms$IconCollectionIcon",
		wantName: "Atlas_Core.Atlas_Filled.pencil",
	}, {
		// The reported case. Same spelling as the row above until the keyword
		// tells them apart.
		name:     "image",
		stored:   iconButton("Forms$ImageIcon", "DesignSystem.Icons_SVG.edit", 0),
		wantMDL:  "Icon: image 'DesignSystem.Icons_SVG.edit'",
		wantType: "Forms$ImageIcon",
		wantName: "DesignSystem.Icons_SVG.edit",
	}, {
		// The half that was worse than reported: no name, so nothing to emit,
		// so silence — and CREATE OR REPLACE PAGE is a full replacement.
		name:     "glyph",
		stored:   iconButton("Forms$GlyphIcon", "", 57377),
		wantMDL:  "Icon: glyph 57377",
		wantType: "Forms$GlyphIcon",
		wantCode: 57377,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := describeButton(t, tc.stored)
			if !strings.Contains(out, tc.wantMDL) {
				t.Fatalf("describe did not emit %q:\n%s", tc.wantMDL, out)
			}
			if strings.Contains(out, "NOT re-executable") {
				t.Errorf("an authorable icon was flagged as unauthorable:\n%s", out)
			}

			icon := rebuildButtonIcon(t, tc.wantMDL)
			if icon == nil {
				t.Fatal("re-executing DESCRIBE's own output produced no icon")
			}
			if icon.TypeName != tc.wantType {
				t.Errorf("replay rebuilt %s, want %s — a silent variant swap",
					icon.TypeName, tc.wantType)
			}
			if icon.Image != tc.wantName {
				t.Errorf("replay rebuilt Image %q, want %q", icon.Image, tc.wantName)
			}
			if icon.Code != tc.wantCode {
				t.Errorf("replay rebuilt Code %d, want %d", icon.Code, tc.wantCode)
			}
		})
	}
}

// rebuildButtonIcon parses one `Icon:` clause inside a real page statement and
// runs the page builder over it, returning the icon element replay would store.
func rebuildButtonIcon(t *testing.T, iconClause string) *pages.Icon {
	t.Helper()
	src := "create page Mod.P (Title: 'T') {\n  actionbutton btnEdit (Caption: 'Edit', " + iconClause + ")\n}"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("DESCRIBE emitted MDL that does not parse (%q): %v", iconClause, errs)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	pb := &pageBuilder{widgetScope: map[string]model.ID{}}
	widget, err := pb.buildWidgetV3(page.Widgets[0])
	if err != nil {
		t.Fatalf("building %q: %v", iconClause, err)
	}
	return widget.(*pages.ActionButton).Icon
}

// TestDescribeButtonIcon_UnknownTypeIsFlagged pins the reason the note is keyed
// on the KIND rather than on a list of known-bad types: a stored $Type this
// build does not recognise must be reported, never treated as "no icon". That is
// how a future fourth variant would be silently dropped.
func TestDescribeButtonIcon_UnknownTypeIsFlagged(t *testing.T) {
	got := describeButton(t, iconButton("Forms$SomeFutureIcon", "Mod.Coll.thing", 0))
	if strings.Contains(got, "Icon: 'Mod.Coll.thing'") || strings.Contains(got, "Icon: image") {
		t.Errorf("an unrecognised icon element was emitted as one of the known kinds:\n%s", got)
	}
	assertIconNote(t, got, "Mod.Coll.thing", "Forms$SomeFutureIcon")
}

// TestDescribeButtonIcon_NoIconIsSilent is the second CONTROL: a button with no
// icon at all must produce neither a clause nor a note. Without it the note
// could fire unconditionally and every test above would still pass.
func TestDescribeButtonIcon_NoIconIsSilent(t *testing.T) {
	got := describeButton(t, map[string]any{
		"$Type": "Forms$ActionButton",
		"Name":  "btnEdit",
	})
	if strings.Contains(got, "Icon") || strings.Contains(got, "NOT re-executable") {
		t.Errorf("an iconless button produced icon output:\n%s", got)
	}
}

// TestDescribeButtonIcon_MalformedPayloadIsFlagged covers the other half of what
// a note is still for. A stored element of a KNOWN kind whose payload is missing
// — a glyph with no Code, a named icon with no name — cannot be emitted as a
// clause: there is nothing in it to rebuild the same icon from, and a clause
// with a guessed payload would rebuild a different one. So it is reported rather
// than either dropped or invented.
func TestDescribeButtonIcon_MalformedPayloadIsFlagged(t *testing.T) {
	t.Run("glyph with no code", func(t *testing.T) {
		got := describeButton(t, iconButton("Forms$GlyphIcon", "", 0))
		if strings.Contains(got, "Icon: glyph") {
			t.Errorf("a glyph with no code was emitted as a clause:\n%s", got)
		}
		assertIconNote(t, got, "no reference stored", "Forms$GlyphIcon")
	})
	t.Run("image with no name", func(t *testing.T) {
		got := describeButton(t, iconButton("Forms$ImageIcon", "", 0))
		if strings.Contains(got, "Icon: image") {
			t.Errorf("an image icon with no name was emitted as a clause:\n%s", got)
		}
		assertIconNote(t, got, "no reference stored", "Forms$ImageIcon")
	})
}

func assertIconNote(t *testing.T, got, wantTarget, wantType string) {
	t.Helper()
	if !strings.Contains(got, "NOT re-executable") {
		t.Errorf("no NOT re-executable note — re-running this output drops the icon silently:\n%s", got)
		return
	}
	for _, want := range []string{wantTarget, wantType} {
		if !strings.Contains(got, want) {
			t.Errorf("the note does not name %q, so it cannot be acted on:\n%s", want, got)
		}
	}
}
