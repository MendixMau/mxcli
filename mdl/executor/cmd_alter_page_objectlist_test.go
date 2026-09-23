// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ALTER PAGE on a PDS dropdown menu's `dropdownitem` entries. The def mirrors
// the real pdsdropdownmenu.def.json a Marketplace-derived project generates
// (list key dropdownItems, container DROPDOWNITEM); it is built in memory
// because no embedded definition declares an object list.

const pdsDropdownID = "mendix.pdsdropdownmenu.PDSDropdownMenu"

func pdsDropdownRegistry() *WidgetRegistry {
	def := &WidgetDefinition{
		WidgetID: pdsDropdownID,
		MDLName:  "PDSDROPDOWNMENU",
		ObjectLists: []ObjectListMapping{
			{PropertyKey: "dropdownItems", MDLContainer: "DROPDOWNITEM"},
		},
	}
	return &WidgetRegistry{
		byMDLName:  map[string]*WidgetDefinition{"PDSDROPDOWNMENU": def},
		byWidgetID: map[string]*WidgetDefinition{def.WidgetID: def},
		// A project path, as LoadWidgetRegistry sets it: without one the kind
		// check stays silent, since an embedded-only registry knows no real widget.
		projectPath: "/nonexistent/App.mpr",
	}
}

// objectListRecorder is a page with one dropdown menu `menu1` holding four
// entries, dropdownitem1..4. It records what the executor asked of it.
type objectListRecorder struct {
	inserts []string
	drops   []string
	widgets [][]backend.WidgetRef
}

func (r *objectListRecorder) mutator() *mock.MockPageMutator {
	return &mock.MockPageMutator{
		PluggableWidgetIDFunc: func(ref string) string {
			if ref == "menu1" {
				return pdsDropdownID
			}
			return ""
		},
		ResolveObjectListItemFunc: func(ref, item string) (backend.ObjectListItemRef, bool, error) {
			if item != "" || !strings.HasPrefix(ref, "dropdownitem") {
				return backend.ObjectListItemRef{}, false, nil
			}
			var n int
			if _, err := fmt.Sscanf(ref, "dropdownitem%d", &n); err != nil || n < 1 || n > 4 {
				return backend.ObjectListItemRef{}, false, nil
			}
			return backend.ObjectListItemRef{Owner: "menu1", WidgetID: pdsDropdownID,
				ListKey: "dropdownItems", Keyword: "dropdownitem", Index: n - 1}, true, nil
		},
		InsertObjectListItemsFunc: func(owner, key string, index int, donor pages.Widget) error {
			r.inserts = append(r.inserts, fmt.Sprintf("%s.%s@%d", owner, key, index))
			return nil
		},
		DropObjectListItemsFunc: func(owner, key string, idx []int) error {
			r.drops = append(r.drops, fmt.Sprintf("%s.%s%v", owner, key, idx))
			return nil
		},
		DropWidgetFunc: func(refs []backend.WidgetRef) error {
			r.widgets = append(r.widgets, refs)
			return nil
		},
		InsertWidgetFunc: func(string, string, backend.InsertPosition, []pages.Widget) error {
			return fmt.Errorf("an object-list entry reached the widget insert path")
		},
		ReplaceWidgetFunc: func(string, string, []pages.Widget) error {
			return fmt.Errorf("an object-list entry reached the widget replace path")
		},
		SaveFunc: func() error { return nil },
	}
}

func runObjectListAlter(t *testing.T, rec *objectListRecorder, ops ...ast.AlterPageOperation) error {
	t.Helper()
	orig := buildObjectListDonor
	buildObjectListDonor = func(_ *ExecContext, _ backend.PageMutator, _, widgetID string,
		_ *ObjectListMapping, _ []*ast.WidgetV3, _ string, _ model.ID) (pages.Widget, error) {
		return &pages.CustomWidget{}, nil
	}
	t.Cleanup(func() { buildObjectListDonor = orig })

	mod := mkModule("UserGroups")
	pg := mkPage(mod.ID, "GroupDetails")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(model.ID) (backend.PageMutator, error) {
			return rec.mutator(), nil
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, pg.ContainerID, mod.ID)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	ctx.widgetRegistry = pdsDropdownRegistry()
	ctx.widgetRegistryLoaded = true
	return execAlterPage(ctx, &ast.AlterPageStmt{
		PageName:   ast.QualifiedName{Module: "UserGroups", Name: "GroupDetails"},
		Operations: ops,
	})
}

func dropdownItem(name string) *ast.WidgetV3 {
	// TypeIsGeneric as the parser sets it: `dropdownitem` is no grammar token.
	return &ast.WidgetV3{Type: "dropdownitem", Name: name, TypeIsGeneric: true,
		Properties: map[string]any{"itemType": "button", "caption": "New Item"}}
}

func TestAlterPage_InsertAfterDropdownItem(t *testing.T) {
	rec := &objectListRecorder{}
	assertNoError(t, runObjectListAlter(t, rec, &ast.InsertWidgetOp{
		Position: "AFTER", Target: ast.WidgetRef{Widget: "dropdownitem1"},
		Widgets: []*ast.WidgetV3{dropdownItem("n")},
	}))
	if want := []string{"menu1.dropdownItems@1"}; !reflect.DeepEqual(rec.inserts, want) {
		t.Errorf("inserts = %v, want %v", rec.inserts, want)
	}
}

func TestAlterPage_InsertBeforeDropdownItem(t *testing.T) {
	rec := &objectListRecorder{}
	assertNoError(t, runObjectListAlter(t, rec, &ast.InsertWidgetOp{
		Position: "BEFORE", Target: ast.WidgetRef{Widget: "dropdownitem3"},
		Widgets: []*ast.WidgetV3{dropdownItem("n")},
	}))
	if want := []string{"menu1.dropdownItems@2"}; !reflect.DeepEqual(rec.inserts, want) {
		t.Errorf("inserts = %v, want %v", rec.inserts, want)
	}
}

func TestAlterPage_InsertIntoDropdownMenuAppends(t *testing.T) {
	rec := &objectListRecorder{}
	assertNoError(t, runObjectListAlter(t, rec, &ast.InsertWidgetOp{
		Position: "INTO", Target: ast.WidgetRef{Widget: "menu1"},
		Widgets: []*ast.WidgetV3{dropdownItem("n")},
	}))
	if want := []string{"menu1.dropdownItems@-1"}; !reflect.DeepEqual(rec.inserts, want) {
		t.Errorf("inserts = %v, want %v", rec.inserts, want)
	}
}

func TestAlterPage_InsertIntoDropdownMenuRefusesMixedBody(t *testing.T) {
	rec := &objectListRecorder{}
	err := runObjectListAlter(t, rec, &ast.InsertWidgetOp{
		Position: "INTO", Target: ast.WidgetRef{Widget: "menu1"},
		Widgets: []*ast.WidgetV3{dropdownItem("n"), {Type: "dynamictext", Name: "t"}},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "mixes `dropdownitem` entries with other widgets")
	if len(rec.inserts) != 0 {
		t.Errorf("a refused insert still wrote: %v", rec.inserts)
	}
}

func TestAlterPage_InsertBesideDropdownItemRefusesAWidget(t *testing.T) {
	rec := &objectListRecorder{}
	err := runObjectListAlter(t, rec, &ast.InsertWidgetOp{
		Position: "AFTER", Target: ast.WidgetRef{Widget: "dropdownitem1"},
		Widgets: []*ast.WidgetV3{{Type: "dynamictext", Name: "t"}},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "only `dropdownitem` entries can go beside it")
}

// Entry names are positions: dropping 2 renumbers 4 to 3. Both must be
// resolved against the page as it was, and removed in one call per list.
func TestAlterPage_DropDropdownItemsResolvesBeforeRemoving(t *testing.T) {
	rec := &objectListRecorder{}
	assertNoError(t, runObjectListAlter(t, rec, &ast.DropWidgetOp{
		Targets: []ast.WidgetRef{{Widget: "dropdownitem4"}, {Widget: "someText"}, {Widget: "dropdownitem2"}},
	}))
	if want := []string{"menu1.dropdownItems[1 3]"}; !reflect.DeepEqual(rec.drops, want) {
		t.Errorf("drops = %v, want %v", rec.drops, want)
	}
	if want := [][]backend.WidgetRef{{{Widget: "someText"}}}; !reflect.DeepEqual(rec.widgets, want) {
		t.Errorf("widget drops = %v, want %v", rec.widgets, want)
	}
}

func TestAlterPage_ReplaceDropdownItemInsertsThenDrops(t *testing.T) {
	rec := &objectListRecorder{}
	assertNoError(t, runObjectListAlter(t, rec, &ast.ReplaceWidgetOp{
		Target:     ast.WidgetRef{Widget: "dropdownitem3"},
		NewWidgets: []*ast.WidgetV3{dropdownItem("r")},
	}))
	if want := []string{"menu1.dropdownItems@3"}; !reflect.DeepEqual(rec.inserts, want) {
		t.Errorf("inserts = %v, want %v", rec.inserts, want)
	}
	if want := []string{"menu1.dropdownItems[2]"}; !reflect.DeepEqual(rec.drops, want) {
		t.Errorf("drops = %v, want %v", rec.drops, want)
	}
}

// The reported MDL-WIDGET25: an ALTER body's top level has no enclosing widget
// in the script, so `dropdownitem` was looked up as a widget and not found.
func TestValidateAlterPage_TopLevelDropdownItemAccepted(t *testing.T) {
	stmt := &ast.AlterPageStmt{
		PageName: ast.QualifiedName{Module: "UserGroups", Name: "GroupDetails"},
		Operations: []ast.AlterPageOperation{
			&ast.InsertWidgetOp{Position: "AFTER", Target: ast.WidgetRef{Widget: "dropdownitem1"},
				Widgets: []*ast.WidgetV3{dropdownItem("n")}},
			&ast.ReplaceWidgetOp{Target: ast.WidgetRef{Widget: "dropdownitem3"},
				NewWidgets: []*ast.WidgetV3{dropdownItem("r")}},
		},
	}
	for _, v := range ValidateWidgetPropertiesForStatement(stmt, pdsDropdownRegistry()) {
		if v.RuleID == "MDL-WIDGET25" || v.RuleID == "MDL-WIDGET26" {
			t.Errorf("ALTER top-level dropdownitem rejected: %s %s", v.RuleID, v.Message)
		}
	}
}

// The control: a document root has no widget to hold an entry, so CREATE PAGE
// must still refuse one there.
func TestValidateCreatePage_RootDropdownItemStillRefused(t *testing.T) {
	stmt := &ast.CreatePageStmtV3{
		Name:    ast.QualifiedName{Module: "UserGroups", Name: "P"},
		Widgets: []*ast.WidgetV3{dropdownItem("n")},
	}
	var ids []string
	for _, v := range ValidateWidgetPropertiesForStatement(stmt, pdsDropdownRegistry()) {
		ids = append(ids, v.RuleID)
	}
	joined := strings.Join(ids, ",")
	if !strings.Contains(joined, "MDL-WIDGET25") && !strings.Contains(joined, "MDL-WIDGET26") {
		t.Errorf("a root dropdownitem in CREATE PAGE was accepted; violations: %v", ids)
	}
}
