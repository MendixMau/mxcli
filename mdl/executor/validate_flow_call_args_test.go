// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// governanceSig is the shape of the reported call target:
// DataLake_Extension.SUB_RegisterEventGovernance takes an enumeration-typed
// EventType plus two object parameters.
func governanceSig() map[string]*flowSignature {
	return map[string]*flowSignature{
		"datalake_extension.sub_registereventgovernance": {
			Params: []flowParam{
				{Name: "EventType", Enum: "DataLake_Extension.DatalakeEventTypes_Governance"},
				{Name: "Member", Entity: "Admin.Member", Object: true},
				{Name: "AppVersionView", Entity: "AppStore.AppVersionView", Object: true},
			},
		},
	}
}

func callGov(args ...ast.CallArgument) *ast.CallMicroflowStmt {
	return &ast.CallMicroflowStmt{
		MicroflowName: ast.QualifiedName{Module: "DataLake_Extension", Name: "SUB_RegisterEventGovernance"},
		Arguments:     args,
	}
}

func strArg(name, v string) ast.CallArgument {
	return ast.CallArgument{Name: name, Value: &ast.SourceExpr{
		Expression: &ast.LiteralExpr{Value: v, Kind: ast.LiteralString}, Source: "'" + v + "'"}}
}

func varArg(name, v string) ast.CallArgument {
	return ast.CallArgument{Name: name, Value: &ast.VariableExpr{Name: v}}
}

func enumArg(name, qn string) ast.CallArgument {
	return ast.CallArgument{Name: name, Value: &ast.QualifiedNameExpr{
		QualifiedName: ast.QualifiedName{Module: "DataLake_Extension", Name: qn}}}
}

func TestFlowCallArgErrors(t *testing.T) {
	tests := []struct {
		name string
		body []ast.MicroflowStatement
		want []string // substrings, one per expected error; nil = no error
	}{
		{
			name: "reported call: string literal for enum AND two missing arguments",
			body: []ast.MicroflowStatement{callGov(strArg("EventType", "UserGroups_GuestRemoved"))},
			want: []string{"CE0117", "DatalakeEventTypes_Governance.UserGroups_GuestRemoved", "CE0115", "$Member", "$AppVersionView"},
		},
		{
			name: "complete call with enum value is fine",
			body: []ast.MicroflowStatement{callGov(
				enumArg("EventType", "DatalakeEventTypes_Governance.UserGroups_GuestRemoved"),
				varArg("Member", "M"), varArg("AppVersionView", "V"))},
		},
		{
			name: "missing argument inside an IF branch",
			body: []ast.MicroflowStatement{&ast.IfStmt{ThenBody: []ast.MicroflowStatement{
				callGov(enumArg("EventType", "DatalakeEventTypes_Governance.X"), varArg("Member", "M"))}}},
			want: []string{"CE0115", "$AppVersionView"},
		},
		{
			name: "argument names match case-insensitively",
			body: []ast.MicroflowStatement{callGov(
				enumArg("eventtype", "DatalakeEventTypes_Governance.X"),
				varArg("member", "M"), varArg("appversionview", "V"))},
		},
		{
			name: "unresolvable target is left to the reference check",
			body: []ast.MicroflowStatement{&ast.CallMicroflowStmt{
				MicroflowName: ast.QualifiedName{Module: "X", Name: "Unknown"}}},
		},
		{
			name: "string variable for enum param is not judged (type unknown here)",
			body: []ast.MicroflowStatement{callGov(
				varArg("EventType", "S"), varArg("Member", "M"), varArg("AppVersionView", "V"))},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := flowCallArgErrors(tt.body, governanceSig())
			joined := strings.Join(got, "\n")
			if tt.want == nil {
				if len(got) != 0 {
					t.Fatalf("expected no errors, got:\n%s", joined)
				}
				return
			}
			for _, w := range tt.want {
				if !strings.Contains(joined, w) {
					t.Errorf("missing %q in:\n%s", w, joined)
				}
			}
		})
	}
}

// A flow created earlier in the same script declares its enum parameter with
// `enum Module.Name`; the signature must carry that.
func TestAstFlowSignatureRecordsExplicitEnum(t *testing.T) {
	sig := astFlowSignature([]ast.MicroflowParam{
		{Name: "Kind", Type: ast.DataType{Kind: ast.TypeEnumeration, ExplicitEnum: true,
			EnumRef: &ast.QualifiedName{Module: "M", Name: "Kinds"}}},
		{Name: "Ambiguous", Type: ast.DataType{Kind: ast.TypeEnumeration,
			EnumRef: &ast.QualifiedName{Module: "M", Name: "Thing"}}},
	}, nil)
	if sig.Params[0].Enum != "M.Kinds" {
		t.Errorf("explicit enum param: Enum = %q, want M.Kinds", sig.Params[0].Enum)
	}
	if sig.Params[1].Enum != "" {
		t.Errorf("bare Module.Name param must not be assumed an enum, got %q", sig.Params[1].Enum)
	}
}

// End to end through the parser: the callee is created in the same script with
// an explicit `enum` parameter, and the caller passes a string literal.
func TestFlowCallArgErrorsFromParsedScript(t *testing.T) {
	script := `create microflow M.SUB ($Kind: enum M.Kinds, $Note: String) begin end;
/
create microflow M.Caller () begin
  call microflow M.SUB (Kind = 'Removed');
end;
/
`
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	callee := prog.Statements[0].(*ast.CreateMicroflowStmt)
	caller := prog.Statements[1].(*ast.CreateMicroflowStmt)
	sigs := map[string]*flowSignature{"m.sub": astFlowSignature(callee.Parameters, callee.ReturnType)}
	joined := strings.Join(flowCallArgErrors(caller.Body, sigs), "\n")
	for _, w := range []string{"CE0117", "M.Kinds.Removed", "CE0115", "$Note"} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing %q in:\n%s", w, joined)
		}
	}
}
