// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// layoutCtx is a project whose only layouts are Atlas_Core.Atlas_Default and
// Atlas_Core.PopupLayout, and whose only module is Atlas_Core.
func layoutCtx(t *testing.T) *ExecContext {
	t.Helper()
	atlas := &model.Module{BaseElement: model.BaseElement{ID: model.ID("mod-atlas")}, Name: "Atlas_Core"}
	mk := func(id, name string) *pages.Layout {
		l := &pages.Layout{Name: name}
		l.ID = model.ID(id)
		l.ContainerID = atlas.ID
		return l
	}
	lays := []*pages.Layout{mk("l1", "Atlas_Default"), mk("l2", "PopupLayout")}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{atlas}, nil },
		ListLayoutsFunc: func() ([]*pages.Layout, error) { return lays, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return nil, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(atlas)))
	return ctx
}

// marketplace-rnd: `Layout: Atlas_Core.Popup_Content` passed `check
// --references` and exec then refused the page ("references layout …, which
// was not found"). The layout is now resolved at check time, the same way
// pageBuilder.resolveLayout does it.
func TestValidatePageLayout(t *testing.T) {
	cases := []struct {
		name    string
		script  string
		wantErr bool
	}{
		{"missing layout", `create module M;
create page M.P (title: 'P', layout: Atlas_Core.Popup_Content) { dynamictext dt (content: 'x') }`, true},
		{"missing layout, no widgets", `create module M;
create page M.P (title: 'P', layout: Atlas_Core.Popup_Content) { }`, true},
		{"existing layout", `create module M;
create page M.P (title: 'P', layout: Atlas_Core.PopupLayout) { dynamictext dt (content: 'x') }`, false},
		{"quoted existing layout", `create module M;
create page M.P (title: 'P', layout: 'Atlas_Core.Atlas_Default') { dynamictext dt (content: 'x') }`, false},
		{"layout in the wrong module", `create module M;
create page M.P (title: 'P', layout: M.Atlas_Default) { dynamictext dt (content: 'x') }`, true},
		{"layout created by the script", `create module M;
create layout M.App_Mine (layouttype: 'Responsive') { scrollcontainer layoutContainer { region center { placeholder Main } } };
create page M.P (title: 'P', layout: M.App_Mine) { dynamictext dt (content: 'x') }`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, errs := visitor.Build(c.script)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			var layoutErr error
			for _, err := range validateProgram(layoutCtx(t), prog) {
				if strings.Contains(err.Error(), "references layout") {
					layoutErr = err
				}
			}
			if (layoutErr != nil) != c.wantErr {
				t.Errorf("layout error = %v, want error: %v", layoutErr, c.wantErr)
			}
		})
	}
}

// A listing that fails or comes back empty is not evidence of absence.
func TestValidatePageLayout_NoListingIsNotAnError(t *testing.T) {
	if !layoutResolves("Atlas_Core.Atlas_Default", map[string]bool{"Atlas_Core.Atlas_Default": true}, nil) {
		t.Error("qualified match failed")
	}
	if !layoutResolves("Atlas_Default", map[string]bool{"Atlas_Core.Atlas_Default": true}, nil) {
		t.Error("bare name should match in any module, as resolveLayout does")
	}
	prog, _ := visitor.Build(`create page M.P (title: 'P', layout: Nope.Nope) { dynamictext dt (content: 'x') }`)
	for _, err := range validateProgram(forwardRefCtx(t), prog) {
		if strings.Contains(err.Error(), "references layout") {
			t.Errorf("empty layout listing produced a layout error: %v", err)
		}
	}
}
