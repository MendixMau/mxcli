// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// `Action: DELETE CLOSE_PAGE` is documented syntax (mxcli syntax page action).
// The visitor tested CLOSE_PAGE() before DELETE(), so the delete was read as a
// plain close: the button was stored as Forms$ClosePageClientAction and closed
// the page WITHOUT deleting — check and exec clean, describe showing
// `Action: close_page`. DELETE / DELETE_OBJECT alone were unaffected.
func TestBuildActionV3_DeleteClosePageKeepsTheDelete(t *testing.T) {
	prog, errs := Build(`create page M.P (Title: 'x', Layout: A.B) {
  actionbutton btnDel (Caption: 'Delete', Action: DELETE CLOSE_PAGE)
  actionbutton btnDel2 (Caption: 'Delete', Action: delete)
  actionbutton btnClose (Caption: 'Close', Action: close_page)
  actionbutton btnDel3 (Caption: 'Delete', Action: delete_object close_page)
}`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	ws := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets
	cases := []struct {
		i         int
		wantType  string
		wantClose bool
	}{
		{0, "delete", true},
		{1, "delete", false},
		{2, "close", false}, // control: a bare close is still a close
		// what DESCRIBE prints for a stored delete-and-close; it did not parse
		{3, "delete", true},
	}
	for _, c := range cases {
		a := ws[c.i].GetAction()
		if a == nil {
			t.Fatalf("%s: no action", ws[c.i].Name)
		}
		if a.Type != c.wantType || a.ClosePage != c.wantClose {
			t.Errorf("%s: action = {Type:%q ClosePage:%v}, want {Type:%q ClosePage:%v}",
				ws[c.i].Name, a.Type, a.ClosePage, c.wantType, c.wantClose)
		}
	}
}
