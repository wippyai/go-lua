package bind

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/typeindex"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func mustSymbol(t *testing.T, result *Result, ident *ast.IdentExpr) Symbol {
	t.Helper()
	id, ok := result.SymbolOf(ident)
	if !ok || id == 0 {
		t.Fatalf("SymbolOf(%q) = %d/%v", ident.Value, id, ok)
	}
	return id
}

func mustLocalAt(t *testing.T, result *Result, stmt *ast.LocalAssignStmt, index int) Symbol {
	t.Helper()
	id, ok := result.LocalSymbolAt(stmt, index)
	if !ok || id == 0 {
		t.Fatalf("LocalSymbolAt(%d) = %d/%v", index, id, ok)
	}
	return id
}

func mustOrigin(t *testing.T, result *Result, fn *ast.FunctionExpr) FunctionOrigin {
	t.Helper()
	origin, ok := result.FunctionOrigin(fn)
	if !ok {
		t.Fatalf("FunctionOrigin(%p) missing", fn)
	}
	return origin
}

func parseBindSource(t *testing.T, source string) ([]ast.Stmt, *Result) {
	t.Helper()
	stmts, err := parse.ParseString(source, "binder_source.lua")
	if err != nil {
		t.Fatal(err)
	}
	return stmts, BindChunk(stmts, typeindex.Table{})
}

func TestParsedTypedOrdinaryFunctionInitializerIsNotRecursive(t *testing.T) {
	stmts, err := parse.ParseString(`
local typed: () -> number = function(): number
	return typed()
end
`, "typed_recursive_local.lua")
	if err != nil {
		t.Fatal(err)
	}
	definition := stmts[0].(*ast.LocalAssignStmt)
	fn := definition.Exprs[0].(*ast.FunctionExpr)
	call := fn.Stmts[0].(*ast.ReturnStmt).Exprs[0].(*ast.FuncCallExpr)
	self := call.Func.(*ast.IdentExpr)

	result := BindChunk(stmts, typeindex.Table{})
	target := mustLocalAt(t, result, definition, 0)
	got := mustSymbol(t, result, self)
	if got == target {
		t.Fatalf("typed ordinary initializer self-read resolved to new local %d", target)
	}
	if kind, known := result.Kind(got); !known || kind != SymbolGlobal {
		t.Fatalf("typed ordinary initializer self-read kind = %v/%v, want Global", kind, known)
	}
	if annotation, ok := result.SymbolTypeAnnotation(target); !ok || annotation != definition.Types[0] {
		t.Fatalf("typed local annotation = %T/%v, want exact parsed annotation", annotation, ok)
	}
	if origin := mustOrigin(t, result, fn); origin.Stmt != definition || origin.LocalIndex != 0 {
		t.Fatalf("typed function origin = %#v, want local initializer", origin)
	}
}

func TestParsedLexicalScopesSelectExactDeclarations(t *testing.T) {
	stmts, result := parseBindSource(t, `
local value = 1
do
	local value = 2
	local selected = value
end
local deferred = deferred
repeat
	local again = true
until again
`)
	outer := stmts[0].(*ast.LocalAssignStmt)
	block := stmts[1].(*ast.DoBlockStmt)
	inner := block.Stmts[0].(*ast.LocalAssignStmt)
	selected := block.Stmts[1].(*ast.LocalAssignStmt).Exprs[0].(*ast.IdentExpr)
	if got, want := mustSymbol(t, result, selected), mustLocalAt(t, result, inner, 0); got != want {
		t.Fatalf("shadowed value = %d, want inner %d", got, want)
	}
	if outerID := mustLocalAt(t, result, outer, 0); outerID == mustLocalAt(t, result, inner, 0) {
		t.Fatal("shadowed declarations share identity")
	}
	deferred := stmts[2].(*ast.LocalAssignStmt)
	deferredUse := deferred.Exprs[0].(*ast.IdentExpr)
	if got := mustSymbol(t, result, deferredUse); got == mustLocalAt(t, result, deferred, 0) {
		t.Fatal("ordinary local initializer saw its pending declaration")
	}
	repeat := stmts[3].(*ast.RepeatStmt)
	again := repeat.Stmts[0].(*ast.LocalAssignStmt)
	if got, want := mustSymbol(t, result, repeat.Condition.(*ast.IdentExpr)), mustLocalAt(t, result, again, 0); got != want {
		t.Fatalf("repeat condition = %d, want body local %d", got, want)
	}
}

func TestParsedFunctionsRetainIdentityOriginsParametersAndCaptures(t *testing.T) {
	stmts, result := parseBindSource(t, `
local outer = function(parameter: number, ...: string): number
	local value = 1
	local child = function()
		return parameter, value, global_name
	end
	return child()
end
`)
	outerDecl := stmts[0].(*ast.LocalAssignStmt)
	outer := outerDecl.Exprs[0].(*ast.FunctionExpr)
	valueDecl := outer.Stmts[0].(*ast.LocalAssignStmt)
	childDecl := outer.Stmts[1].(*ast.LocalAssignStmt)
	child := childDecl.Exprs[0].(*ast.FunctionExpr)
	returns := child.Stmts[0].(*ast.ReturnStmt).Exprs
	params := result.ParamSlots(outer)
	if len(params) != 2 || params[0].Name != "parameter" || !params[1].Vararg {
		t.Fatalf("ParamSlots = %#v, want typed parameter and vararg", params)
	}
	if annotation, ok := result.SymbolTypeAnnotation(params[0].Symbol); !ok || annotation != outer.ParList.Types[0] {
		t.Fatalf("parameter annotation = %T/%v", annotation, ok)
	}
	if annotation, ok := result.SymbolTypeAnnotation(params[1].Symbol); !ok || annotation != outer.ParList.VarargType {
		t.Fatalf("vararg annotation = %T/%v", annotation, ok)
	}
	var captures []Capture
	result.ForEachEntryCapture(func(fn *ast.FunctionExpr, capture Capture) bool {
		if fn == child {
			captures = append(captures, capture)
		}
		return true
	})
	if len(captures) != 2 ||
		captures[0].Captured != params[0].Symbol ||
		captures[1].Captured != mustLocalAt(t, result, valueDecl, 0) {
		t.Fatalf("DirectCaptures = %#v, want parameter then value", captures)
	}
	if got := mustSymbol(t, result, returns[0].(*ast.IdentExpr)); got != params[0].Symbol {
		t.Fatalf("captured parameter = %d, want %d", got, params[0].Symbol)
	}
	global := mustSymbol(t, result, returns[2].(*ast.IdentExpr))
	if kind, _ := result.Kind(global); kind != SymbolGlobal {
		t.Fatalf("global kind = %v", kind)
	}
	outerOrigin := mustOrigin(t, result, outer)
	childOrigin := mustOrigin(t, result, child)
	if outerOrigin.Parent != nil || childOrigin.Parent != outer ||
		outerOrigin.Stmt != outerDecl || outerOrigin.LocalIndex != 0 ||
		childOrigin.Stmt != childDecl || childOrigin.LocalIndex != 0 {
		t.Fatalf("origins = %#v / %#v", outerOrigin, childOrigin)
	}
}

func TestParsedFunctionDeclarationAndMethodSelfAuthority(t *testing.T) {
	stmts, result := parseBindSource(t, `
function global_fn() return global_fn end
local local_fn
function local_fn() return local_fn end
function module:method(value) return self, value end
`)
	globalStmt := stmts[0].(*ast.FuncDefStmt)
	localDecl := stmts[1].(*ast.LocalAssignStmt)
	localStmt := stmts[2].(*ast.FuncDefStmt)
	method := stmts[3].(*ast.FuncDefStmt).Func
	global := mustSymbol(t, result, globalStmt.Name.Func.(*ast.IdentExpr))
	if kind, _ := result.Kind(global); kind != SymbolGlobal {
		t.Fatalf("global target kind = %v", kind)
	}
	if local := mustSymbol(t, result, localStmt.Name.Func.(*ast.IdentExpr)); local != mustLocalAt(t, result, localDecl, 0) {
		t.Fatalf("local function target = %d", local)
	}
	slots := result.ParamSlots(method)
	if len(slots) != 2 || !slots[0].ImplicitSelf || slots[0].Name != "self" || slots[1].Name != "value" {
		t.Fatalf("method slots = %#v", slots)
	}
}

func TestParsedTypeQueryFunctionsRetainStaticStructure(t *testing.T) {
	stmts, result := parseBindSource(t, `
local outer = 1
type Snapshot = typeof(function<T: typeof(outer)>(value: T): T
	return value
end)
`)
	fn := stmts[1].(*ast.TypeDefStmt).Type.(*ast.TypeOfExpr).Expr.(*ast.FunctionExpr)
	origin := mustOrigin(t, result, fn)
	if !origin.Static {
		t.Fatalf("origin = %#v, want static", origin)
	}
	params := result.ParamSlots(fn)
	body := fn.Stmts[0].(*ast.ReturnStmt).Exprs[0].(*ast.IdentExpr)
	if len(params) != 1 || mustSymbol(t, result, body) != params[0].Symbol {
		t.Fatalf("static function parameters = %#v", params)
	}
	if typeParams := result.FunctionTypeParams(fn); len(typeParams) != 1 || typeParams[0].Name != "T" {
		t.Fatalf("static type parameters = %#v", typeParams)
	}
}
