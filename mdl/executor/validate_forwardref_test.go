// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestAnnotateForwardRef_SkipsOwnName covers a misfire found while adding
// MDL054: a statement's OWN name is "defined in the script but not yet
// created" at the moment it fails, so any validation error whose message names
// its own subject picked up the reorder hint — advising the author to move a
// statement before itself.
//
// A genuine forward reference (a name some LATER statement defines) must still
// be annotated.
func TestAnnotateForwardRef_SkipsOwnName(t *testing.T) {
	script := `create non-persistent entity Test.NpEntity ( "Name" : String(100) not null );
create microflow Test.Later () returns Boolean begin return true; end;`
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	allDefined := newScriptContext()
	for _, s := range prog.Statements {
		allDefined.collectSingle(s)
	}
	created := newScriptContext() // nothing created yet

	t.Run("own name is not a forward reference", func(t *testing.T) {
		err := errors.New("attribute 'Name' declares `not null` on non-persistent entity Test.NpEntity")
		got := annotateForwardRef(err, prog.Statements[0], created, allDefined)
		if strings.Contains(got.Error(), "defined later in this script") {
			t.Errorf("statement was annotated as referring forward to itself:\n%s", got.Error())
		}
	})

	t.Run("a genuine forward reference is still annotated", func(t *testing.T) {
		err := errors.New("microflow not found: Test.Later")
		got := annotateForwardRef(err, prog.Statements[0], created, allDefined)
		if !strings.Contains(got.Error(), "defined later in this script") {
			t.Errorf("expected a reorder hint for Test.Later, got:\n%s", got.Error())
		}
	})
}

// A workflow's own name is not a forward reference either. scriptContext.has
// checked every kind a script can create EXCEPT workflows — while allNames, the
// list the hint is drawn from, includes them — so every error naming the
// workflow being created ("workflow 'X' has reference errors") told the author
// to move the statement before itself. Found through the task page / targeting
// signature errors, which name their workflow on every refusal.
func TestAnnotateForwardRef_SkipsOwnWorkflowName(t *testing.T) {
	script := `create workflow Test.Earlier parameter $C: Test.Ctx begin call microflow Test.ACT; end workflow;
create workflow Test.Current parameter $C: Test.Ctx begin call microflow Test.ACT; end workflow;
create workflow Test.Later parameter $C: Test.Ctx begin call microflow Test.ACT; end workflow;`
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	allDefined := newScriptContext()
	for _, s := range prog.Statements {
		allDefined.collectSingle(s)
	}
	created := newScriptContext()
	created.collectSingle(prog.Statements[0]) // Test.Earlier already exists
	current := prog.Statements[1]

	t.Run("own workflow name is not a forward reference", func(t *testing.T) {
		err := errors.New("workflow 'Test.Current' has reference errors")
		if got := annotateForwardRef(err, current, created, allDefined); strings.Contains(got.Error(), "defined later in this script") {
			t.Errorf("workflow was annotated as referring forward to itself:\n%s", got.Error())
		}
	})

	t.Run("an earlier workflow is not a forward reference", func(t *testing.T) {
		err := errors.New("call workflow Test.Earlier: parameter is not mapped")
		if got := annotateForwardRef(err, current, created, allDefined); strings.Contains(got.Error(), "defined later in this script") {
			t.Errorf("an already-created workflow was annotated as a forward reference:\n%s", got.Error())
		}
	})

	t.Run("a genuine forward reference to a workflow is still annotated", func(t *testing.T) {
		err := errors.New("workflow not found: Test.Later")
		if got := annotateForwardRef(err, current, created, allDefined); !strings.Contains(got.Error(), "defined later in this script") {
			t.Errorf("expected a reorder hint for Test.Later, got:\n%s", got.Error())
		}
	})
}
