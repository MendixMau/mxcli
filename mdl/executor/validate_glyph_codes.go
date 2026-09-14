// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// validateGlyphCodes (MDL078) flags a glyph icon whose code the Mendix glyph
// font does not define — on a navigation menu item, a menu document, or a page
// widget.
//
// # Why this needs its own rule
//
// A glyph code is a bare integer: nothing resolves it, so it passes `mxcli check`
// AND `mx check` at 0 errors, and fails only at `mxbuild --target=deploy` — which
// is what `mxcli run --local` does. The message it fails with names a document
// that is not the problem:
//
//	ERROR: One or more errors occurred.
//	(An exception occurred while exporting layout 'CapTrack.App_Default.')
//
// Neither the navigation nor the menu item is mentioned. Bisecting it cost three
// build cycles (ako/CapTrackV4 007, R2).
//
// A glyph on a page WIDGET fails the same way and names the page instead —
// measured on 11.13.0 with `Icon: glyph 57562` on an action button:
//
//	(An exception occurred while exporting page 'MyFirstModule.IconProbe'.)
//	 ---> System.InvalidOperationException: Sequence contains no matching element
//
// So the message must not promise "layout": the document named is whichever one
// contains the icon, which is exactly the document the author will not suspect.
//
// # The mechanism, and why a code-point table is the right check
//
// mxbuild resolves the code through GlyphFont.GetClass(Int32), which is a LINQ
// `.First(...)` over its glyph table and throws `InvalidOperationException:
// Sequence contains no matching element` when the code is absent. Measured on
// 11.14.0 with two builds: 57562 (0xEBDA, past the font's 0xE260 end) produces
// exactly that stack, and 57377 (0xE021, in the font) exports pages and layouts
// cleanly. The font's cmap and mxbuild's table agree on both points.
//
// # A warning, not an error
//
// The table is a snapshot of a Mendix asset. If Mendix ever extends the font,
// a correct code would be reported here, and refusing it would be worse than the
// gap this closes — so `exec` still writes it and the author still gets told.
//
// # It needs no project
//
// The code is in the script. This runs in the project-free pass alongside
// MDL077, which is how CI reaches it.
//
// # One walk
//
// The statements are enumerated by iconRefsInStatement, the package's single
// icon walk, rather than by a second switch here. This rule covered menu items
// only while menus were the only place a glyph could be authored; the day
// `Icon: glyph <n>` landed on a widget (mendixlabs/mxcli#1059) a private walk
// would have kept passing every widget glyph in silence, which is the drift the
// shared walk exists to prevent.
func validateGlyphCodes(stmt ast.Statement) []linter.Violation {
	var out []linter.Violation
	for _, ref := range iconRefsInStatement(stmt) {
		if ref.kind != types.MenuIconGlyph || glyphCodeDefined(ref.code) {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL078",
			Severity: linter.SeverityWarning,
			Message: fmt.Sprintf(
				"%s uses glyph %d, which the Mendix glyph font does not define — the build fails "+
					"at `mxbuild --target=deploy` with \"An exception occurred while exporting "+
					"<page or layout> '<name>'\" and \"Sequence contains no matching element\", "+
					"which names the document the icon is IN and never the icon",
				ref.where, ref.code),
			Suggestion: "prefer an icon collection reference, which IS resolved before anything " +
				"is written: `Atlas_Core.Atlas.<name>` (list them with " +
				"`describe icon collection Atlas_Core.Atlas`). To keep a glyph, pick a code from " +
				"`show glyphs` — the font's codes are sparse, so nearby numbers are usually " +
				"not defined either.",
		})
	}
	return out
}
