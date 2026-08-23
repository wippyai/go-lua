package bind

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/typeindex"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestParsedGenericConstraintsSeeTheirParameterGroup(t *testing.T) {
	stmts, err := parse.ParseString(`
type Pair<T, U: T> = { first: T, second: U }
local pair = function<T, U: T>(first: T, second: U): U
	return second
end
interface API
	function pair<T, U: T>(first: T, second: U): U
end
`, "generic_constraints.lua")
	if err != nil {
		t.Fatal(err)
	}
	alias := stmts[0].(*ast.TypeDefStmt)
	runtimeFn := stmts[1].(*ast.LocalAssignStmt).Exprs[0].(*ast.FunctionExpr)
	staticFn, ok := stmts[2].(*ast.InterfaceDefStmt).Members[0].Type.(*ast.FunctionTypeExpr)
	if !ok {
		t.Fatalf("interface member = %#v, want function signature", stmts[2].(*ast.InterfaceDefStmt).Members[0])
	}
	result := BindChunk(stmts, typeindex.Table{})

	tests := []struct {
		name       string
		params     []TypeDecl
		constraint *ast.PrimitiveTypeExpr
	}{
		{"alias", result.TypeDefParams(alias), alias.TypeParams[1].Constraint.(*ast.PrimitiveTypeExpr)},
		{"runtime", result.FunctionTypeParams(runtimeFn), runtimeFn.TypeParams[1].Constraint.(*ast.PrimitiveTypeExpr)},
		{"static", result.FunctionTypeParams(staticFn), staticFn.TypeParams[1].Constraint.(*ast.PrimitiveTypeExpr)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if len(test.params) != 2 {
				t.Fatalf("parameters = %#v, want T and U", test.params)
			}
			got, ok := result.PrimitiveTypeRef(test.constraint)
			if !ok || got.ID != test.params[0].ID {
				t.Fatalf("U constraint = %#v/%v, want T %#v", got, ok, test.params[0])
			}
		})
	}
}

func TestParsedGenericParameterScopesRestoreOuterType(t *testing.T) {
	stmts, err := parse.ParseString(`
type T = string
type Pair<T, U: T> = { first: T, second: U }
local afterAlias: T
local pair = function<T, U: T>(first: T, second: U): U
	return second
end
local afterRuntime: T
interface API
	function pair<T, U: T>(first: T, second: U): U
end
local afterStatic: T
`, "generic_scope_lifetime.lua")
	if err != nil {
		t.Fatal(err)
	}
	outer := stmts[0].(*ast.TypeDefStmt)
	alias := stmts[1].(*ast.TypeDefStmt)
	runtimeFn := stmts[3].(*ast.LocalAssignStmt).Exprs[0].(*ast.FunctionExpr)
	staticFn, ok := stmts[5].(*ast.InterfaceDefStmt).Members[0].Type.(*ast.FunctionTypeExpr)
	if !ok {
		t.Fatalf("interface member = %#v, want function signature", stmts[5].(*ast.InterfaceDefStmt).Members[0])
	}
	result := BindChunk(stmts, typeindex.Table{})
	outerDecl, _ := result.TypeDef(outer)

	groups := []struct {
		params     []TypeDecl
		constraint *ast.PrimitiveTypeExpr
	}{
		{result.TypeDefParams(alias), alias.TypeParams[1].Constraint.(*ast.PrimitiveTypeExpr)},
		{result.FunctionTypeParams(runtimeFn), runtimeFn.TypeParams[1].Constraint.(*ast.PrimitiveTypeExpr)},
		{result.FunctionTypeParams(staticFn), staticFn.TypeParams[1].Constraint.(*ast.PrimitiveTypeExpr)},
	}
	for i, group := range groups {
		if len(group.params) != 2 {
			t.Fatalf("group %d = %#v, want T/U", i, group.params)
		}
		got, ok := result.PrimitiveTypeRef(group.constraint)
		if !ok || got.ID != group.params[0].ID || got.ID == outerDecl.ID {
			t.Fatalf("group %d U constraint = %#v/%v, want nearest T %#v", i, got, ok, group.params[0])
		}
	}
	for _, index := range []int{2, 4, 6} {
		annotation := stmts[index].(*ast.LocalAssignStmt).Types[0].(*ast.PrimitiveTypeExpr)
		got, ok := result.PrimitiveTypeRef(annotation)
		if !ok || got.ID != outerDecl.ID {
			t.Fatalf("post-group annotation %d = %#v/%v, want outer T %#v", index, got, ok, outerDecl)
		}
	}
}
