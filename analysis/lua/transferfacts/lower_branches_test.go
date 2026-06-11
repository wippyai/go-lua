package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLowerIdentifierNilTruthyFalsyBranches(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	nilRead := ident("x")
	nilStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{Operator: "==", Lhs: nilRead, Rhs: &ast.NilExpr{}}}
	notNilRead := ident("x")
	notNilStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{Operator: "~=", Lhs: notNilRead, Rhs: &ast.NilExpr{}}}
	truthyRead := ident("x")
	truthyStmt := &ast.IfStmt{Condition: truthyRead}
	falsyRead := ident("x")
	falsyStmt := &ast.IfStmt{Condition: &ast.UnaryNotOpExpr{Expr: falsyRead}}
	stmts := []ast.Stmt{decl, nilStmt, notNilStmt, truthyStmt, falsyStmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	xPath := path.NewPath(mustIdentSymbol(t, bindings, nilRead), "x")
	assertLoweredBranchValuePresence(t, facts, requireStmtPoints(t, built, nilStmt, 1)[0], xPath, presence.Absent(), true, presence.Present(), true)
	assertLoweredBranchValuePresence(t, facts, requireStmtPoints(t, built, notNilStmt, 1)[0], xPath, presence.Present(), true, presence.Absent(), true)
	assertLoweredBranchValuePresence(t, facts, requireStmtPoints(t, built, truthyStmt, 1)[0], xPath, presence.Present(), true, presence.Bottom(), false)
	assertLoweredBranchValuePresence(t, facts, requireStmtPoints(t, built, falsyStmt, 1)[0], xPath, presence.Bottom(), false, presence.Present(), true)
}

func TestLowerMemberPathBranchRefinement(t *testing.T) {
	decl := localAssign([]string{"t"}, &ast.TableExpr{})
	rootRead := ident("t")
	memberStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{Operator: "~=", Lhs: dot(rootRead, "child"), Rhs: &ast.NilExpr{}}}
	stmts := []ast.Stmt{decl, memberStmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	wantPath := path.NewPath(mustIdentSymbol(t, bindings, rootRead), "t").Field("child")
	assertLoweredBranchValuePresence(t, facts, requireStmtPoints(t, built, memberStmt, 1)[0], wantPath, presence.Present(), true, presence.Absent(), true)
}

func TestLowerTypeGuardTableEqualityBranchRefinement(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	xRead := ident("x")
	typeStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      typeCall(xRead),
		Rhs:      stringLit("table"),
	}}
	stmts := []ast.Stmt{decl, typeStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	point := requireStmtPoints(t, built, typeStmt, 1)[0]
	xPath := path.NewPath(mustIdentSymbol(t, bindings, xRead), "x")
	assertLoweredBranchValueRefinement(t, facts, point, xPath,
		valueRefinementExpectation{
			presence:       presence.Present(),
			hasPresence:    true,
			runtimeKind:    runtimekind.Singleton(runtimekind.Table),
			hasRuntimeKind: true,
		},
		valueRefinementExpectation{
			runtimeKind:    runtimekind.Top().Without(runtimekind.Table),
			hasRuntimeKind: true,
		},
	)
}

func TestLowerTypeGuardFunctionInequalityBranchRefinement(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	xRead := ident("x")
	typeStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "~=",
		Lhs:      typeCall(xRead),
		Rhs:      stringLit("function"),
	}}
	stmts := []ast.Stmt{decl, typeStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	point := requireStmtPoints(t, built, typeStmt, 1)[0]
	xPath := path.NewPath(mustIdentSymbol(t, bindings, xRead), "x")
	assertLoweredBranchValueRefinement(t, facts, point, xPath,
		valueRefinementExpectation{
			runtimeKind:    runtimekind.Top().Without(runtimekind.Function),
			hasRuntimeKind: true,
		},
		valueRefinementExpectation{
			presence:       presence.Present(),
			hasPresence:    true,
			runtimeKind:    runtimekind.Singleton(runtimekind.Function),
			hasRuntimeKind: true,
		},
	)
}

func TestLowerTypeGuardNilBranchRefinements(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	eqRead := ident("x")
	eqStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      typeCall(eqRead),
		Rhs:      stringLit("nil"),
	}}
	notRead := ident("x")
	notStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "~=",
		Lhs:      typeCall(notRead),
		Rhs:      stringLit("nil"),
	}}
	stmts := []ast.Stmt{decl, eqStmt, notStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	xPath := path.NewPath(mustIdentSymbol(t, bindings, eqRead), "x")
	nilValue := valueRefinementExpectation{
		presence:       presence.Absent(),
		hasPresence:    true,
		runtimeKind:    runtimekind.Singleton(runtimekind.Nil),
		hasRuntimeKind: true,
	}
	notNilValue := valueRefinementExpectation{
		presence:       presence.Present(),
		hasPresence:    true,
		runtimeKind:    runtimekind.Top().Without(runtimekind.Nil),
		hasRuntimeKind: true,
	}
	assertLoweredBranchValueRefinement(t, facts, requireStmtPoints(t, built, eqStmt, 1)[0], xPath, nilValue, notNilValue)
	assertLoweredBranchValueRefinement(t, facts, requireStmtPoints(t, built, notStmt, 1)[0], xPath, notNilValue, nilValue)
}

func TestLowerTypeGuardRuntimeTypeNames(t *testing.T) {
	l := lowerer{registry: product.DefaultRegistry()}
	target := path.NewPath(symbol.ID(1), "x")
	tests := []struct {
		typeName string
		tag      runtimekind.Tag
	}{
		{"nil", runtimekind.Nil},
		{"boolean", runtimekind.Boolean},
		{"number", runtimekind.Number},
		{"string", runtimekind.String},
		{"table", runtimekind.Table},
		{"function", runtimekind.Function},
		{"thread", runtimekind.Thread},
		{"userdata", runtimekind.Userdata},
	}

	for _, tt := range tests {
		refinement, ok := l.typeBranchRefinement(target, branchcond.CheckTypeEqual, tt.typeName)
		if !ok {
			t.Fatalf("typeBranchRefinement(%q) returned false", tt.typeName)
		}
		trueValue, ok := refinement.TrueValue()
		if !ok {
			t.Fatalf("typeBranchRefinement(%q) missing true-edge refinement", tt.typeName)
		}
		falseValue, ok := refinement.FalseValue()
		if !ok {
			t.Fatalf("typeBranchRefinement(%q) missing false-edge refinement", tt.typeName)
		}

		truePresence := presence.Present()
		falsePresence := presence.Top()
		falseHasPresence := false
		if tt.tag == runtimekind.Nil {
			truePresence = presence.Absent()
			falsePresence = presence.Present()
			falseHasPresence = true
		}
		assertValueRefinement(t, tt.typeName+" true edge", trueValue, valueRefinementExpectation{
			presence:       truePresence,
			hasPresence:    true,
			runtimeKind:    runtimekind.Singleton(tt.tag),
			hasRuntimeKind: true,
		})
		assertValueRefinement(t, tt.typeName+" false edge", falseValue, valueRefinementExpectation{
			presence:       falsePresence,
			hasPresence:    falseHasPresence,
			runtimeKind:    runtimekind.Top().Without(tt.tag),
			hasRuntimeKind: true,
		})
	}
}

func TestLowerTypeGuardReversedOperandsBranchRefinement(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	xRead := ident("x")
	typeStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      stringLit("table"),
		Rhs:      typeCall(xRead),
	}}
	stmts := []ast.Stmt{decl, typeStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	point := requireStmtPoints(t, built, typeStmt, 1)[0]
	xPath := path.NewPath(mustIdentSymbol(t, bindings, xRead), "x")
	assertLoweredBranchValueRefinement(t, facts, point, xPath,
		valueRefinementExpectation{
			presence:       presence.Present(),
			hasPresence:    true,
			runtimeKind:    runtimekind.Singleton(runtimekind.Table),
			hasRuntimeKind: true,
		},
		valueRefinementExpectation{
			runtimeKind:    runtimekind.Top().Without(runtimekind.Table),
			hasRuntimeKind: true,
		},
	)
}

func TestLowerSkipsUnknownTypeGuardBranchRefinements(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	typeStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      typeCall(ident("x")),
		Rhs:      stringLit("mystery"),
	}}
	stmts := []ast.Stmt{decl, typeStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, product.DefaultRegistry())
	point := requireStmtPoints(t, built, typeStmt, 1)[0]
	if _, ok := facts.BranchRefinement(point); ok {
		t.Fatalf("unknown type guard branch point %d lowered as branch refinement", point)
	}
}
