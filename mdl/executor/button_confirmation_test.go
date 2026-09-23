// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// confirmTextMap builds a Texts$Text raw map with the given translations, in
// the marker-3 Items shape Studio Pro stores.
func confirmTextMap(pairs ...string) map[string]any {
	items := []any{int32(3)}
	for i := 0; i+1 < len(pairs); i += 2 {
		items = append(items, map[string]any{
			"$Type": "Texts$Translation", "LanguageCode": pairs[i], "Text": pairs[i+1],
		})
	}
	return map[string]any{"$Type": "Texts$Text", "Items": items}
}

func confirmInfoMap(question, proceed, cancel map[string]any) map[string]any {
	return map[string]any{
		"$Type":                "Forms$ConfirmationInfo",
		"Question":             question,
		"ProceedButtonCaption": proceed,
		"CancelButtonCaption":  cancel,
	}
}

// Shape measured on AppStore.Framework_Overview: the dialog sits under
// MicroflowSettings, with default captions stored explicitly and several
// languages. The defaults are omitted on describe (the writer puts them back),
// which is what keeps describe → exec → describe stable.
func TestExtractButtonConfirmation_MicroflowStudioProShape(t *testing.T) {
	ctx := (&Executor{}).newExecContext(context.Background())
	w := map[string]any{
		"$Type": "Forms$ActionButton",
		"Action": map[string]any{
			"$Type": "Forms$MicroflowAction",
			"MicroflowSettings": map[string]any{
				"$Type":     "Forms$MicroflowSettings",
				"Microflow": "AppStore.RetireFrameworkMajorSeries",
				"ConfirmationInfo": confirmInfoMap(
					confirmTextMap("en_US", "Please confirm.", "nl_NL", "Bevestig."),
					confirmTextMap("en_US", "Proceed", "nl_NL", "Doorgaan"),
					confirmTextMap("en_US", "Cancel", "nl_NL", "Annuleren"),
				),
			},
		},
	}
	q, p, c := extractButtonConfirmation(ctx, w)
	if q != "Please confirm." {
		t.Errorf("question = %q", q)
	}
	if p != "" || c != "" {
		t.Errorf("default captions described as (%q, %q), want omitted", p, c)
	}
}

func TestExtractButtonConfirmation_NanoflowCustomCaptions(t *testing.T) {
	ctx := (&Executor{}).newExecContext(context.Background())
	w := map[string]any{
		"$Type": "Forms$ActionButton",
		"Action": map[string]any{
			"$Type": "Forms$CallNanoflowClientAction",
			"ConfirmationInfo": confirmInfoMap(
				confirmTextMap("en_US", "Go on?"),
				confirmTextMap("en_US", "Yes"),
				confirmTextMap("en_US", "No"),
			),
		},
	}
	q, p, c := extractButtonConfirmation(ctx, w)
	if q != "Go on?" || p != "Yes" || c != "No" {
		t.Errorf("got (%q, %q, %q), want (Go on?, Yes, No)", q, p, c)
	}
	props := appendConfirmationProps(nil, rawWidget{Confirmation: q, ConfirmProceed: p, ConfirmCancel: c})
	want := "Confirmation: 'Go on?', ConfirmProceed: 'Yes', ConfirmCancel: 'No'"
	if got := strings.Join(props, ", "); got != want {
		t.Errorf("props = %s, want %s", got, want)
	}
}

// No dialog (the null slot every mxcli-written and most Studio Pro actions
// carry) and a dialog with an empty question both describe as nothing.
func TestExtractButtonConfirmation_AbsentOrEmpty(t *testing.T) {
	ctx := (&Executor{}).newExecContext(context.Background())
	for name, info := range map[string]any{
		"null":           nil,
		"empty question": confirmInfoMap(confirmTextMap("en_US", ""), confirmTextMap("en_US", "Proceed"), confirmTextMap("en_US", "Cancel")),
	} {
		w := map[string]any{"Action": map[string]any{
			"$Type":             "Forms$MicroflowAction",
			"MicroflowSettings": map[string]any{"ConfirmationInfo": info},
		}}
		if q, p, c := extractButtonConfirmation(ctx, w); q != "" || p != "" || c != "" {
			t.Errorf("%s: got (%q, %q, %q), want nothing", name, q, p, c)
		}
	}
}

func buttonWith(action *ast.ActionV3, props map[string]any) *ast.WidgetV3 {
	w := &ast.WidgetV3{Type: "actionbutton", Name: "btn", Properties: map[string]any{}}
	for k, v := range props {
		w.Properties[k] = v
	}
	if action != nil {
		w.Properties["Action"] = action
	}
	return w
}

func TestBuildButton_ConfirmationOnMicroflowAction(t *testing.T) {
	pb := &pageBuilder{}
	w := buttonWith(&ast.ActionV3{Type: "microflow", Target: "M.ACT_Delete"},
		map[string]any{"Caption": "Delete", "Confirmation": "Are you sure?", "ConfirmProceed": "Yes"})
	act := &pages.MicroflowClientAction{MicroflowName: "M.ACT_Delete"}
	if err := pb.applyConfirmationV3(w, act); err != nil {
		t.Fatalf("applyConfirmationV3: %v", err)
	}
	if act.Confirmation == nil {
		t.Fatal("Confirmation was dropped")
	}
	lang := pb.textLang()
	for name, pair := range map[string][2]string{
		"question": {act.Confirmation.Question.Translations[lang], "Are you sure?"},
		"proceed":  {act.Confirmation.ProceedCaption.Translations[lang], "Yes"},
		"cancel":   {act.Confirmation.CancelCaption.Translations[lang], "Cancel"},
	} {
		if pair[0] != pair[1] {
			t.Errorf("%s = %q, want %q", name, pair[0], pair[1])
		}
	}
}

// Every way the properties could be silently dropped is refused, by the same
// function `mxcli check` (MDL-WIDGET32) and the builder call.
func TestConfirmationPropsProblem(t *testing.T) {
	mf := &ast.ActionV3{Type: "microflow", Target: "M.X"}
	nf := &ast.ActionV3{Type: "nanoflow", Target: "M.N"}
	cases := []struct {
		name    string
		w       *ast.WidgetV3
		refused bool
	}{
		{"no confirmation", buttonWith(mf, nil), false},
		{"microflow", buttonWith(mf, map[string]any{"Confirmation": "Q?"}), false},
		{"nanoflow", buttonWith(nf, map[string]any{"Confirmation": "Q?", "ConfirmCancel": "No"}), false},
		{"lowercase key", buttonWith(mf, map[string]any{"confirmation": "Q?"}), false},
		{"save_changes action", buttonWith(&ast.ActionV3{Type: "save"}, map[string]any{"Confirmation": "Q?"}), true},
		{"no action", buttonWith(nil, map[string]any{"Confirmation": "Q?"}), true},
		{"caption without question", buttonWith(mf, map[string]any{"ConfirmProceed": "Yes"}), true},
		{"not a button", &ast.WidgetV3{Type: "textbox", Name: "t", Properties: map[string]any{"Confirmation": "Q?"}}, true},
	}
	for _, tc := range cases {
		got := confirmationPropsProblem(tc.w)
		if (got != "") != tc.refused {
			t.Errorf("%s: problem = %q, refused want %v", tc.name, got, tc.refused)
		}
	}
	// The builder refuses too — a check skipped is not a write that drops it.
	pb := &pageBuilder{}
	if err := pb.applyConfirmationV3(buttonWith(&ast.ActionV3{Type: "save"}, map[string]any{"Confirmation": "Q?"}),
		&pages.SaveChangesClientAction{}); err == nil {
		t.Error("builder accepted Confirmation on a save_changes action")
	}
}

// The describe vocabulary must not warn about itself (MDL-WIDGET07).
func TestConfirmationPropsAreKnown(t *testing.T) {
	for _, k := range []string{"Confirmation", "ConfirmProceed", "ConfirmCancel"} {
		if !isKnownStaticWidgetProp(k) {
			t.Errorf("%s is not a known widget property — describe output would warn on re-exec", k)
		}
	}
}

// ALTER PAGE applies properties in sorted order, and byte order puts
// ConfirmCancel/ConfirmProceed before Confirmation — which would edit a dialog
// the question has not created yet. Action must precede all three.
func TestConfirmationSetRankOrdersQuestionFirst(t *testing.T) {
	names := []string{"ConfirmProceed", "Confirmation", "Caption", "ConfirmCancel", "Action"}
	orderSetProperties(names)
	want := "Action,Caption,Confirmation,ConfirmProceed,ConfirmCancel"
	if got := strings.Join(names, ","); got != want {
		t.Errorf("order = %s, want %s", got, want)
	}
}
