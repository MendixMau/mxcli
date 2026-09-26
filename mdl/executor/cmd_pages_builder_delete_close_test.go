// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// `Action: DELETE CLOSE_PAGE` must reach the model as a delete that closes the
// page: Forms$DeleteClientAction.ClosePage. The builder dropped the flag.
func TestBuildClientAction_DeleteKeepsClosePage(t *testing.T) {
	pb := &pageBuilder{}
	for _, want := range []bool{true, false} {
		act, err := pb.buildClientActionV3(&ast.ActionV3{Type: "delete", ClosePage: want})
		if err != nil {
			t.Fatal(err)
		}
		del, ok := act.(*pages.DeleteClientAction)
		if !ok {
			t.Fatalf("got %T, want *pages.DeleteClientAction", act)
		}
		if del.ClosePage != want {
			t.Errorf("ClosePage = %v, want %v", del.ClosePage, want)
		}
	}
}
