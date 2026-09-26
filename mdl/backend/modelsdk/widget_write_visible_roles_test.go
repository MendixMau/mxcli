// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Pinned against WorkflowCommons.WorkflowAttachment_New actionButton3 (Studio
// Pro-authored, 11.14.0): Expression "", Attribute "", Conditions [2],
// IgnoreSecurity false, ModuleRoles [1, "WorkflowCommons.Administrator"].
func TestWriteActionButton_VisibleForModuleRoles(t *testing.T) {
	d := encodeWidget(t, &pages.ActionButton{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{ID: "b"}, Name: "btnDelete",
			ConditionalVisibility: &pages.ConditionalVisibilitySettings{
				ModuleRoles: []model.ID{"Procurement.Admin"},
			},
		},
	})
	cv := bsonnav.DGetDoc(d, "ConditionalVisibilitySettings")
	if cv == nil {
		t.Fatal("no ConditionalVisibilitySettings written")
	}
	var got []string
	for _, e := range bsonnav.DGetArrayElements(bsonnav.DGet(cv, "ModuleRoles")) {
		if s, ok := e.(string); ok {
			got = append(got, s)
		}
	}
	if len(got) != 1 || got[0] != "Procurement.Admin" {
		t.Errorf("ModuleRoles = %v, want [Procurement.Admin]", got)
	}
}
