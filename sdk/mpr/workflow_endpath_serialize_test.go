// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/workflows"
	"go.mongodb.org/mongo-driver/bson"
)

// The legacy serializer's activity switch returned nil for both end-of-path
// markers, so a path built with one lost it on write — the runtime then skips
// the path's contents (ako/view-entity-examples §6).
func TestSerializeEndOfPathMarkers(t *testing.T) {
	for want, act := range map[string]workflows.WorkflowActivity{
		"Workflows$EndOfParallelSplitPathActivity": &workflows.EndOfParallelSplitPathActivity{},
		"Workflows$EndOfBoundaryEventPathActivity": &workflows.EndOfBoundaryEventPathActivity{},
	} {
		doc := serializeWorkflowActivity(act)
		if doc == nil {
			t.Errorf("%s serialized to nil — the marker would be dropped", want)
			continue
		}
		got := ""
		for _, e := range doc {
			if e.Key == "$Type" {
				got, _ = e.Value.(string)
			}
		}
		if got != want {
			t.Errorf("$Type = %q, want %q", got, want)
		}
	}
}

var _ = bson.D{}
