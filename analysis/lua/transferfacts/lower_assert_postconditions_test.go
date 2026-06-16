package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLowerDirectAssertTruthyPostcondition(t *testing.T) {
	decl := localAssign([]string{"x"}, &ast.NilExpr{})
	xRead := ident("x")
	stmt := assertStmt(xRead)
	stmts := []ast.Stmt{decl, stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"assert"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	point := requireStmtPoints(t, built, stmt, 1)[0]
	xPath := path.NewPath(mustIdentSymbol(t, bindings, xRead), "x")
	assertLoweredPostconditionRefinement(t, facts, point, xPath, valueRefinementExpectation{
		presence:    presence.Present(),
		hasPresence: true,
	})
}

func TestLowerDirectAssertNotNilPostcondition(t *testing.T) {
	decl := localAssign([]string{"x"}, &ast.NilExpr{})
	xRead := ident("x")
	stmt := assertStmt(&ast.RelationalOpExpr{Operator: "~=", Lhs: xRead, Rhs: &ast.NilExpr{}})
	stmts := []ast.Stmt{decl, stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"assert"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	point := requireStmtPoints(t, built, stmt, 1)[0]
	xPath := path.NewPath(mustIdentSymbol(t, bindings, xRead), "x")
	assertLoweredPostconditionRefinement(t, facts, point, xPath, valueRefinementExpectation{
		presence:    presence.Present(),
		hasPresence: true,
	})
}

func TestLowerDirectAssertTypeEqualPostcondition(t *testing.T) {
	decl := localAssign([]string{"x"}, &ast.NilExpr{})
	xRead := ident("x")
	stmt := assertStmt(&ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      typeCall(xRead),
		Rhs:      stringLit("number"),
	})
	stmts := []ast.Stmt{decl, stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"assert", "type"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	point := requireStmtPoints(t, built, stmt, 1)[0]
	xPath := path.NewPath(mustIdentSymbol(t, bindings, xRead), "x")
	assertLoweredPostconditionRefinement(t, facts, point, xPath, valueRefinementExpectation{
		presence:       presence.Present(),
		hasPresence:    true,
		runtimeKind:    runtimekind.Singleton(runtimekind.Number),
		hasRuntimeKind: true,
	})
}

func TestLowerAssertPostconditionRequiresDirectGlobalStatementCall(t *testing.T) {
	decl := localAssign([]string{"x"}, &ast.NilExpr{})
	shadow := localAssign([]string{"assert"}, ident("other"))
	xRead := ident("x")
	shadowedAssert := assertStmt(xRead)
	receiverCall := &ast.FuncCallStmt{Expr: &ast.FuncCallExpr{
		Receiver: ident("assert"),
		Method:   "not_nil",
		Args:     []ast.Expr{xRead},
	}}
	stmts := []ast.Stmt{decl, shadow, shadowedAssert, receiverCall}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"assert", "other"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	for _, stmt := range []ast.Stmt{shadowedAssert, receiverCall} {
		point := requireStmtPoints(t, built, stmt, 1)[0]
		if got := facts.PostconditionRefinements(point); len(got) != 0 {
			t.Fatalf("postcondition refinements at point %d = %#v, want none", point, got)
		}
	}
}

func assertStmt(cond ast.Expr) *ast.FuncCallStmt {
	return &ast.FuncCallStmt{Expr: &ast.FuncCallExpr{
		Func: ident("assert"),
		Args: []ast.Expr{cond},
	}}
}

func assertLoweredPostconditionRefinement(
	t *testing.T,
	facts factflow.Facts,
	point cfg.Point,
	wantPath path.Path,
	want valueRefinementExpectation,
) {
	t.Helper()
	refinements := facts.PostconditionRefinements(point)
	if len(refinements) != 1 {
		t.Fatalf("postcondition refinements at point %d = %d, want 1", point, len(refinements))
	}
	if !refinements[0].TargetPath().Equal(wantPath) {
		t.Fatalf("postcondition target path = %#v, want %#v", refinements[0].TargetPath(), wantPath)
	}
	assertValueRefinement(t, "postcondition", refinements[0].Value(), want)
}
