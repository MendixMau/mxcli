// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
)

// The System module's enumerations are platform built-ins with no stored unit:
// the backend synthesizes them so they can be READ (mendixlabs/mxcli#1102).
// Making them visible also makes them addressable by the write paths, and the
// System module has no stored unit to contain anything — so every write has to
// be refused at the statement, not discovered as a disk error underneath.
//
// On the enumeration path the pre-fix behaviour was worse than a bad message:
// `CREATE ENUMERATION System.BrandNewThing` REPORTED SUCCESS and wrote a unit
// whose ContainerID was the synthetic module ID 00000000-…-0001, which is not a
// unit in the project — an orphan with a dangling parent. (Measured on the
// expr-checker fixture: 369 → 370 units, container present in no Unit row.)
// Entities happen to fail safe because they need the virtual domain-model unit
// loaded first; enumerations are units in their own right, so nothing stopped
// them.

// systemEnumCtx builds a context whose backend exposes a user module plus the
// virtual System module and one synthesized System enumeration, and records
// whether any write reached the backend.
func systemEnumCtx(t *testing.T) (*ExecContext, *[]string) {
	t.Helper()
	user := mkModule("Sales")
	system := &model.Module{
		BaseElement: model.BaseElement{ID: model.ID(meta.SystemModuleID)},
		Name:        "System",
	}

	userEnum := mkEnumeration(user.ID, "OrderStatus", "Draft", "Shipped")
	sysEnum := mkEnumeration(system.ID, "WorkflowActivityType", "UserTask", "CallMicroflow")

	h := mkHierarchy(user, system)
	withContainer(h, userEnum.ContainerID, user.ID)
	withContainer(h, sysEnum.ContainerID, system.ID)

	var writes []string
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{user, system}, nil
		},
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) {
			return []*model.Enumeration{userEnum, sysEnum}, nil
		},
		CreateEnumerationFunc: func(e *model.Enumeration) error {
			writes = append(writes, "create:"+e.Name)
			return nil
		},
		UpdateEnumerationFunc: func(e *model.Enumeration) error {
			writes = append(writes, "update:"+e.Name)
			return nil
		},
		DeleteEnumerationFunc: func(id model.ID) error {
			writes = append(writes, "delete:"+string(id))
			return nil
		},
		MoveEnumerationFunc: func(e *model.Enumeration) error {
			writes = append(writes, "move:"+e.Name)
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, &writes
}

func sysQN(name string) ast.QualifiedName {
	return ast.QualifiedName{Module: "System", Name: name}
}

// TestDescribeEnumeration_System is the symptom from the issue: the values have
// to be reportable, because nothing else in mxcli can tell you what they are.
func TestDescribeEnumeration_System(t *testing.T) {
	ctx, _ := systemEnumCtx(t)
	var out strings.Builder
	ctx.Output = &out

	if err := describeEnumeration(ctx, sysQN("WorkflowActivityType")); err != nil {
		t.Fatalf("describe enumeration System.WorkflowActivityType: %v", err)
	}
	got := out.String()
	for _, want := range []string{"System.WorkflowActivityType", "UserTask", "CallMicroflow"} {
		if !strings.Contains(got, want) {
			t.Errorf("describe output missing %q:\n%s", want, got)
		}
	}
	// The write paths refuse System, so DESCRIBE must not emit a statement that
	// mxcli would reject if pasted back — a describe → exec round trip that
	// cannot work is worse than one that is plainly marked read-only.
	if strings.Contains(got, "create or modify enumeration") {
		t.Errorf("describe emits a CREATE statement for a read-only System enumeration:\n%s", got)
	}
	if !strings.Contains(got, "read-only") {
		t.Errorf("describe output does not say the enumeration is read-only:\n%s", got)
	}
}

// TestDescribeEnumeration_UserStillRoundTrips is the control for the branch
// above: an ordinary enumeration must still describe as re-executable MDL.
func TestDescribeEnumeration_UserStillRoundTrips(t *testing.T) {
	ctx, _ := systemEnumCtx(t)
	var out strings.Builder
	ctx.Output = &out

	if err := describeEnumeration(ctx, ast.QualifiedName{Module: "Sales", Name: "OrderStatus"}); err != nil {
		t.Fatalf("describe enumeration Sales.OrderStatus: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "create or modify enumeration Sales.OrderStatus") {
		t.Errorf("user enumeration no longer describes as re-executable MDL:\n%s", got)
	}
}

// TestSystemEnumerationWrites_AreRefused covers all four write verbs. Each one
// must refuse BEFORE the backend is touched — the assertion on `writes` is the
// point, since a refusal that still wrote would leave the orphan behind.
func TestSystemEnumerationWrites_AreRefused(t *testing.T) {
	cases := []struct {
		name string
		run  func(ctx *ExecContext) error
	}{
		{"create", func(ctx *ExecContext) error {
			return execCreateEnumeration(ctx, &ast.CreateEnumerationStmt{
				Name:   sysQN("BrandNewThing"),
				Values: []ast.EnumValue{{Name: "A", Caption: "a"}},
			})
		}},
		{"create or modify", func(ctx *ExecContext) error {
			return execCreateEnumeration(ctx, &ast.CreateEnumerationStmt{
				Name:           sysQN("WorkflowActivityType"),
				CreateOrModify: true,
				Values:         []ast.EnumValue{{Name: "A", Caption: "a"}},
			})
		}},
		{"alter add value", func(ctx *ExecContext) error {
			return execAlterEnumeration(ctx, &ast.AlterEnumerationStmt{
				Name:      sysQN("WorkflowActivityType"),
				Operation: ast.AlterEnumAdd,
				ValueName: "Invented",
				Caption:   "Invented",
			})
		}},
		{"drop", func(ctx *ExecContext) error {
			return execDropEnumeration(ctx, &ast.DropEnumerationStmt{
				Name: sysQN("WorkflowActivityType"),
			})
		}},
		{"move out", func(ctx *ExecContext) error {
			return moveEnumeration(ctx, sysQN("WorkflowActivityType"), model.ID("mod-sales"), "Sales")
		}},
		{"rename", func(ctx *ExecContext) error {
			return execRenameEnumeration(ctx, &ast.RenameStmt{
				Name:    sysQN("WorkflowActivityType"),
				NewName: "Renamed",
			})
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, writes := systemEnumCtx(t)
			err := tc.run(ctx)
			if err == nil {
				t.Fatalf("%s on a System enumeration was accepted, want a refusal", tc.name)
			}
			msg := err.Error()
			if !strings.Contains(msg, "System") {
				t.Errorf("refusal does not name the System module: %q", msg)
			}
			// The pre-fix failures were a success message (create) or a raw
			// mxunit path (the rest). Neither is an explanation.
			if strings.Contains(msg, ".mxunit") || strings.Contains(msg, "no such file") {
				t.Errorf("refusal leaks a storage error instead of explaining: %q", msg)
			}
			if len(*writes) != 0 {
				t.Errorf("refusal still reached the backend: %v", *writes)
			}
		})
	}
}

// TestMoveEnumerationIntoSystem_IsRefused covers the other end of MOVE: the
// enumeration being moved is an ordinary one, but the destination is System.
// Guarding only the source would let a user enumeration be moved INTO a module
// with no stored unit, which is the same orphan by another route.
func TestMoveEnumerationIntoSystem_IsRefused(t *testing.T) {
	ctx, writes := systemEnumCtx(t)
	err := moveEnumeration(ctx,
		ast.QualifiedName{Module: "Sales", Name: "OrderStatus"},
		model.ID(meta.SystemModuleID), "System")
	if err == nil {
		t.Fatal("moving a user enumeration into System was accepted, want a refusal")
	}
	if !strings.Contains(err.Error(), "System") {
		t.Errorf("refusal does not name the System module: %q", err)
	}
	if len(*writes) != 0 {
		t.Errorf("refusal still reached the backend: %v", *writes)
	}
}

// TestUserEnumerationWrites_StillWork is the control. Without it the guard could
// be refusing every enumeration write and every test above would still pass.
func TestUserEnumerationWrites_StillWork(t *testing.T) {
	userQN := ast.QualifiedName{Module: "Sales", Name: "OrderStatus"}

	t.Run("create or modify", func(t *testing.T) {
		ctx, writes := systemEnumCtx(t)
		if err := execCreateEnumeration(ctx, &ast.CreateEnumerationStmt{
			Name:           userQN,
			CreateOrModify: true,
			Values:         []ast.EnumValue{{Name: "Draft", Caption: "Draft"}},
		}); err != nil {
			t.Fatalf("create or modify on a user enumeration: %v", err)
		}
		if len(*writes) == 0 {
			t.Error("user enumeration write did not reach the backend")
		}
	})

	t.Run("alter add value", func(t *testing.T) {
		ctx, writes := systemEnumCtx(t)
		if err := execAlterEnumeration(ctx, &ast.AlterEnumerationStmt{
			Name:      userQN,
			Operation: ast.AlterEnumAdd,
			ValueName: "Cancelled",
			Caption:   "Cancelled",
		}); err != nil {
			t.Fatalf("alter on a user enumeration: %v", err)
		}
		if len(*writes) == 0 {
			t.Error("user enumeration alter did not reach the backend")
		}
	})

	t.Run("drop", func(t *testing.T) {
		ctx, writes := systemEnumCtx(t)
		if err := execDropEnumeration(ctx, &ast.DropEnumerationStmt{Name: userQN}); err != nil {
			t.Fatalf("drop on a user enumeration: %v", err)
		}
		if len(*writes) == 0 {
			t.Error("user enumeration drop did not reach the backend")
		}
	})
}

// TestDropModuleSystem_IsRefused: DROP MODULE cascades over the module's
// documents, and the System module's are all synthesized. Before the refusal it
// reported "unit not found" once per document — 15 warnings and no change — which
// is noise the enumeration fix would otherwise have introduced (#1102).
func TestDropModuleSystem_IsRefused(t *testing.T) {
	ctx, writes := systemEnumCtx(t)
	err := execDropModule(ctx, &ast.DropModuleStmt{Name: "System"})
	if err == nil {
		t.Fatal("DROP MODULE System was accepted, want a refusal")
	}
	if !strings.Contains(err.Error(), "System") {
		t.Errorf("refusal does not name the module: %q", err)
	}
	if len(*writes) != 0 {
		t.Errorf("refusal still reached the backend: %v", *writes)
	}
}
