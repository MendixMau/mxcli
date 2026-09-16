// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#490 — `ReadOnlyStyle:` parsed, passed check, was emitted by DESCRIBE,
// and was never written: the codec hardcoded "Inherit" on every check box.
//
// Not cosmetic. With "Inherit" a read-only check box in a DataGrid2 cell renders
// as the text "Yes"/"No"; with "Control" it renders the (disabled) checkbox
// glyph — measured on 11.6.6 by patching the stored string by hand.
func TestCheckBoxReadOnlyStyle_Written(t *testing.T) {
	tests := []struct {
		name  string
		style string
		want  string
	}{
		// An omitted property keeps the stored default, so scripts that never
		// mention it produce the same document as before.
		{"unset keeps the default", "", "Inherit"},
		{"control", "Control", "Control"},
		{"text", "Text", "Text"},
		{"inherit", "Inherit", "Inherit"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cb := &pages.CheckBox{ReadOnlyStyle: tc.style}
			cb.Name = "cbActive"
			doc := encodeWidget(t, cb)
			if got := docGet(doc, "ReadOnlyStyle"); got != tc.want {
				t.Errorf("ReadOnlyStyle = %v, want %q — the authored value never reached "+
					"the document (ako/mxcli#490)", got, tc.want)
			}
		})
	}
}
