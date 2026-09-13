// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// Mendix holds end-of-path markers to the workflow's name-uniqueness rule. The
// first cut named every marker "EndOfParallelSplitPath" and the dedupe's type
// switch did not list the marker types, so a split with two paths built with
// CE0495 "Duplicate name 'EndOfParallelSplitPath'" — unit tests on the shape were
// green, and only mxbuild saw it.
func TestDeduplicateNamesEndOfPathMarkers(t *testing.T) {
	call := func(mf string) ast.WorkflowActivityNode {
		return &ast.WorkflowCallMicroflowNode{Microflow: ast.QualifiedName{Module: "M", Name: mf}}
	}
	acts := []workflows.WorkflowActivity{
		buildParallelSplit(&ast.WorkflowParallelSplitNode{Name: "SplitOne", Paths: []ast.WorkflowParallelPathNode{
			{PathNumber: 1, Activities: []ast.WorkflowActivityNode{call("ACT_A")}},
			{PathNumber: 2, Activities: []ast.WorkflowActivityNode{call("ACT_B")}},
			{PathNumber: 3},
		}}),
		buildParallelSplit(&ast.WorkflowParallelSplitNode{Name: "SplitTwo", Paths: []ast.WorkflowParallelPathNode{
			{PathNumber: 1, Activities: []ast.WorkflowActivityNode{call("ACT_C")}},
			{PathNumber: 2},
		}}),
	}
	deduplicateActivityNames(acts)

	seen := map[string]int{}
	markers := 0
	var walk func([]workflows.WorkflowActivity)
	walk = func(list []workflows.WorkflowActivity) {
		for _, a := range list {
			seen[a.GetName()]++
			if _, ok := a.(*workflows.EndOfParallelSplitPathActivity); ok {
				markers++
			}
			for _, f := range nestedFlows(a) {
				walk(f.Activities)
			}
		}
	}
	walk(acts)

	if markers != 5 {
		t.Fatalf("markers = %d, want one per path (5)", markers)
	}
	for name, n := range seen {
		if n > 1 {
			t.Errorf("name %q used %d times — mxbuild refuses this as CE0495", name, n)
		}
	}
}
