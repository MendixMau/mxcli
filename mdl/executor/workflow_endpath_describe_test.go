// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// Every parallel split path now ends with Mendix's end-of-path marker. DESCRIBE
// must omit it — it is implicit in MDL — or every split would print a line that
// either is not valid MDL or would add a second marker on re-execution.
func TestDescribeOmitsEndOfPathMarkers(t *testing.T) {
	call := &workflows.CallMicroflowTask{Microflow: "M.ACT_A"}
	call.Name, call.Caption = "ACT_A", "ACT_A"
	flow := workflows.EndParallelSplitPath(&workflows.Flow{Activities: []workflows.WorkflowActivity{call}}, newWorkflowID)
	flow.Activities = append(flow.Activities, &workflows.EndOfBoundaryEventPathActivity{})

	out := strings.Join(formatWorkflowActivities(flow, "  "), "\n")
	for _, leak := range []string{"EndOfParallelSplitPath", "EndOfBoundaryEventPath", "End of parallel split path", "[Workflows$"} {
		if strings.Contains(out, leak) {
			t.Errorf("describe output leaks %q:\n%s", leak, out)
		}
	}
	if !strings.Contains(out, "M.ACT_A") {
		t.Errorf("describe dropped the real activity:\n%s", out)
	}
}
