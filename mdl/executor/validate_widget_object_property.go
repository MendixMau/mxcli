// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// validateObjectEntryProperties (MDL-WIDGET27) rejects a repeatable widget
// property written as a property VALUE:
//
//	htmlelement frame ( attributes: [(attributeName: 'data-x')] )
//
// # Why this is an error rather than a fix-up
//
// mendixlabs/mxcli#999. The shape had two failure modes and the dangerous one
// looked like success:
//
//	[(configMode: simple)]            parsed as a list of EXPRESSIONS, flattened
//	                                  to a []string no writer claimed — check
//	                                  clean, exec successful, property gone from
//	                                  storage
//	[(configMode: simple, x: y)]      died as `missing ')' at ','`, which names a
//	                                  paren and leaves the author to guess
//
// The reporter's framing was "check/exec-pass-then-discard". On FileUploader the
// dropped property was `allowedFileFormats`, which restricts what a user may
// upload — so the silent drop is a correctness problem and a mild security one
// in the built app.
//
// MDL already has a spelling for these, and it works: the entries are CONTAINER
// BLOCKS in the widget body, which slices 2-3 of the def-driven widget work made
// authorable for every widget with a definition.
//
//	htmlelement frame ( tagName: 'div' ) {
//	  attribute a1 (attributeName: 'data-x', attributeValueType: 'expression')
//	}
//
// So this is NOT wired to the object-list builder as a second route. Two
// spellings for one construct is the anti-pattern the syntax design guide names:
// it doubles what a reader has to learn, doubles what DESCRIBE must choose
// between, and the two would drift. The property form is reported, and the
// message names the container keyword so the error carries its own remedy.
//
// # An error, not a warning
//
// A warning would leave `exec` free to write the page and discard the entries,
// which is the bug. `exec` refuses only on errors.
//
// # It needs no project
//
// The SHAPE is wrong regardless of what the widget declares, so the rule fires
// without `-p` — which is how `make check-mdl` runs, and how a rule that needed
// a definition would be inert in CI. A definition, when there is one, only makes
// the message better: it supplies the container keyword.
func validateObjectEntryProperties(w *ast.WidgetV3, registry *WidgetRegistry, locationPrefix string) []linter.Violation {
	if w == nil || len(w.Properties) == 0 {
		return nil
	}

	// Sorted, because ranging a map emits violations in a random order and two
	// runs of the same binary then disagree — the noise floor that had to be
	// fixed before the slice 2-3 corpus diff could be read at all.
	keys := make([]string, 0, len(w.Properties))
	for k := range w.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []linter.Violation
	for _, key := range keys {
		value := w.Properties[key]
		// One violation per property, so the three shapes below are exclusive:
		// an empty list on a declared container matches two of them.
		switch {
		case isObjectEntryList(value):
			entries, _ := value.(*ast.ObjectEntryListV3)
			out = append(out, linter.Violation{
				RuleID:   "MDL-WIDGET27",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"%s: widget `%s` property `%s` is a repeated entry written as a property value — "+
						"MDL writes these as %s in the widget body, and a value here is discarded on write",
					locationPrefix, w.Name, key, objectEntryRemedy(w, registry, key)),
				Suggestion: objectEntryExample(w, registry, key, entries),
			})
		case isEmptyListValue(value):
			// `p: []`. The generic `[expr, …]` branch of the visitor turns this
			// into an empty []string that no writer claims, so it is #999's
			// silent drop reached by a different spelling (mendixlabs/mxcli#1056).
			// Keyed on EMPTINESS, never on the brackets: `visible: [expr]` and a
			// filter's `attributes: [Name]` are how MDL spells those properties.
			out = append(out, linter.Violation{
				RuleID:   "MDL-WIDGET27",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"%s: widget `%s` property `%s` is an empty list value, which writes nothing — "+
						"%s",
					locationPrefix, w.Name, key, emptyListRemedy(w, registry, key)),
				Suggestion: objectEntryExample(w, registry, key, nil),
			})
		case isDeclaredContainer(w, registry, key):
			// The definition says this property holds repeated entries or child
			// widgets, so NO value in the property list can reach storage. Only
			// knowable with a project; without one `p: 'x'` is the ordinary
			// property form and flagging it would be a guess.
			out = append(out, linter.Violation{
				RuleID:   "MDL-WIDGET27",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"%s: widget `%s` property `%s` holds %s, not a value — "+
						"a value here is discarded on write",
					locationPrefix, w.Name, key, containerNoun(w, registry, key)),
				Suggestion: objectEntryExample(w, registry, key, nil),
			})
		}
	}
	return out
}

func isObjectEntryList(v any) bool {
	entries, ok := v.(*ast.ObjectEntryListV3)
	return ok && entries != nil
}

// isEmptyListValue reports `p: []`. The visitor renders a bracketed value with
// no elements as an empty []string — the one shape that can produce one.
func isEmptyListValue(v any) bool {
	items, ok := v.([]string)
	return ok && len(items) == 0
}

// isDeclaredContainer reports whether the widget's definition declares this
// property as an object list or a child slot. False without a definition, which
// is what keeps the scalar case project-gated.
func isDeclaredContainer(w *ast.WidgetV3, registry *WidgetRegistry, propertyKey string) bool {
	kw, _ := containerKeyword(w, registry, propertyKey)
	return kw != ""
}

// containerNoun describes what the property holds, for the message.
func containerNoun(w *ast.WidgetV3, registry *WidgetRegistry, propertyKey string) string {
	if _, slot := containerKeyword(w, registry, propertyKey); slot {
		return "child widgets"
	}
	return "repeated entries"
}

// emptyListRemedy names the container when the definition declares one, and
// otherwise says only what is certain: the value does not reach storage.
func emptyListRemedy(w *ast.WidgetV3, registry *WidgetRegistry, propertyKey string) string {
	kw, slot := containerKeyword(w, registry, propertyKey)
	switch {
	case kw != "" && slot:
		return fmt.Sprintf("`%s` holds child widgets, written as `%s <name> { … }` in the widget body", propertyKey, kw)
	case kw != "":
		return fmt.Sprintf("`%s` holds repeated entries, written as `%s <name> (…)` in the widget body", propertyKey, kw)
	}
	return "remove it, or — if this property takes repeated entries or child widgets — " +
		"write them as blocks in the widget body"
}

// objectEntryRemedy names the container keyword when the widget's definition
// declares one, and describes the shape when it does not — with no project there
// is no definition to consult, and naming a keyword that might be wrong is worse
// than describing the form.
func objectEntryRemedy(w *ast.WidgetV3, registry *WidgetRegistry, propertyKey string) string {
	if kw := objectEntryKeyword(w, registry, propertyKey); kw != "" {
		return fmt.Sprintf("`%s <name> (…)` blocks", kw)
	}
	return "`<container> <name> (…)` blocks"
}

// objectEntryKeyword resolves the property key to the MDL container keyword the
// widget declares for it, or "" when it cannot be known.
func objectEntryKeyword(w *ast.WidgetV3, registry *WidgetRegistry, propertyKey string) string {
	kw, _ := containerKeyword(w, registry, propertyKey)
	return kw
}

// containerKeyword resolves a property key to the MDL container keyword the
// widget's definition declares for it, and reports whether that container is a
// CHILD SLOT (holds widgets, written as `kw name { … }`) rather than an object
// list (holds entries, written as `kw name (…)`). The two spell their remedy
// differently, so a rule that conflated them would print an example that does
// not parse. Returns "" when there is no definition to consult.
func containerKeyword(w *ast.WidgetV3, registry *WidgetRegistry, propertyKey string) (keyword string, isSlot bool) {
	def := lookupWidgetDef(w, registry)
	if def == nil {
		return "", false
	}
	for _, ol := range def.ObjectLists {
		if strings.EqualFold(ol.PropertyKey, propertyKey) {
			return strings.ToLower(ol.MDLContainer), false
		}
	}
	for _, cs := range def.ChildSlots {
		if strings.EqualFold(cs.PropertyKey, propertyKey) {
			return strings.ToLower(cs.MDLContainer), true
		}
	}
	return "", false
}

// objectEntryExample rewrites what the author wrote into the form that works, so
// the fix is a copy rather than a translation exercise. Falls back to naming the
// discovery command when the keyword is unknown.
func objectEntryExample(w *ast.WidgetV3, registry *WidgetRegistry, propertyKey string, entries *ast.ObjectEntryListV3) string {
	kw, slot := containerKeyword(w, registry, propertyKey)
	if kw == "" {
		return "move the entries into the widget body as container blocks; " +
			"`mxcli widget describe <widget> -p <project.mpr>` lists the container keywords"
	}
	if slot {
		return fmt.Sprintf("write `%s %s1 { … }` inside the widget body, holding the child widgets", kw, kw)
	}
	if entries == nil || len(entries.Entries) == 0 {
		return fmt.Sprintf("write `%s %s1 (…)` inside the widget body", kw, kw)
	}
	var parts []string
	for k, v := range entries.Entries[0] {
		parts = append(parts, fmt.Sprintf("%s: %s", k, formatEntryValue(v)))
	}
	sort.Strings(parts)
	return fmt.Sprintf("write `%s %s1 (%s)` inside the widget body", kw, kw, strings.Join(parts, ", "))
}

func formatEntryValue(v any) string {
	switch t := v.(type) {
	case string:
		return "'" + t + "'"
	case nil:
		return "''"
	default:
		return fmt.Sprintf("%v", t)
	}
}
