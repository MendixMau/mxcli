// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A microflow parameter written `enum M.Kinds` or `Enumeration(M.Kinds)` is an
// enumeration beyond doubt, and must say so: checks that act only on a certain
// enum (a string literal passed to it is CE0117) read ExplicitEnum, and the
// microflow builders never set it, so those checks stayed silent.
func TestMicroflowEnumParamIsExplicit(t *testing.T) {
	prog, errs := Build("create microflow M.SUB ($A: enum M.Kinds, $B: Enumeration(M.Kinds), $C: M.Thing) begin end;\n/\n")
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	ps := prog.Statements[0].(*ast.CreateMicroflowStmt).Parameters
	for _, i := range []int{0, 1} {
		if ps[i].Type.Kind != ast.TypeEnumeration || !ps[i].Type.ExplicitEnum {
			t.Errorf("param $%s: got %+v, want explicit enumeration", ps[i].Name, ps[i].Type)
		}
	}
	if ps[2].Type.ExplicitEnum {
		t.Errorf("bare $C: M.Thing must not be marked an explicit enum: %+v", ps[2].Type)
	}
}
