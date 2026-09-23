// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// pdsGetProperties is the PDS Dropdown Menu's getProperties, verbatim from the
// widget's compiled editorConfig.js. Its first statement hides two properties
// with no guard at all; Studio Pro therefore stores their TextTemplates null,
// and a replace that wrote them populated failed CE0463.
const pdsGetProperties = `exports.getProperties=function(e,t){return M.hidePropertiesIn(t,e,["ariaLabelCaption","moreOptionsCaption"]),"icon"===e.variant&&M.hidePropertiesIn(t,e,["buttonIcon","caption"]),e.dropdownItems.forEach(function(r,n){"button"!==r.itemType&&M.hidePropertyIn(t,e,"dropdownItems",n,"destructive"),"link"!==r.itemType&&M.hidePropertyIn(t,e,"dropdownItems",n,"url"),"divider"===r.itemType&&M.hidePropertyIn(t,e,"dropdownItems",n,"onClickAction")}),t}`

func TestExtractVisibility_UnconditionalHide(t *testing.T) {
	rules, stats := extractVisibilityRulesFromJS(pdsGetProperties)
	for _, key := range []string{"ariaLabelCaption", "moreOptionsCaption"} {
		r := findRule(rules, key)
		if r == nil || r.HiddenWhen == nil || !r.HiddenWhen.Always() || r.Nested() {
			t.Errorf("%s: got %+v, want an unconditional top-level rule", key, r)
			continue
		}
		fires, determinable := r.Fires(func(types.WidgetVisibilityCondition) (string, bool) { return "", false })
		if !fires || !determinable {
			t.Errorf("%s: Fires = %v,%v, want true,true with no lookups", key, fires, determinable)
		}
	}
	// The guarded rules are unchanged.
	if r := findRule(rules, "caption"); r == nil || r.HiddenWhen == nil || r.HiddenWhen.Operator != "eq" || r.HiddenWhen.Value != "icon" {
		t.Errorf("caption: got %+v, want hidden when variant==icon", r)
	}
	if stats.SkippedComplex != 0 {
		t.Errorf("SkippedComplex = %d, want 0", stats.SkippedComplex)
	}
}

// A hide that is not the first thing getProperties does is NOT unconditional,
// and a hide at the start of some other function says nothing about visibility.
func TestExtractVisibility_UnconditionalHideNarrow(t *testing.T) {
	for _, js := range []string{
		`exports.check=function(e){return M.hidePropertiesIn(t,e,["a"]),[]}`,
		`exports.getProperties=function(e,t){return x(e),M.hidePropertiesIn(t,e,["a"]),t}`,
		`exports.getProperties=function(e,t){return e.items.forEach(function(r,n){M.hidePropertyIn(t,e,"items",n,"a")}),t}`,
	} {
		rules, _ := extractVisibilityRulesFromJS(js)
		for _, r := range rules {
			if r.HiddenWhen != nil && r.HiddenWhen.Always() {
				t.Errorf("%s: produced an unconditional rule %+v", js, r)
			}
		}
	}
}
