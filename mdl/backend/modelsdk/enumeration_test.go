// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
)

// TestReadSlice_Enumerations checks the enum adapter: values are converted (for
// the Values count) and captions decode via textElementToModel. SHOW
// ENUMERATIONS is cross-checked byte-for-byte against legacy in the plan.
//
// The count is of STORED enumerations. ListEnumerations also returns the System
// module's synthesized ones (#1102), which have no stored unit and no caption to
// decode, so counting the whole listing here would stop testing the adapter and
// start tracking the size of a hardcoded table.
func TestReadSlice_Enumerations(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	all, err := b.ListEnumerations()
	if err != nil {
		t.Fatalf("ListEnumerations: %v", err)
	}
	var enums []*model.Enumeration
	for _, e := range all {
		if string(e.ContainerID) != meta.SystemModuleID {
			enums = append(enums, e)
		}
	}
	if len(enums) != 7 {
		t.Fatalf("stored enumeration count = %d, want 7", len(enums))
	}
	for _, e := range enums {
		if e.Name == "Filter_Operators" {
			if len(e.Values) != 12 {
				t.Errorf("Filter_Operators value count = %d, want 12", len(e.Values))
			}
			return
		}
	}
	t.Error("Filter_Operators enumeration not found")
}
