// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// `show page M.P($X = $X) with Title = 'New vendor'` writes FormSettings.
// TitleOverride — Studio Pro's "Override page title", the way one popup gets a
// per-mode title — but the read path never read it back. DESCRIBE therefore
// printed the statement without `with Title`, and any rewrite that went through
// the reader (a describe→exec round trip) silently dropped the override.
func TestActionFromGen_ShowPageKeepsTitleOverride(t *testing.T) {
	action := &microflows.ShowPageAction{
		PageName:       "Sales.Vendor_NewEdit",
		FormSettingsID: "fs-1",
		OverridePageTitle: &model.Text{
			BaseElement:  model.BaseElement{ID: "txt-1", TypeName: "Texts$Text"},
			Translations: map[string]string{"en_US": "New vendor"},
		},
	}
	raw, err := (&codec.Encoder{}).Encode(microflowActionToGen(action))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(bson.Raw(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := actionFromGen(el).(*microflows.ShowPageAction)
	if !ok {
		t.Fatalf("read back %T", actionFromGen(el))
	}
	if got.OverridePageTitle == nil || got.OverridePageTitle.Translations["en_US"] != "New vendor" {
		t.Errorf("OverridePageTitle = %+v, want en_US \"New vendor\"", got.OverridePageTitle)
	}

	// Control: no override reads back as none.
	action.OverridePageTitle = nil
	raw, _ = (&codec.Encoder{}).Encode(microflowActionToGen(action))
	el, _ = codec.NewDecoder(codec.DefaultRegistry).Decode(bson.Raw(raw))
	if got := actionFromGen(el).(*microflows.ShowPageAction); got.OverridePageTitle != nil {
		t.Errorf("no override stored, read back %+v", got.OverridePageTitle)
	}
}
