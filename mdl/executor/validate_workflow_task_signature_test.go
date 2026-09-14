// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// wfSigCtx is a project shaped like the mxbuild measurements behind these rules
// (11.13.0): one module M with a context entity Ctx and a Base <- Sub pair, a
// task page per parameter shape, a targeting microflow per signature shape, and
// one stored workflow for the ALTER cases.
func wfSigCtx(t *testing.T, version string, major, minor int) *ExecContext {
	t.Helper()
	mod := &model.Module{Name: "M"}
	mod.ID = "mod1"

	ctxEnt := &domainmodel.Entity{Name: "Ctx"}
	base := &domainmodel.Entity{Name: "Base"}
	sub := &domainmodel.Entity{Name: "Sub", GeneralizationRef: "M.Base"}
	for _, e := range []*domainmodel.Entity{ctxEnt, base, sub} {
		e.ContainerID = "dm1"
	}
	dm := &domainmodel.DomainModel{Entities: []*domainmodel.Entity{ctxEnt, base, sub}}
	dm.ID = "dm1"
	dm.ContainerID = "mod1"

	page := func(name string, paramEntities ...string) *pages.Page {
		p := &pages.Page{Name: name}
		p.ContainerID = "mod1"
		for i, e := range paramEntities {
			p.Parameters = append(p.Parameters, &pages.PageParameter{Name: "P" + string(rune('A'+i)), EntityName: e})
		}
		return p
	}
	obj := func(qn string) microflows.DataType { return &microflows.ObjectType{EntityQualifiedName: qn} }
	mf := func(name string, types ...microflows.DataType) *microflows.Microflow {
		m := &microflows.Microflow{Name: name}
		m.ContainerID = "mod1"
		for i, dt := range types {
			m.Parameters = append(m.Parameters, &microflows.MicroflowParameter{Name: "p" + string(rune('a'+i)), Type: dt})
		}
		return m
	}

	stored := &workflows.Workflow{Name: "Stored", Parameter: &workflows.WorkflowParameter{EntityRef: "M.Ctx"}}
	stored.ContainerID = "mod1"

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ProjectVersionFunc: func() *types.ProjectVersion {
			return &types.ProjectVersion{ProductVersion: version, MajorVersion: major, MinorVersion: minor}
		},
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if strings.EqualFold(name, "M") {
				return mod, nil
			}
			return nil, nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListPagesFunc: func() ([]*pages.Page, error) {
			return []*pages.Page{
				page("PgGood", "System.WorkflowUserTask"),
				page("PgNone"),
				page("PgCtx", "M.Ctx"),
				page("PgExtra", "System.WorkflowUserTask", "M.Ctx"),
			}, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{
				mf("MF_Good", obj("System.Workflow"), obj("M.Ctx")),
				mf("MF_Reversed", obj("M.Ctx"), obj("System.Workflow")),
				mf("MF_WfOnly", obj("System.Workflow")),
				mf("MF_NoParams"),
				mf("MF_Extra", obj("System.Workflow"), obj("M.Ctx"), &microflows.StringType{}),
				mf("MF_TakesBase", obj("System.Workflow"), obj("M.Base")),
				mf("MF_TakesSub", obj("System.Workflow"), obj("M.Sub")),
			}, nil
		},
		ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{stored}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx
}

// Each case is a row of the mxbuild measurement (11.13.0) — want is the CE code
// the build reported, "" where it built at 0 errors. The clean rows are the ones
// that keep the rule from being a false refusal, and two of them are not what a
// plausible reading of the error message would predict: a task page may carry
// extra parameters, and a targeting microflow may take its two in either order.
func TestWorkflowTaskSignatures_Create(t *testing.T) {
	ut := func(ctxEntity, clause string) string {
		return `create workflow M.W parameter $C: ` + ctxEntity + ` begin ` + clause + ` end workflow;`
	}
	cases := []struct{ name, src, want string }{
		// Task page (CE7410 no parameters, CE7412 no WorkflowUserTask parameter).
		{"page takes WorkflowUserTask", ut("M.Ctx", `user task T 'c' page M.PgGood outcomes 'a' { } 'b' { };`), ""},
		{"page takes WorkflowUserTask plus an extra param", ut("M.Ctx", `user task T 'c' page M.PgExtra outcomes 'a' { } 'b' { };`), ""},
		{"page takes nothing", ut("M.Ctx", `user task T 'c' page M.PgNone outcomes 'a' { } 'b' { };`), "CE7410"},
		{"page takes the context entity", ut("M.Ctx", `user task T 'c' page M.PgCtx outcomes 'a' { } 'b' { };`), "CE7412"},
		{"multi user task, context page", ut("M.Ctx", `multi user task T 'c' page M.PgCtx outcomes 'a' { } 'b' { };`), "CE7412"},
		{"nested in an outcome", ut("M.Ctx", `user task T 'c' page M.PgGood outcomes 'a' { user task U 'u' page M.PgCtx outcomes 'x' { } 'y' { }; } 'b' { };`), "CE7412"},

		// Targeting microflow (CE6677): exactly System.Workflow + the context
		// entity or a generalization of it, in either order.
		{"targeting (Workflow, Ctx)", ut("M.Ctx", `user task T 'c' page M.PgGood targeting microflow M.MF_Good outcomes 'a' { } 'b' { };`), ""},
		{"targeting (Ctx, Workflow) reversed", ut("M.Ctx", `user task T 'c' page M.PgGood targeting microflow M.MF_Reversed outcomes 'a' { } 'b' { };`), ""},
		{"targeting (Workflow) only", ut("M.Ctx", `user task T 'c' page M.PgGood targeting microflow M.MF_WfOnly outcomes 'a' { } 'b' { };`), "CE6677"},
		{"targeting no params", ut("M.Ctx", `user task T 'c' page M.PgGood targeting microflow M.MF_NoParams outcomes 'a' { } 'b' { };`), "CE6677"},
		{"targeting extra param", ut("M.Ctx", `user task T 'c' page M.PgGood targeting microflow M.MF_Extra outcomes 'a' { } 'b' { };`), "CE6677"},
		{"targeting users microflow", ut("M.Ctx", `user task T 'c' page M.PgGood targeting users microflow M.MF_WfOnly outcomes 'a' { } 'b' { };`), "CE6677"},
		{"targeting groups microflow", ut("M.Ctx", `user task T 'c' page M.PgGood targeting groups microflow M.MF_WfOnly outcomes 'a' { } 'b' { };`), "CE6677"},
		{"microflow takes a generalization of the context", ut("M.Sub", `user task T 'c' page M.PgGood targeting microflow M.MF_TakesBase outcomes 'a' { } 'b' { };`), ""},
		{"microflow takes a specialization of the context", ut("M.Base", `user task T 'c' page M.PgGood targeting microflow M.MF_TakesSub outcomes 'a' { } 'b' { };`), "CE6677"},
		{"xpath targeting is not a microflow", ut("M.Ctx", `user task T 'c' page M.PgGood targeting xpath '[true()]' outcomes 'a' { } 'b' { };`), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := checkScript(t, wfSigCtx(t, "11.13.0", 11, 13), c.src)
			assertSignatureVerdict(t, got, c.want)
		})
	}
}

// The ordinary shape is ONE script that creates the page, the targeting
// microflow and the workflow. A rule that could only see stored documents would
// fire on the minority case, and the majority case is the one that reaches a
// build half-written.
func TestWorkflowTaskSignatures_ScriptDefined(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			"script page typed to the context",
			`create page M.ScriptPg ( title: 'x', layout: Atlas_Core.Atlas_Default, params: { $C: M.Ctx } ) { };
create workflow M.W parameter $C: M.Ctx begin user task T 'c' page M.ScriptPg outcomes 'a' { } 'b' { }; end workflow;`,
			"CE7412",
		},
		{
			"script page typed to WorkflowUserTask",
			`create page M.ScriptPg ( title: 'x', layout: Atlas_Core.Atlas_Default, params: { $WorkflowUserTask: System.WorkflowUserTask } ) { };
create workflow M.W parameter $C: M.Ctx begin user task T 'c' page M.ScriptPg outcomes 'a' { } 'b' { }; end workflow;`,
			"",
		},
		{
			"script microflow with one parameter",
			`create microflow M.ScriptMF ( $workflow: System.Workflow ) returns list of System.User as $u begin return $u; end;
create workflow M.W parameter $C: M.Ctx begin user task T 'c' page M.PgGood targeting microflow M.ScriptMF outcomes 'a' { } 'b' { }; end workflow;`,
			"CE6677",
		},
		{
			"script microflow reversed",
			`create microflow M.ScriptMF ( $context: M.Ctx, $workflow: System.Workflow ) returns list of System.User as $u begin return $u; end;
create workflow M.W parameter $C: M.Ctx begin user task T 'c' page M.PgGood targeting microflow M.ScriptMF outcomes 'a' { } 'b' { }; end workflow;`,
			"",
		},
		{
			// A script entity inheriting from a stored one: the chain is
			// resolvable through the script, then the project.
			"script context entity extends the microflow's entity",
			`create persistent entity M.NewSub extends M.Base ( X: string(10) );
create workflow M.W parameter $C: M.NewSub begin user task T 'c' page M.PgGood targeting microflow M.MF_TakesBase outcomes 'a' { } 'b' { }; end workflow;`,
			"",
		},
		{
			// The chain leaves what can be resolved. Unprovable is not a
			// refusal: exec refuses on an error, so a guess here blocks a
			// script mxbuild would build.
			"unresolvable inheritance chain is not refused",
			`create persistent entity M.Orphan extends Elsewhere.Unknown ( X: string(10) );
create workflow M.W parameter $C: M.Orphan begin user task T 'c' page M.PgGood targeting microflow M.MF_TakesBase outcomes 'a' { } 'b' { }; end workflow;`,
			"",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := checkScript(t, wfSigCtx(t, "11.13.0", 11, 13), c.src)
			assertSignatureVerdict(t, got, c.want)
		})
	}
}

// ALTER writes the same two references into a stored workflow, so it gets the
// same rule. SET TARGETING MICROFLOW has no context entity in the statement —
// it comes from the stored workflow's parameter.
func TestWorkflowTaskSignatures_Alter(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"set page", `alter workflow M.Stored set activity T page M.PgCtx;`, "CE7412"},
		{"set page (valid)", `alter workflow M.Stored set activity T page M.PgExtra;`, ""},
		{"set targeting microflow", `alter workflow M.Stored set activity T targeting microflow M.MF_WfOnly;`, "CE6677"},
		{"set targeting microflow (valid)", `alter workflow M.Stored set activity T targeting microflow M.MF_Reversed;`, ""},
		{"insert user task", `alter workflow M.Stored insert after T user task U 'u' page M.PgNone outcomes 'a' { } 'b' { };`, "CE7410"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := checkScript(t, wfSigCtx(t, "11.13.0", 11, 13), c.src)
			assertSignatureVerdict(t, got, c.want)
		})
	}
}

// The rules are measured on 11.13 only. On an older project the check stays
// out of the way rather than applying a guess — the MDL-WF07 precedent.
func TestWorkflowTaskSignatures_NotAppliedBeforeMendix11(t *testing.T) {
	src := `create workflow M.W parameter $C: M.Ctx begin user task T 'c' page M.PgCtx targeting microflow M.MF_WfOnly outcomes 'a' { } 'b' { }; end workflow;`
	got := checkScript(t, wfSigCtx(t, "10.24.0", 10, 24), src)
	assertSignatureVerdict(t, got, "")
}

// assertSignatureVerdict checks for the CE code, or for the absence of every
// code this rule reports. It does not demand an empty result: script-defined
// pages trip unrelated checks in a mock project (no Atlas layout), and those
// are not what these tests are about.
func assertSignatureVerdict(t *testing.T, got, want string) {
	t.Helper()
	if want != "" {
		if !strings.Contains(got, want) {
			t.Errorf("expected %s, got:\n%s", want, got)
		}
		return
	}
	for _, code := range []string{"CE7410", "CE7412", "CE6677"} {
		if strings.Contains(got, code) {
			t.Errorf("expected no signature error, got %s:\n%s", code, got)
		}
	}
}
