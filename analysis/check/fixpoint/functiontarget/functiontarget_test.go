package functiontarget

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
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

	got := Collect(bindings, stmts)
	if len(got) != 3 {
		t.Fatalf("Collect returned %d targets, want 3: %#v", len(got), got)
	}

	moduleID := mustLocalSymbol(t, bindings, localStmt, 0)
	base := path.NewPath(moduleID, bindings.Name(moduleID))
	assertPathTarget(t, got, nestedFn, base.Field("nested").Field("inner"))
	assertPathTarget(t, got, castedFn, base.Field("nested").Field("casted"))
	assertPathTarget(t, got, assertedFn, base.Field("nested").Field("asserted"))
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

	got := Collect(bindings, stmts)
	if len(got) != 2 {
		t.Fatalf("Collect returned %d targets, want 2: %#v", len(got), got)
	}

	outerID := mustLocalSymbol(t, bindings, localStmt, 0)
	base := path.NewPath(outerID, bindings.Name(outerID))
	assertPathTarget(t, got, outerFn, base)

	innerStmt := outerFn.Stmts[0].(*ast.LocalAssignStmt)
	innerID := mustLocalSymbol(t, bindings, innerStmt, 0)
	assertPathTarget(t, got, innerFn, path.NewPath(innerID, bindings.Name(innerID)))
}

func assertPathTarget(t *testing.T, got map[*ast.FunctionExpr]path.Path, fn *ast.FunctionExpr, want path.Path) {
	t.Helper()
	path, ok := got[fn]
	if !ok {
		t.Fatalf("Collect missing target for %p", fn)
	}
	if !path.Equal(want) {
		t.Fatalf("Collect(%p) = %s, want %s", fn, path, want)
	}
}

func mustLocalSymbol(t *testing.T, bindings *bind.Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
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
