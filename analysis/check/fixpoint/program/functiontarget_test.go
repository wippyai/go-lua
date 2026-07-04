package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestCollectNestedTablePathsAndWrappers(t *testing.T) {
	nestedFn := &ast.FunctionExpr{}
	castedFn := &ast.FunctionExpr{}
	assertedFn := &ast.FunctionExpr{}

	localStmt := &ast.LocalAssignStmt{
		Names: []string{"module"},
		Exprs: []ast.Expr{
			&ast.TableExpr{
				Fields: []*ast.Field{
					{
						Key:       &ast.StringExpr{Value: "nested"},
						KeySyntax: ast.AttrKeyDot,
						Value: &ast.TableExpr{
							Fields: []*ast.Field{
								{
									Key:       &ast.StringExpr{Value: "inner"},
									KeySyntax: ast.AttrKeyDot,
									Value:     nestedFn,
								},
								{
									Key:       &ast.StringExpr{Value: "casted"},
									KeySyntax: ast.AttrKeyDot,
									Value: &ast.CastExpr{
										Expr:   castedFn,
										Type:   &ast.PrimitiveTypeExpr{Name: "any"},
										Syntax: ast.CastSyntaxAs,
									},
								},
								{
									Key:       &ast.StringExpr{Value: "asserted"},
									KeySyntax: ast.AttrKeyDot,
									Value:     &ast.NonNilAssertExpr{Expr: assertedFn},
								},
							},
						},
					},
				},
			},
		},
	}
	stmts := []ast.Stmt{localStmt}
	bindings := bind.BindChunk(stmts, bind.Options{})

	got := collectFunctionPathTargets(bindings, stmts)
	if len(got) != 3 {
		t.Fatalf("Collect returned %d targets, want 3: %#v", len(got), got)
	}

	moduleID := mustCollectLocalSymbol(t, bindings, localStmt, 0)
	base := path.NewPath(moduleID, bindings.Name(moduleID))
	assertCollectedPathTarget(t, got, nestedFn, base.Field("nested").Field("inner"))
	assertCollectedPathTarget(t, got, castedFn, base.Field("nested").Field("casted"))
	assertCollectedPathTarget(t, got, assertedFn, base.Field("nested").Field("asserted"))
}

func TestCollectKeysIndexesDottedFunctionDeclarationPath(t *testing.T) {
	stmts, err := parse.ParseString(`
local mapper = {}

function mapper.classify_error(api_error)
	return "invalid_request", api_error.message or "Bad request"
end
`, "functiontarget_test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{})
	targets := collectFunctionPathTargets(bindings, stmts)

	var fn *ast.FunctionExpr
	for _, origin := range bindings.FunctionOrigins() {
		if origin.Kind == bind.FunctionOriginDeclaration && origin.Func != nil {
			fn = origin.Func
			break
		}
	}
	if fn == nil {
		t.Fatalf("dotted function origin missing: %#v", bindings.FunctionOrigins())
	}
	target, ok := targets[fn]
	if !ok || target.String() != "mapper.classify_error" {
		t.Fatalf("target path = %s/%v, want mapper.classify_error/true", target.String(), ok)
	}
	origin, ok := bindings.FunctionOrigin(fn)
	if !ok {
		t.Fatal("FunctionOrigin missing for dotted declaration")
	}
	if fnType, ok := lowerFunctionOriginType(origin, bindings, nil, metatableMethodProof{}); ok || fnType != nil {
		t.Fatalf("lowerFunctionOriginType = %#v/%v, want strict solver metadata to reject untyped declaration", fnType, ok)
	}
	calleeKey, ok := factflow.CalleePathKeyFromPath(target)
	if !ok {
		t.Fatalf("CalleePathKeyFromPath(%s) failed", target.String())
	}
	keys := collectKeys(bindings, rootKey(summary.SummaryKey{}), standard.Registry(), nil, body.Config{}.ModuleExports, stmts)
	if _, ok := keys.pathKeys[calleeKey]; !ok {
		t.Fatalf("path key %s missing from collectKeys", calleeKey.String())
	}
	fnType := functionValueDeclaredType(keys, keys.pathKeys[calleeKey], nil)
	if fnType == nil || len(fnType.Params) != 1 || !typ.IsAny(fnType.Params[0].Type) {
		t.Fatalf("functionValueDeclaredType = %#v, want one any parameter for function-value projection", fnType)
	}
}

func TestCollectScansFunctionOriginsBodies(t *testing.T) {
	innerFn := &ast.FunctionExpr{}
	outerFn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"inner"},
				Exprs: []ast.Expr{innerFn},
			},
		},
	}
	localStmt := &ast.LocalAssignStmt{
		Names: []string{"outer"},
		Exprs: []ast.Expr{outerFn},
	}
	stmts := []ast.Stmt{localStmt}
	bindings := bind.BindChunk(stmts, bind.Options{})

	got := collectFunctionPathTargets(bindings, stmts)
	if len(got) != 2 {
		t.Fatalf("Collect returned %d targets, want 2: %#v", len(got), got)
	}

	outerID := mustCollectLocalSymbol(t, bindings, localStmt, 0)
	base := path.NewPath(outerID, bindings.Name(outerID))
	assertCollectedPathTarget(t, got, outerFn, base)

	innerStmt := outerFn.Stmts[0].(*ast.LocalAssignStmt)
	innerID := mustCollectLocalSymbol(t, bindings, innerStmt, 0)
	assertCollectedPathTarget(t, got, innerFn, path.NewPath(innerID, bindings.Name(innerID)))
}

func assertCollectedPathTarget(t *testing.T, got map[*ast.FunctionExpr]path.Path, fn *ast.FunctionExpr, want path.Path) {
	t.Helper()
	path, ok := got[fn]
	if !ok {
		t.Fatalf("Collect missing target for %p", fn)
	}
	if !path.Equal(want) {
		t.Fatalf("Collect(%p) = %s, want %s", fn, path, want)
	}
}

func mustCollectLocalSymbol(t *testing.T, bindings *bind.Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
	t.Helper()
	locals := bindings.LocalSymbols(stmt)
	if index < 0 || index >= len(locals) {
		t.Fatalf("local symbol index %d out of range", index)
	}
	if locals[index] == 0 {
		t.Fatalf("local symbol index %d is zero", index)
	}
	return locals[index]
}
