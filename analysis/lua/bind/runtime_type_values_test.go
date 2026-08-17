package bind

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestParsedRuntimeTypeValueAuthorityAndCallForms(t *testing.T) {
	stmts, err := parse.ParseString(`
type Shape = number
interface Contract end
local value = {}
return number(value), Shape(value), Contract(value), External(value),
	Shape.is(value), Shape["kind"](value), Shape:elem(value)
`, "runtime_type_authority.lua")
	if err != nil {
		t.Fatal(err)
	}
	result := BindChunk(stmts)
	shape, _ := result.TypeDef(stmts[0].(*ast.TypeDefStmt))
	contract, _ := result.InterfaceDef(stmts[1].(*ast.InterfaceDefStmt))
	calls := stmts[3].(*ast.ReturnStmt).Exprs
	wants := []struct {
		kind RuntimeTypeValueKind
		name string
		decl TypeDeclID
	}{
		{RuntimeTypeValuePrimitive, "number", 0},
		{RuntimeTypeValueDeclaration, "Shape", shape.ID},
		{RuntimeTypeValueDeclaration, "Contract", contract.ID},
		{},
		{RuntimeTypeValueDeclaration, "Shape", shape.ID},
		{RuntimeTypeValueDeclaration, "Shape", shape.ID},
		{RuntimeTypeValueDeclaration, "Shape", shape.ID},
	}
	for i, expression := range calls {
		call := expression.(*ast.FuncCallExpr)
		base := parsedRuntimeTypeBase(t, call)
		if wants[i].kind == 0 {
			if value, ok := result.RuntimeTypeValue(base); ok || value.Kind != 0 {
				t.Fatalf("call %d RuntimeTypeValue = %#v/%v, want absent", i, value, ok)
			}
			if !result.IsImplicitGlobalUse(base) {
				t.Fatalf("call %d external base is not an implicit global read", i)
			}
			global, ok := result.GlobalIdentity(base)
			if !ok || !global.Matches("External") {
				t.Fatalf("call %d external global = %#v/%v, want External", i, global, ok)
			}
			continue
		}
		value, ok := result.RuntimeTypeValue(base)
		if !ok || value.Kind != wants[i].kind || value.Name != wants[i].name || value.Decl.ID != wants[i].decl {
			t.Fatalf("call %d RuntimeTypeValue = %#v/%v, want %#v", i, value, ok, wants[i])
		}
		if result.IsImplicitGlobalUse(base) {
			t.Fatalf("call %d type base became implicit runtime global", i)
		}
	}
}

func TestParsedRuntimeTypeValueRejectsOrdinaryAndShadowedForms(t *testing.T) {
	stmts, err := parse.ParseString(`
type Shape = number
local method = "is"
local ordinary = Shape
local dynamic = Shape[method]()
local unknown = Shape.clone()
local function shadow(Shape)
	return Shape(), Shape.is(), Shape["kind"](), Shape:elem()
end
`, "runtime_type_shadow.lua")
	if err != nil {
		t.Fatal(err)
	}
	result := BindChunk(stmts)
	ordinary := stmts[2].(*ast.LocalAssignStmt).Exprs[0].(*ast.IdentExpr)
	if value, ok := result.RuntimeTypeValue(ordinary); ok || value.Kind != 0 {
		t.Fatalf("ordinary value = %#v/%v, want absent", value, ok)
	}
	for _, index := range []int{3, 4} {
		call := stmts[index].(*ast.LocalAssignStmt).Exprs[0].(*ast.FuncCallExpr)
		base := parsedRuntimeTypeBase(t, call)
		if value, ok := result.RuntimeTypeValue(base); ok || value.Kind != 0 {
			t.Fatalf("inexact call %d = %#v/%v, want absent", index, value, ok)
		}
	}
	fn := stmts[5].(*ast.LocalAssignStmt).Exprs[0].(*ast.FunctionExpr)
	param := result.ParamSlots(fn)[0].Symbol
	for _, expression := range fn.Stmts[0].(*ast.ReturnStmt).Exprs {
		base := parsedRuntimeTypeBase(t, expression.(*ast.FuncCallExpr))
		if value, ok := result.RuntimeTypeValue(base); ok || value.Kind != 0 {
			t.Fatalf("shadowed base = %#v/%v, want absent", value, ok)
		}
		if got := mustSymbol(t, result, base); got != param {
			t.Fatalf("shadowed base = %d, want parameter %d", got, param)
		}
	}
}

func TestParsedStaticTypePublicationsAreExactPairs(t *testing.T) {
	stmts, err := parse.ParseString(`
type User = { id: string }
interface Shape end
local value = 1
M.User, M.value, M.Extra = User, value
M.Shape = Shape
M.Bad = Builder.new
`, "static_publications.lua")
	if err != nil {
		t.Fatal(err)
	}
	result := BindChunk(stmts)
	mixed := stmts[3].(*ast.AssignStmt)
	entries := result.StaticTypePublications(mixed)
	if len(entries) != 1 || entries[0].Index != 0 || len(entries[0].Source) != 1 || entries[0].Source[0] != "User" {
		t.Fatalf("mixed publications = %#v, want only M.User", entries)
	}
	if entries := result.StaticTypePublications(stmts[4].(*ast.AssignStmt)); len(entries) != 1 || entries[0].Source[0] != "Shape" {
		t.Fatalf("interface publications = %#v, want M.Shape", entries)
	}
	if entries := result.StaticTypePublications(stmts[5].(*ast.AssignStmt)); entries != nil {
		t.Fatalf("unproven publications = %#v, want nil", entries)
	}
}

func TestParsedRuntimeTypeValuesInStaticQueries(t *testing.T) {
	stmts, err := parse.ParseString(`
local value = "x"
type Direct = typeof(string(value))
type Dot = typeof(string.is(value))
type Index = typeof(string["is"](value))
type Colon = typeof(string:is(value))
local annotated: number @proof(
	string(value),
	string.is(value),
	string["is"](value),
	string:is(value)
) = 0
`, "runtime_type_static.lua")
	if err != nil {
		t.Fatal(err)
	}
	calls := make([]*ast.FuncCallExpr, 0, 8)
	for _, stmt := range stmts[1:5] {
		calls = append(calls, stmt.(*ast.TypeDefStmt).Type.(*ast.TypeOfExpr).Expr.(*ast.FuncCallExpr))
	}
	annotation := stmts[5].(*ast.LocalAssignStmt).Types[0].(*ast.AnnotatedTypeExpr).Annotations[0]
	for _, argument := range annotation.Args {
		calls = append(calls, argument.(*ast.FuncCallExpr))
	}
	result := BindChunk(stmts)
	for i, call := range calls {
		base := parsedRuntimeTypeBase(t, call)
		value, ok := result.RuntimeTypeValue(base)
		if !ok || value.Kind != RuntimeTypeValuePrimitive || value.Name != "string" {
			t.Fatalf("static call %d = %#v/%v, want primitive string", i, value, ok)
		}
	}
}

func parsedRuntimeTypeBase(t testing.TB, call *ast.FuncCallExpr) *ast.IdentExpr {
	t.Helper()
	if call.Receiver != nil {
		base, ok := call.Receiver.(*ast.IdentExpr)
		if !ok {
			t.Fatalf("receiver = %T, want identifier", call.Receiver)
		}
		return base
	}
	switch callee := call.Func.(type) {
	case *ast.IdentExpr:
		return callee
	case *ast.AttrGetExpr:
		base, ok := callee.Object.(*ast.IdentExpr)
		if !ok {
			t.Fatalf("member base = %T, want identifier", callee.Object)
		}
		return base
	default:
		t.Fatalf("callee = %T, want runtime type form", call.Func)
		return nil
	}
}
