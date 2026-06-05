package summary_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestReferencePathProjectionForGraphSeparatesCallableReadsAndEscapes(t *testing.T) {
	mUsed := attr(ident("M"), "used")
	mForwarded := attr(ident("M"), "forwarded")
	mReturned := attr(ident("M"), "returned")
	mAlias := attr(ident("M"), "alias")
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{
		&ast.FuncCallStmt{Expr: &ast.FuncCallExpr{Func: mUsed}},
		&ast.FuncCallStmt{Expr: &ast.FuncCallExpr{Func: ident("sink"), Args: []ast.Expr{mForwarded}}},
		&ast.LocalAssignStmt{Names: []string{"x"}, Exprs: []ast.Expr{mAlias}},
		&ast.ReturnStmt{Exprs: []ast.Expr{mReturned}},
	}}
	graph := cfg.Build(fn)
	mSym, ok := graph.GlobalSymbol("M")
	if !ok {
		t.Fatal("M global symbol not found")
	}
	sinkSym, ok := graph.GlobalSymbol("sink")
	if !ok {
		t.Fatal("sink global symbol not found")
	}

	projection := summary.ReferencePathProjectionForGraph(graph)

	if !hasPath(projection.Exact, constraint.NewPath(mSym, "M").Field("used")) {
		t.Fatalf("exact paths = %v, want M.used", projection.Exact)
	}
	if !hasPath(projection.Exact, constraint.NewPath(sinkSym, "sink")) {
		t.Fatalf("exact paths = %v, want sink callee", projection.Exact)
	}
	for _, path := range []constraint.Path{
		constraint.NewPath(mSym, "M").Field("forwarded"),
		constraint.NewPath(mSym, "M").Field("returned"),
		constraint.NewPath(mSym, "M").Field("alias"),
	} {
		if !hasPath(projection.Subtrees, path) {
			t.Fatalf("subtree paths = %v, want %s", projection.Subtrees, path.String())
		}
	}
	if hasPath(projection.Subtrees, constraint.NewPath(mSym, "M")) {
		t.Fatalf("subtree paths = %v, did not expect whole M root", projection.Subtrees)
	}
	if !hasPath(projection.Exact, constraint.NewPath(mSym, "M")) {
		t.Fatalf("exact paths = %v, want M owner surface for callable member read", projection.Exact)
	}
	if hasPath(projection.Exact, constraint.NewPath(mSym, "M").Field("unused")) ||
		hasPath(projection.Subtrees, constraint.NewPath(mSym, "M").Field("unused")) {
		t.Fatalf("projection retained unused path: exact=%v subtrees=%v", projection.Exact, projection.Subtrees)
	}
}

func TestReferencePathProjectionForGraphDoesNotPromoteStaticDataReadToRoot(t *testing.T) {
	opType := attr(attr(ident("M"), "OP_TYPES"), "WITH_INPUT")
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{
		&ast.IfStmt{
			Condition: &ast.RelationalOpExpr{Lhs: opType, Operator: "==", Rhs: ident("value")},
		},
	}}
	graph := cfg.Build(fn)
	mSym, ok := graph.GlobalSymbol("M")
	if !ok {
		t.Fatal("M global symbol not found")
	}

	projection := summary.ReferencePathProjectionForGraph(graph)

	if hasPath(projection.Exact, constraint.NewPath(mSym, "M")) {
		t.Fatalf("exact paths = %v, did not expect whole M root from static data read", projection.Exact)
	}
	if !hasPath(projection.Exact, constraint.NewPath(mSym, "M").Field("OP_TYPES").Field("WITH_INPUT")) {
		t.Fatalf("exact paths = %v, want nested OP_TYPES.WITH_INPUT", projection.Exact)
	}
}

func ident(name string) *ast.IdentExpr {
	return &ast.IdentExpr{Value: name}
}

func attr(obj ast.Expr, name string) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       ident(name),
		KeySyntax: ast.AttrKeyDot,
	}
}

func hasPath(paths []constraint.Path, want constraint.Path) bool {
	for _, path := range paths {
		if path.Equal(want) {
			return true
		}
	}
	return false
}
