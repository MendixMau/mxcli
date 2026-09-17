// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#1113: "DESCRIBE ENUMERATION returns empty captions for values
// that have captions defined in Studio Pro" — every value came back as
// `MyValue ”`, whatever Studio Pro showed.
//
// The read side asked the stored Texts$Text for a hardcoded "en_US", so a
// project whose default language is anything else lost every caption. Mendix has
// no language-neutral text (#970): a Dutch project's captions carry
// LanguageCode "nl_NL" and nothing else. Measured end to end before the fix, on
// an nl_NL copy of testdata/expr-checker, with the captions plainly present in
// the stored unit (`Texts$Translation LanguageCode nl_NL Text Nieuw`):
//
//	create or modify enumeration MyFirstModule.OrderStatus (
//	  Fresh '',
//	  Shipped '',
//	  Cancelled ''
//	);
//
// which is the reported symptom, and is also destructive: re-executing that
// DESCRIBE output replaces the real captions with empty strings.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// enumWithCaptions builds a one-value enumeration whose caption carries the
// given translations.
func enumWithCaptions(modID model.ID, translations map[string]string) *model.Enumeration {
	return &model.Enumeration{
		BaseElement: model.BaseElement{ID: nextID("enum")},
		ContainerID: modID,
		Name:        "OrderStatus",
		Values: []model.EnumerationValue{{
			BaseElement: model.BaseElement{ID: nextID("ev")},
			Name:        "Fresh",
			Caption:     &model.Text{Translations: translations},
		}},
	}
}

// describeEnumerationWithLanguage runs DESCRIBE ENUMERATION against a project
// whose DefaultLanguageCode is defaultLang and returns the output.
func describeEnumerationWithLanguage(t *testing.T, defaultLang string, translations map[string]string) string {
	t.Helper()
	mod := mkModule("MyFirstModule")
	enum := enumWithCaptions(mod.ID, translations)

	h := mkHierarchy(mod)
	withContainer(h, enum.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{enum}, nil },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) {
			if defaultLang == "" {
				return nil, nil
			}
			return &model.ProjectSettings{
				Language: &model.LanguageSettings{DefaultLanguageCode: defaultLang},
			}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeEnumeration(ctx, ast.QualifiedName{Module: "MyFirstModule", Name: "OrderStatus"}))
	return buf.String()
}

func TestDescribeEnumeration_CaptionFollowsProjectLanguage_Issue1113(t *testing.T) {
	tests := []struct {
		name         string
		defaultLang  string
		translations map[string]string
		want         string
	}{{
		// The reported case: a single-language project that is not en_US.
		name:         "nl_NL-only project",
		defaultLang:  "nl_NL",
		translations: map[string]string{"nl_NL": "Nieuw"},
		want:         "Nieuw",
	}, {
		// DESCRIBE must read back the language CREATE writes, or describe -> exec
		// stops round-tripping: it would rewrite the Dutch caption as English.
		name:         "multi-language project prefers its default",
		defaultLang:  "nl_NL",
		translations: map[string]string{"en_US": "New", "nl_NL": "Nieuw"},
		want:         "Nieuw",
	}, {
		name:         "en_US project is unaffected",
		defaultLang:  "en_US",
		translations: map[string]string{"en_US": "New"},
		want:         "New",
	}, {
		// A caption Studio Pro shows is never reported as empty: with no
		// translation in either the project language or en_US, any stored one
		// beats nothing.
		name:         "falls back to a stored translation",
		defaultLang:  "nl_NL",
		translations: map[string]string{"de_DE": "Neu"},
		want:         "Neu",
	}, {
		name:         "settings unavailable falls back to en_US",
		defaultLang:  "",
		translations: map[string]string{"en_US": "New"},
		want:         "New",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := describeEnumerationWithLanguage(t, tc.defaultLang, tc.translations)
			if strings.Contains(out, "Fresh ''") {
				t.Errorf("caption came back empty (issue #1113):\n%s", out)
			}
			if !strings.Contains(out, "Fresh '"+tc.want+"'") {
				t.Errorf("want caption %q, got:\n%s", tc.want, out)
			}
		})
	}
}

// The last-resort fallback iterates a map, so it has to sort: a caption that
// changes between runs makes DESCRIBE output undiffable and the round-trip
// non-deterministic.
func TestPickTextTranslation_FallbackIsDeterministic(t *testing.T) {
	txt := &model.Text{Translations: map[string]string{
		"nl_NL": "Nieuw", "de_DE": "Neu", "fr_FR": "Nouveau", "es_ES": "Nuevo",
	}}
	first := pickTextTranslation(txt, "pt_BR")
	for i := 0; i < 50; i++ {
		if got := pickTextTranslation(txt, "pt_BR"); got != first {
			t.Fatalf("fallback is not deterministic: %q then %q", first, got)
		}
	}
	if first != "Neu" {
		t.Errorf("fallback = %q, want the lowest language code's text %q", first, "Neu")
	}
}
