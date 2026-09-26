// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Pinned against ConversationalUI Snippet_Agent_Details (Studio Pro-authored,
// 11.14.0): Forms$ConfirmationInfo with Question / ProceedButtonCaption /
// CancelButtonCaption, each a Texts$Text, inside the MicroflowSettings.
func TestWriteMicroflowAction_ConfirmationInfo(t *testing.T) {
	d := encodeWidget(t, &pages.ActionButton{
		BaseWidget: pages.BaseWidget{BaseElement: model.BaseElement{ID: "b"}, Name: "btnDelete"},
		Action: &pages.MicroflowClientAction{
			MicroflowName: "M.ACT_Delete",
			Confirmation:  &pages.ConfirmationInfo{Question: "Delete?", ProceedCaption: "Delete", CancelCaption: "Cancel"},
		},
	})
	settings := bsonnav.DGetDoc(bsonnav.DGetDoc(d, "Action"), "MicroflowSettings")
	ci := bsonnav.DGetDoc(settings, "ConfirmationInfo")
	if ci == nil {
		t.Fatal("ConfirmationInfo not written")
	}
	for _, k := range []string{"Question", "ProceedButtonCaption", "CancelButtonCaption"} {
		if bsonnav.DGetDoc(ci, k) == nil {
			t.Errorf("%s missing", k)
		}
	}
}

// Control: no confirmation keeps the null Studio Pro stores.
func TestWriteMicroflowAction_NoConfirmationIsNull(t *testing.T) {
	d := encodeWidget(t, &pages.ActionButton{
		BaseWidget: pages.BaseWidget{BaseElement: model.BaseElement{ID: "b"}, Name: "btn"},
		Action:     &pages.MicroflowClientAction{MicroflowName: "M.X"},
	})
	settings := bsonnav.DGetDoc(bsonnav.DGetDoc(d, "Action"), "MicroflowSettings")
	if v := bsonnav.DGet(settings, "ConfirmationInfo"); v != nil {
		t.Errorf("ConfirmationInfo = %#v, want null", v)
	}
}
