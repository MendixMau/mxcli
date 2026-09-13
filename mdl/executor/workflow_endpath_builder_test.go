// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// PARALLEL SPLIT paths built from MDL were stored without Mendix's end-of-path
// marker. `mxcli check`, `describe` round-trip and mxbuild were all clean, and at
// runtime each path ran from the split straight to a synthesised end — the
// activities inside never executed and left no activity record. Measured on
// Mendix 11.14.0, marker as the only variable: without it neither path's
// call-microflow ran; with it both finished (ako/view-entity-examples §6).
func TestBuildParallelSplitEndsEveryPath(t *testing.T) {
	split := buildParallelSplit(&ast.WorkflowParallelSplitNode{
		Name: "probeSplit",
		Paths: []ast.WorkflowParallelPathNode{
			{PathNumber: 1, Activities: []ast.WorkflowActivityNode{&ast.WorkflowCallMicroflowNode{Microflow: ast.QualifiedName{Module: "M", Name: "ACT_A"}}}},
			{PathNumber: 2, Activities: []ast.WorkflowActivityNode{&ast.WorkflowCallMicroflowNode{Microflow: ast.QualifiedName{Module: "M", Name: "ACT_B"}}}},
			{PathNumber: 3}, // `path 3 { }`
		},
	})
	if len(split.Outcomes) != 3 {
		t.Fatalf("paths = %d, want 3", len(split.Outcomes))
	}
	for i, oc := range split.Outcomes {
		if oc.Flow == nil || len(oc.Flow.Activities) == 0 {
			t.Fatalf("path %d has no flow; an empty path is stored as a flow holding only the marker", i+1)
		}
		last := oc.Flow.Activities[len(oc.Flow.Activities)-1]
		if _, ok := last.(*workflows.EndOfParallelSplitPathActivity); !ok {
			t.Errorf("path %d ends with %T, want *workflows.EndOfParallelSplitPathActivity — without it the runtime skips the path", i+1, last)
		}
	}
	// The real activities are still there, ahead of the marker.
	if n := len(split.Outcomes[0].Flow.Activities); n != 2 {
		t.Errorf("path 1 activities = %d, want the call followed by the marker", n)
	}
}
