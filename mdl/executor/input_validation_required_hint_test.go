// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// `Required: true` is the spelling people reach for; Studio Pro stores its
// "Required" validation preset as the expression `$value != empty`, which MDL
// writes with `Validation:`. The warning must name that, not a near-miss key.
func TestRequiredKeyNamesTheValidationSpelling(t *testing.T) {
	vs := validateStaticWidgetUnknownProps(&ast.WidgetV3{Type: "TEXTBOX", Name: "t",
		Properties: map[string]any{"Required": true}}, "page P")
	if len(vs) != 1 || !strings.Contains(vs[0].Message, "Validation: '$value != empty'") {
		t.Errorf("got %v, want one MDL-WIDGET07 naming Validation: '$value != empty'", vs)
	}
}
