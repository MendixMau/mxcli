// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ALTER PAGE on the entries of a pluggable widget's object list — a PDS
// dropdown menu's `dropdownitem`s, an accordion's `group`s. DESCRIBE names the
// entries positionally (`dropdownitem1`, …), so the same names are the targets
// here: `insert after dropdownitem1 { dropdownitem … }`,
// `insert into pDSDropdownMenu1 { dropdownitem … }`, `drop widget dropdownitem2`,
// `replace dropdownitem3 with { dropdownitem … }`.
//
// An entry is not a widget: it is a CustomWidgets$WidgetObject in its owner's
// list, typed by the owner's Type. So new entries are built as the children of
// a throwaway "donor" widget of the owner's widget id — the same builder CREATE
// PAGE uses, so every item property, visibility rule and default is the one a
// fresh page would get — and the mutator lifts them out and repoints them at
// the owner's stored Type (see pagemutator/objectlist_items.go). DataGrid2
// columns keep their own path (`grid.column`), which runs first.

// objectListDonorName names the donor widget. It is built, serialized and
// discarded; it never reaches the page.
const objectListDonorName = "mxcliObjectListDonor"

// applyInsertObjectListItems routes an INSERT whose target or body is an
// object-list entry. handled is false when the op is an ordinary widget insert.
func applyInsertObjectListItems(ctx *ExecContext, mutator backend.PageMutator, op *ast.InsertWidgetOp, moduleName string, moduleID model.ID) (bool, error) {
	if len(op.Widgets) == 0 {
		return false, nil
	}
	if strings.EqualFold(op.Position, "INTO") {
		if op.Target.IsColumn() {
			return false, nil
		}
		id := mutator.PluggableWidgetID(op.Target.Widget)
		if id == "" {
			return false, nil
		}
		def := widgetDefByID(ctx, id)
		if def == nil {
			return false, nil
		}
		ol, matched, err := objectListForBody(def, op.Widgets)
		if err != nil || !matched {
			return matched, err
		}
		return true, insertObjectListDonor(ctx, mutator, op.Target.Widget, id, ol, -1, op.Widgets, moduleName, moduleID)
	}

	ref, ok, err := mutator.ResolveObjectListItem(op.Target.Widget, op.Target.Column)
	if err != nil {
		return true, err
	}
	if !ok {
		return false, nil
	}
	def := widgetDefByID(ctx, ref.WidgetID)
	if def == nil {
		return true, mdlerrors.NewValidation(fmt.Sprintf(
			"%s is an entry of %q, whose widget %s has no definition in this project; run `mxcli widget init`",
			op.Target.Name(), ref.Owner, ref.WidgetID))
	}
	ol, err := objectListByKey(def, ref.ListKey)
	if err != nil {
		return true, err
	}
	if err := requireEntriesOf(ol, op.Widgets, op.Target.Name()); err != nil {
		return true, err
	}
	index := ref.Index
	if strings.EqualFold(op.Position, "AFTER") {
		index++
	}
	return true, insertObjectListDonor(ctx, mutator, ref.Owner, ref.WidgetID, ol, index, op.Widgets, moduleName, moduleID)
}

// applyReplaceObjectListItem routes a REPLACE whose target is an object-list
// entry. The new entries are inserted after the old one before it is dropped,
// so a refused build leaves the list as it was.
func applyReplaceObjectListItem(ctx *ExecContext, mutator backend.PageMutator, op *ast.ReplaceWidgetOp, moduleName string, moduleID model.ID) (bool, error) {
	ref, ok, err := mutator.ResolveObjectListItem(op.Target.Widget, op.Target.Column)
	if err != nil {
		return true, err
	}
	if !ok {
		return false, nil
	}
	def := widgetDefByID(ctx, ref.WidgetID)
	if def == nil {
		return true, mdlerrors.NewValidation(fmt.Sprintf(
			"%s is an entry of %q, whose widget %s has no definition in this project; run `mxcli widget init`",
			op.Target.Name(), ref.Owner, ref.WidgetID))
	}
	ol, err := objectListByKey(def, ref.ListKey)
	if err != nil {
		return true, err
	}
	if err := requireEntriesOf(ol, op.NewWidgets, op.Target.Name()); err != nil {
		return true, err
	}
	if err := insertObjectListDonor(ctx, mutator, ref.Owner, ref.WidgetID, ol, ref.Index+1, op.NewWidgets, moduleName, moduleID); err != nil {
		return true, err
	}
	return true, mutator.DropObjectListItems(ref.Owner, ref.ListKey, []int{ref.Index})
}

// applyDropWidgetsAndItems splits a DROP WIDGET list into object-list entries
// and widgets. Every reference is resolved before anything is removed — entry
// names are positions, and removing one renumbers the rest.
func applyDropWidgetsAndItems(mutator backend.PageMutator, op *ast.DropWidgetOp) error {
	type listKey struct{ owner, key string }
	items := map[listKey][]int{}
	var order []listKey
	var widgets []backend.WidgetRef
	for _, t := range op.Targets {
		ref, ok, err := mutator.ResolveObjectListItem(t.Widget, t.Column)
		if err != nil {
			return err
		}
		if !ok {
			widgets = append(widgets, backend.WidgetRef{Widget: t.Widget, Column: t.Column})
			continue
		}
		k := listKey{ref.Owner, ref.ListKey}
		if _, seen := items[k]; !seen {
			order = append(order, k)
		}
		items[k] = append(items[k], ref.Index)
	}
	for _, k := range order {
		idx := items[k]
		sort.Ints(idx)
		if err := mutator.DropObjectListItems(k.owner, k.key, idx); err != nil {
			return err
		}
	}
	if len(widgets) == 0 {
		return nil
	}
	return mutator.DropWidget(widgets)
}

func widgetDefByID(ctx *ExecContext, widgetID string) *WidgetDefinition {
	reg := ctx.GetWidgetRegistry()
	if reg == nil {
		return nil
	}
	def, ok := reg.GetByWidgetID(widgetID)
	if !ok {
		return nil
	}
	return def
}

func objectListByKey(def *WidgetDefinition, key string) (*ObjectListMapping, error) {
	for i := range def.ObjectLists {
		if def.ObjectLists[i].PropertyKey == key {
			return &def.ObjectLists[i], nil
		}
	}
	return nil, mdlerrors.NewValidation(fmt.Sprintf("widget %s declares no object list %q", def.WidgetID, key))
}

// objectListForBody decides whether an INSERT INTO body is a run of entries of
// one of def's lists. matched is false for an ordinary widget body; a body
// that mixes entries with widgets, or entries of two lists, is refused.
func objectListForBody(def *WidgetDefinition, body []*ast.WidgetV3) (*ObjectListMapping, bool, error) {
	var found *ObjectListMapping
	entries := 0
	for _, w := range body {
		var hit *ObjectListMapping
		for i := range def.ObjectLists {
			if strings.EqualFold(def.ObjectLists[i].MDLContainer, w.Type) {
				hit = &def.ObjectLists[i]
				break
			}
		}
		if hit == nil {
			continue
		}
		entries++
		if found != nil && found != hit {
			return nil, true, mdlerrors.NewValidation(
				"one INSERT INTO adds entries to two different lists of the widget; use one INSERT per list")
		}
		found = hit
	}
	if entries == 0 {
		return nil, false, nil
	}
	if entries != len(body) {
		return nil, true, mdlerrors.NewValidation(fmt.Sprintf(
			"this INSERT mixes `%s` entries with other widgets; entries go into the widget's %q list, "+
				"widgets do not — use one INSERT for each",
			strings.ToLower(found.MDLContainer), found.PropertyKey))
	}
	return found, true, nil
}

// requireEntriesOf refuses a body that is not entirely entries of ol, which is
// all a position between two entries can hold.
func requireEntriesOf(ol *ObjectListMapping, body []*ast.WidgetV3, target string) error {
	for _, w := range body {
		if !strings.EqualFold(w.Type, ol.MDLContainer) {
			return mdlerrors.NewValidation(fmt.Sprintf(
				"%s is a `%s` entry of a %q list; only `%s` entries can go beside it or replace it, not `%s`",
				target, strings.ToLower(ol.MDLContainer), ol.PropertyKey, strings.ToLower(ol.MDLContainer), strings.ToLower(w.Type)))
		}
	}
	return nil
}

// insertObjectListDonor builds a donor widget of widgetID holding the entries
// and hands it to the mutator, which splices them into owner's list at index.
func insertObjectListDonor(ctx *ExecContext, mutator backend.PageMutator, owner, widgetID string,
	ol *ObjectListMapping, index int, entries []*ast.WidgetV3, moduleName string, moduleID model.ID) error {
	donor, err := buildObjectListDonor(ctx, mutator, owner, widgetID, ol, entries, moduleName, moduleID)
	if err != nil {
		return err
	}
	return mutator.InsertObjectListItems(owner, ol.PropertyKey, index, donor)
}

// buildObjectListDonor is a variable so executor tests can route an ALTER
// without the widget's .mpk type template, which only a real project carries;
// the donor's serialization and repointing are tested in pagemutator.
var buildObjectListDonor = func(ctx *ExecContext, mutator backend.PageMutator, owner, widgetID string,
	ol *ObjectListMapping, entries []*ast.WidgetV3, moduleName string, moduleID model.ID) (pages.Widget, error) {
	donor := &ast.WidgetV3{
		Type:       "pluggablewidget",
		Name:       objectListDonorName,
		Properties: map[string]any{"WidgetType": widgetID},
		Children:   entries,
	}
	entityCtx := mutator.EnclosingEntity(owner)
	built, err := buildWidgetsFromAST(ctx, []*ast.WidgetV3{donor}, moduleName, moduleID, entityCtx, mutator)
	if err != nil {
		return nil, mdlerrors.NewBackend("build "+strings.ToLower(ol.MDLContainer)+" entries", err)
	}
	if len(built) != 1 {
		return nil, mdlerrors.NewValidation(fmt.Sprintf("could not build the new %s entries", strings.ToLower(ol.MDLContainer)))
	}
	return built[0], nil
}
