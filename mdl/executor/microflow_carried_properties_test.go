// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Executor-level coverage for the microflow properties MDL cannot author and a
// rewrite therefore has to carry: the deep link, the export level, and the four
// concurrency settings. All were hardcoded in buildMicroflowFromStmt's rebuild
// struct or in microflowToGen, and all were lost with every checker green.
//
// Each has a paired control here — a CREATE must not acquire what only a stored
// microflow can supply — because "preserve the stored value" and "invent one"
// are the same edit seen from opposite sides.

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

// TestCreateOrModifyMicroflow_PreservesExportLevel is the executor half of the
// export-level carry. Same shape as the deep link above and found the same way:
// a constant in the writer that nothing downstream would miss.
func TestCreateOrModifyMicroflow_PreservesExportLevel(t *testing.T) {
	const moduleID = model.ID("module-1")
	stored := []*microflows.Microflow{{
		BaseElement: model.BaseElement{ID: "mf-api"},
		ContainerID: moduleID,
		Name:        "ACT_PublicApi",
		ExportLevel: "API",
	}}
	ctx, written := microflowWriteProbe(t, stored, moduleID)

	stmt := &ast.CreateMicroflowStmt{
		Name:           ast.QualifiedName{Module: "MyModule", Name: "ACT_PublicApi"},
		CreateOrModify: true,
	}
	if err := execCreateMicroflow(ctx, stmt); err != nil {
		t.Fatalf("CREATE OR MODIFY MICROFLOW failed: %v", err)
	}
	if *written == nil {
		t.Fatal("no microflow was written")
	}
	if got := (*written).ExportLevel; got != "API" {
		t.Errorf("rewrite demoted the export level: got %q, want %q", got, "API")
	}
}

// TestDescribeMicroflow_ReportsUnauthorableProperties covers the read side of
// both carries. Neither the deep link nor a non-default export level has an MDL
// spelling, so DESCRIBE cannot emit them as re-executable text — but a
// describe -> rename -> exec COPY has nothing to preserve from, so staying
// silent would hand the reader output that looks complete and is not.
//
// The export-level line is conditional on purpose: every document in every
// module measured stores "Hidden", so emitting it unconditionally would add a
// comment to every describe in order to say nothing.
func TestDescribeMicroflow_ReportsUnauthorableProperties(t *testing.T) {
	ctx, _ := newMockCtx(t)
	name := ast.QualifiedName{Module: "MyModule", Name: "ACT_Item"}

	render := func(mf *microflows.Microflow) string {
		return renderMicroflowMDL(ctx, "microflow", mf, name, nil, nil, nil)
	}

	got := render(&microflows.Microflow{Name: "ACT_Item", URL: "item/{Key}", ExportLevel: "API"})
	if !strings.Contains(got, "-- URL: item/{Key}") {
		t.Errorf("describe omitted the deep link; the output reads as complete:\n%s", got)
	}
	if !strings.Contains(got, "-- Export level: API") {
		t.Errorf("describe omitted a non-default export level:\n%s", got)
	}

	// The control: an ordinary microflow gets neither line. Without this the
	// test would pass against a describer that comments on every microflow.
	plain := render(&microflows.Microflow{Name: "ACT_Item", ExportLevel: "Hidden"})
	if strings.Contains(plain, "-- URL:") || strings.Contains(plain, "-- Export level:") {
		t.Errorf("describe commented on defaults:\n%s", plain)
	}
}

// TestCreateOrModifyMicroflow_PreservesConcurrencySettings is the executor half,
// and the one that was actually reachable by a user: the backend already read
// AllowConcurrentExecution and MarkAsUsed back, but the rebuild in
// buildMicroflowFromStmt overwrote both with its own literals before the backend
// ever saw them.
//
// The direction is why this went unreported. The rebuild wrote `true`, so a
// microflow that DISALLOWED concurrent execution came back allowing it — the
// running app's concurrency protection removed. CE4899 fires on
// disallow-without-a-message, never on allow, so no checker says anything; the
// error message and its translations go at the same time.
func TestCreateOrModifyMicroflow_PreservesConcurrencySettings(t *testing.T) {
	const moduleID = model.ID("module-1")
	stored := []*microflows.Microflow{{
		BaseElement:              model.BaseElement{ID: "mf-serial"},
		ContainerID:              moduleID,
		Name:                     "ACT_Serial",
		AllowConcurrentExecution: false,
		MarkAsUsed:               true,
		ConcurrencyErrorMessage: &model.Text{Translations: map[string]string{
			"en_US": "Already running",
			"nl_NL": "Wordt al uitgevoerd",
		}},
		ConcurrencyErrorMicroflow: "MyModule.ACT_OnBusy",
	}}
	ctx, written := microflowWriteProbe(t, stored, moduleID)

	stmt := &ast.CreateMicroflowStmt{
		Name:           ast.QualifiedName{Module: "MyModule", Name: "ACT_Serial"},
		CreateOrModify: true,
	}
	if err := execCreateMicroflow(ctx, stmt); err != nil {
		t.Fatalf("CREATE OR MODIFY MICROFLOW failed: %v", err)
	}
	if *written == nil {
		t.Fatal("no microflow was written")
	}
	if (*written).AllowConcurrentExecution {
		t.Error("rewrite re-allowed concurrent execution; the app's concurrency " +
			"protection is gone and no checker reports it")
	}
	if !(*written).MarkAsUsed {
		t.Error("rewrite cleared MarkAsUsed; the document is reported unused again")
	}
	if got := (*written).ConcurrencyErrorMicroflow; got != "MyModule.ACT_OnBusy" {
		t.Errorf("rewrite dropped the concurrency error microflow: %q", got)
	}
	msg := (*written).ConcurrencyErrorMessage
	if msg == nil || len(msg.Translations) != 2 {
		t.Fatalf("rewrite dropped the concurrency error message (or its translations): %#v", msg)
	}
}

// TestCreateMicroflow_ConcurrencyDefaults is the control: a NEW microflow still
// gets Mendix's defaults. Carrying is only ever from a stored document, so the
// fix must not change what a create produces — allow concurrency, not marked as
// used, no error handling.
func TestCreateMicroflow_ConcurrencyDefaults(t *testing.T) {
	const moduleID = model.ID("module-1")
	ctx, written := microflowWriteProbe(t, nil, moduleID)

	stmt := &ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "MyModule", Name: "ACT_Fresh"},
	}
	if err := execCreateMicroflow(ctx, stmt); err != nil {
		t.Fatalf("CREATE MICROFLOW failed: %v", err)
	}
	got := *written
	if got == nil {
		t.Fatal("no microflow was written")
	}
	if !got.AllowConcurrentExecution {
		t.Error("a new microflow must default to allowing concurrent execution")
	}
	if got.MarkAsUsed {
		t.Error("a new microflow must not be marked as used")
	}
	if got.ConcurrencyErrorMessage != nil || got.ConcurrencyErrorMicroflow != "" {
		t.Errorf("a new microflow acquired concurrency error handling: %#v / %q",
			got.ConcurrencyErrorMessage, got.ConcurrencyErrorMicroflow)
	}
}
