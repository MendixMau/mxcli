// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// TestCreateOrModifyMicroflow_PreservesDeepLinkURL is the executor half of
// #1120: a statement that never mentions the deep link must not clear one.
//
// A microflow's URL (Mendix 10.6+) has no MDL spelling, so `CREATE OR MODIFY
// MICROFLOW` rebuilt it from the AST default — an empty string — and the deep
// link was gone. The class is the one ADR-0005 calls guard-don't-drop, but with
// nothing to guard against: unlike a queued call or an unwritable REST body,
// a microflow with no URL is a perfectly valid document, so `mxcli check`,
// `mx check` and mxbuild all report success before and after. Only Studio Pro
// shows the loss. Preserving is therefore the whole remedy; there is no
// refusal to fall back on.
func TestCreateOrModifyMicroflow_PreservesDeepLinkURL(t *testing.T) {
	const moduleID = model.ID("module-1")
	stored := []*microflows.Microflow{{
		BaseElement:         model.BaseElement{ID: "mf-item"},
		ContainerID:         moduleID,
		Name:                "ACT_Item",
		URL:                 "item/{Key}",
		URLSearchParameters: []string{"MyModule.ACT_Item.Key"},
	}}
	ctx, written := microflowWriteProbe(t, stored, moduleID)

	stmt := &ast.CreateMicroflowStmt{
		Name:           ast.QualifiedName{Module: "MyModule", Name: "ACT_Item"},
		CreateOrModify: true,
	}
	if err := execCreateMicroflow(ctx, stmt); err != nil {
		t.Fatalf("CREATE OR MODIFY MICROFLOW failed: %v", err)
	}
	if *written == nil {
		t.Fatal("no microflow was written")
	}
	if got := (*written).URL; got != "item/{Key}" {
		t.Errorf("rewrite dropped the deep-link URL: got %q, want %q", got, "item/{Key}")
	}
	if got := (*written).URLSearchParameters; len(got) != 1 || got[0] != "MyModule.ACT_Item.Key" {
		t.Errorf("rewrite dropped UrlSearchParameters: got %v, want [MyModule.ACT_Item.Key]", got)
	}
}

// TestCreateMicroflow_InventsNoDeepLinkURL is the control for the test above: a
// microflow that never had a URL must not acquire one, or "preserve the stored
// value" would just be a different silent change in the same place.
func TestCreateMicroflow_InventsNoDeepLinkURL(t *testing.T) {
	const moduleID = model.ID("module-1")
	ctx, written := microflowWriteProbe(t, nil, moduleID)

	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "ACT_Fresh"},
	}
	if err := execCreateMicroflow(ctx, stmt); err != nil {
		t.Fatalf("CREATE MICROFLOW failed: %v", err)
	}
	if *written == nil {
		t.Fatal("no microflow was written")
	}
	if got := (*written).URL; got != "" {
		t.Errorf("a fresh microflow acquired a deep-link URL: %q", got)
	}
	if got := (*written).URLSearchParameters; len(got) != 0 {
		t.Errorf("a fresh microflow acquired UrlSearchParameters: %v", got)
	}
}
