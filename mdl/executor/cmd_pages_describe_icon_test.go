// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
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

// TestDescribeButtonIcon_CollectionRoundTrips is the CONTROL. An icon-collection
// icon is the one variant mxcli can author, so it must keep emitting a plain
// `Icon:` clause and no warning — otherwise the fix below would "pass" by
// flagging everything.
func TestDescribeButtonIcon_CollectionRoundTrips(t *testing.T) {
	got := describeButton(t, iconButton("Forms$IconCollectionIcon", "Atlas_Core.Atlas_Filled.pencil", 0))
	if !strings.Contains(got, "Icon: 'Atlas_Core.Atlas_Filled.pencil'") {
		t.Errorf("the authorable variant lost its Icon clause:\n%s", got)
	}
	if strings.Contains(got, "NOT re-executable") {
		t.Errorf("an authorable icon was flagged as unauthorable:\n%s", got)
	}
}

// TestDescribeButtonIcon_ImageIconIsNotEmittedAsACollectionRef is the reported
// bug. The qualified name must NOT come out as a bare `Icon:` clause: that
// clause is spelled the same as a collection reference, so re-executing it
// rebuilds the wrong element.
func TestDescribeButtonIcon_ImageIconIsNotEmittedAsACollectionRef(t *testing.T) {
	got := describeButton(t, iconButton("Forms$ImageIcon", "DesignSystem.Icons_SVG.edit", 0))
	if strings.Contains(got, "Icon: 'DesignSystem.Icons_SVG.edit'") {
		t.Errorf("an image icon was emitted as an icon-collection reference — "+
			"re-executing this is CE1613:\n%s", got)
	}
	assertIconNote(t, got, "DesignSystem.Icons_SVG.edit", "Forms$ImageIcon")
}

// TestDescribeButtonIcon_GlyphIconIsFlagged covers the half that was worse than
// reported: a glyph icon has no qualified name, so DESCRIBE emitted NOTHING —
// not a clause, not a comment. CREATE OR REPLACE PAGE is a full replacement, so
// re-running that output deleted the icon with an exit 0 and a success message.
func TestDescribeButtonIcon_GlyphIconIsFlagged(t *testing.T) {
	got := describeButton(t, iconButton("Forms$GlyphIcon", "", 57377))
	assertIconNote(t, got, "57377", "Forms$GlyphIcon")
}

// TestDescribeButtonIcon_UnknownTypeIsFlagged pins the reason the note is keyed
// on the KIND rather than on a list of known-bad types: a stored $Type this
// build does not recognise must be reported, never treated as "no icon". That is
// how a future fourth variant would be silently dropped.
func TestDescribeButtonIcon_UnknownTypeIsFlagged(t *testing.T) {
	got := describeButton(t, iconButton("Forms$SomeFutureIcon", "Mod.Coll.thing", 0))
	if strings.Contains(got, "Icon: 'Mod.Coll.thing'") {
		t.Errorf("an unrecognised icon element was emitted as a collection reference:\n%s", got)
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

// TestBuildButtonV3_IconAlwaysBuildsACollectionIcon documents WHY the describer
// has to decline: the builder has exactly one icon element. Any name reaching
// `Icon:` is written as a custom-icon reference regardless of what it names, so
// emitting an image icon's name there is emitting a CE1613.
//
// This is the "prove the fix is the cause" control in reverse — it asserts the
// unchanged write-side behaviour the read-side guard exists to protect against,
// and it will start failing the day `Icon: image …` is authorable.
func TestBuildButtonV3_IconAlwaysBuildsACollectionIcon(t *testing.T) {
	pb := &pageBuilder{widgetScope: map[string]model.ID{}}
	widget, err := pb.buildWidgetV3(&ast.WidgetV3{
		Type:       "actionbutton",
		Name:       "btnEdit",
		Properties: map[string]any{"Caption": "Edit", "icon": "DesignSystem.Icons_SVG.edit"},
	})
	if err != nil {
		t.Fatalf("buildWidgetV3: %v", err)
	}
	btn := widget.(*pages.ActionButton)
	if btn.Icon == nil {
		t.Fatal("icon dropped")
	}
	if btn.Icon.TypeName != "Forms$IconCollectionIcon" {
		t.Errorf("Icon.TypeName = %q — the builder gained a second variant, so the "+
			"describer may now emit a clause for it", btn.Icon.TypeName)
	}
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
