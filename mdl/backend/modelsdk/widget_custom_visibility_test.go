// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A pluggable widget's `Visible: [expr]` must reach disk as a
// ConditionalVisibilitySettings node, exactly as a built-in widget's does.
// Studio Pro stores it on CustomWidgets$CustomWidget (the PDS dropdown menu on
// a real Marketplace page carries one), and describe emits it, so a
// describe -> replace round trip that loses it silently makes the widget
// visible to everyone.
func TestCustomWidgetConditionalVisibility_Serialized(t *testing.T) {
	cw := &pages.CustomWidget{}
	cw.Name = "menu"
	cw.ConditionalVisibility = &pages.ConditionalVisibilitySettings{
		BaseElement: model.BaseElement{TypeName: "Forms$ConditionalVisibilitySettings"},
		Expression:  "$currentObject/IsAdmin",
	}
	doc := encodeWidget(t, cw)
	cvs, ok := docGet(doc, "ConditionalVisibilitySettings").(bsonv1.D)
	if !ok {
		t.Fatalf("ConditionalVisibilitySettings not serialized on a CustomWidget (got %T)",
			docGet(doc, "ConditionalVisibilitySettings"))
	}
	if got := docGet(cvs, "Expression"); got != "$currentObject/IsAdmin" {
		t.Errorf("Expression = %v, want $currentObject/IsAdmin", got)
	}
}
