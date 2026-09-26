// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// validateGalleryOnClickSelection (MDL-WIDGET35) reports a Gallery whose item
// click action sits beside an item selection.
//
// A Gallery triggers its `onClick` on a single click (the definition fixes
// onClickTrigger to "single"), and a single click on an item is also what
// selects it. Mendix refuses that combination: with `OnClick:` and the
// definition's default `Selection: Single`, exec wrote the page and mx check
// (Mendix 11.14.0) reported "The item click action is ambiguous. Change "On
// click trigger" to "Double click" or set "Selection" to "None"." MDL has no
// spelling for the trigger, so the remedy is `selection: none`, which checks
// clean.
func validateGalleryOnClickSelection(w *ast.WidgetV3, locationPrefix string) []linter.Violation {
	if w == nil || !strings.EqualFold(w.Type, "gallery") || w.GetAction() == nil {
		return nil
	}
	selection := "Single" // the Gallery definition's default for itemSelection
	if v, ok := lookupPropCI(w, "Selection"); ok {
		selection = strings.TrimSpace(fmt.Sprint(v))
	}
	if strings.EqualFold(selection, "none") {
		return nil
	}
	return []linter.Violation{{
		RuleID:   "MDL-WIDGET35",
		Severity: linter.SeverityError,
		Message: fmt.Sprintf(
			"%s: gallery `%s` has an OnClick action and Selection %s — a single click on an item would both select it and run the action, which Mendix rejects as ambiguous",
			locationPrefix, w.Name, selection),
		Suggestion: "Add `selection: none` to the gallery, or drop OnClick and put the action on a clickable `container` inside the template",
	}}
}
