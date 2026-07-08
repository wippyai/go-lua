package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLowerDirectAssertTruthyPostcondition(t *testing.T) {
	decl := localAssign([]string{"x"}, &ast.NilExpr{})
	xRead := ident("x")
	stmt := assertStmt(xRead)
	stmts := []ast.Stmt{decl, stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"assert"}})
	built := cfgbuild.BuildChunk(stmts, bindings)

	body := wirlower.Lower("assert-postcondition", stmts, bindings, built)
	facts := Lower(built.Graph, Config{Registry: standard.Registry(), WIR: body})
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

	body := wirlower.Lower("assert-postcondition", stmts, bindings, built)
	facts := Lower(built.Graph, Config{Registry: standard.Registry(), WIR: body})
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

	body := wirlower.Lower("assert-postcondition", stmts, bindings, built)
	facts := Lower(built.Graph, Config{Registry: standard.Registry(), WIR: body})
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

	body := wirlower.Lower("assert-postcondition", stmts, bindings, built)
	facts := Lower(built.Graph, Config{Registry: standard.Registry(), WIR: body})
	for _, stmt := range []ast.Stmt{shadowedAssert, receiverCall} {
		point := requireStmtPoints(t, built, stmt, 1)[0]
		if got := facts.PostconditionRefinements(point); len(got) != 0 {
			t.Fatalf("postcondition refinements at point %d = %#v, want none", point, got)
		}
	}
}

func TestLowerAssertPostconditionComesFromWIRInWIRMode(t *testing.T) {
	xDecl := localAssign([]string{"x"}, &ast.NilExpr{})
	yDecl := localAssign([]string{"y"}, &ast.NilExpr{})
	xRead := ident("x")
	stmt := assertStmt(xRead)
	stmts := []ast.Stmt{xDecl, yDecl, stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"assert"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	point := requireStmtPoints(t, built, stmt, 1)[0]
	assertSym, ok := bindings.GlobalSymbol("assert")
	if !ok {
		t.Fatal("missing assert global symbol")
	}
	yPath := path.NewPath(mustLocalAt(t, bindings, yDecl, 0), "y")
	assertPath := path.NewPath(assertSym, "assert")

	body := wir.NewBody("assert-postcondition-wir")
	stampSyntheticWIRPathSymbols(t, body, bindings, assertPath, yPath)
	start := body.Emit(wir.Instruction{
		Op:          wir.OpCall,
		Point:       point,
		CallContext: wir.CallContextStatement,
		Call: wir.CallInfo{
			Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(assertPath))},
		},
		Check: body.InternCheck(wir.Check{Kind: wir.CheckTruthy, Path: yPath}),
	})
	body.SetPointRange(point, start, start+1)

	facts := Lower(built.Graph, Config{Registry: standard.Registry(), WIR: body})
	assertLoweredPostconditionRefinement(t, facts, point, yPath, valueRefinementExpectation{
		presence:    presence.Present(),
		hasPresence: true,
	})
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

func TestLowerAssertPostconditionCarriesStaticWitnessForCapturedOptionsAlias(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
type RetryOptions = {
    retry: {
        max_attempts: number,
        initial_delay: number,
    }?,
}

local captured_options: RetryOptions? = nil
local setter = function(opts: RetryOptions)
    captured_options = opts
end
local options = captured_options
assert(options)
assert(options.retry)
local attempts: number = options.retry.max_attempts
`, "assert")

	optionsStmt, ok := stmts[3].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt 3 = %T, want options local assignment", stmts[3])
	}
	optionsSym, ok := bindings.LocalSymbolAt(optionsStmt, 0)
	if !ok || optionsSym == 0 {
		t.Fatalf("options symbol = %d/%v, want local symbol", optionsSym, ok)
	}
	assertRetry, ok := stmts[5].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt 5 = %T, want assert(options.retry)", stmts[5])
	}

	body := wirlower.Lower("captured-options-assert-postcondition", stmts, bindings, built)
	lowered := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body})
	point := requireStmtPoints(t, built, assertRetry, 1)[0]
	refinements := lowered.Facts.PostconditionRefinements(point)
	if len(refinements) != 1 {
		t.Fatalf("postconditions at assert(options.retry) = %d, want 1: %#v", len(refinements), refinements)
	}
	wantPath := path.NewPath(optionsSym, "options").Field("retry")
	if !refinements[0].TargetPath().Equal(wantPath) {
		t.Fatalf("postcondition target = %s, want %s", refinements[0].TargetPath(), wantPath)
	}
	constraint, ok := refinements[0].Value().Constraint()
	if !ok {
		t.Fatal("assert(options.retry) postcondition missing value constraint")
	}
	gotType, ok := typevalue.TypeOf(standard.Registry(), constraint)
	if !ok {
		t.Fatalf("postcondition constraint has no type witness: %v", constraint)
	}
	wantType := typetable.NewRecord().
		Field("max_attempts", typ.Number).
		Field("initial_delay", typ.Number).
		Build()
	if !typ.TypeEquals(gotType, wantType) {
		t.Fatalf("postcondition constraint type = %v, want %v", gotType, wantType)
	}
}
