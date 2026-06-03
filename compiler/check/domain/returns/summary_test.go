package returns_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/returns"
	"github.com/wippyai/go-lua/compiler/check/domain/trace"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/typ"
)

func TestObservedSummaryPreservesDynamicAnyBranch(t *testing.T) {
	dynamic := &ast.IdentExpr{Value: "inst"}
	nilExpr := &ast.NilExpr{}
	synth := observedSummarySynth{
		types: map[ast.Expr]typ.Type{
			dynamic: typ.Any,
			nilExpr: typ.Nil,
		},
	}

	got := returns.ObservedSummary(nil, []api.ReturnEvidence{
		{Point: 1, Info: &cfg.ReturnInfo{Exprs: []ast.Expr{dynamic}}},
		{Point: 2, Info: &cfg.ReturnInfo{Exprs: []ast.Expr{nilExpr}}},
	}, nil, synth)

	if len(got) != 1 || !typ.IsAny(got[0]) {
		t.Fatalf("ObservedSummary() = %v, want [any]", got)
	}
}

func TestObservedSummaryBareReturnIsNilOutcome(t *testing.T) {
	okExpr := &ast.StringExpr{Value: "ok"}
	got := returns.ObservedSummary(nil, []api.ReturnEvidence{
		{Point: 1, Info: &cfg.ReturnInfo{}},
		{Point: 2, Info: &cfg.ReturnInfo{Exprs: []ast.Expr{okExpr}}},
	}, nil, observedSummarySynth{
		types: map[ast.Expr]typ.Type{okExpr: typ.LiteralString("ok")},
	})

	want := typ.NewOptional(typ.LiteralString("ok"))
	if len(got) != 1 || !typ.TypeEquals(got[0], want) {
		t.Fatalf("ObservedSummary() = %v, want [%v]", got, want)
	}
}

func TestObservedSummaryImplicitSelfReturnStaysPolymorphic(t *testing.T) {
	chunk, err := parse.ParseString(`
		local Query = {}
		function Query:type()
			return self
		end
	`, "implicit_self_return.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := cfg.Build(&ast.FunctionExpr{Stmts: chunk}, "implicit_self_return")
	var methodFn *ast.FunctionExpr
	for _, stmt := range chunk {
		if def, ok := stmt.(*ast.FuncDefStmt); ok && def.Func != nil {
			methodFn = def.Func
			break
		}
	}
	if methodFn == nil {
		t.Fatal("expected method function")
	}
	methodGraph := cfg.BuildWithBindings(methodFn, root.Bindings())
	evidence := trace.GraphEvidence(methodGraph, methodGraph.Bindings())
	staleSelf := typ.NewRecord().SetOpen(true).Build()

	got := returns.ObservedSummary(methodGraph, evidence.Returns, nil, observedSummarySynth{
		types:       map[ast.Expr]typ.Type{},
		defaultType: staleSelf,
	})

	if len(got) != 1 || got[0] != typ.Self {
		t.Fatalf("ObservedSummary() = %v, want [Self]", got)
	}
}

func TestExpandValuesUsesLastExpressionMultiReturnAndPadsNil(t *testing.T) {
	first := &ast.StringExpr{Value: "ok"}
	call := &ast.FuncCallExpr{Func: &ast.IdentExpr{Value: "f"}}

	got := returns.ExpandValues([]ast.Expr{first, call}, 4, 1, observedSummarySynth{
		types: map[ast.Expr]typ.Type{first: typ.String},
		multi: map[ast.Expr][]typ.Type{call: []typ.Type{typ.Integer, typ.Boolean}},
	})

	want := []typ.Type{typ.String, typ.Integer, typ.Boolean, typ.Nil}
	if len(got) != len(want) {
		t.Fatalf("ExpandValues() len = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if !typ.TypeEquals(got[i], want[i]) {
			t.Fatalf("ExpandValues()[%d] = %v, want %v; all=%v", i, got[i], want[i], got)
		}
	}
}

type observedSummarySynth struct {
	types       map[ast.Expr]typ.Type
	multi       map[ast.Expr][]typ.Type
	defaultType typ.Type
}

func (s observedSummarySynth) TypeOf(expr ast.Expr, _ cfg.Point) typ.Type {
	if s.types != nil {
		if t := s.types[expr]; t != nil {
			return t
		}
	}
	if s.defaultType != nil {
		return s.defaultType
	}
	return typ.Unknown
}

func (s observedSummarySynth) TypeOfWithExpected(expr ast.Expr, p cfg.Point, _ typ.Type) typ.Type {
	return s.TypeOf(expr, p)
}

func (s observedSummarySynth) MultiTypeOf(expr ast.Expr, p cfg.Point) []typ.Type {
	if s.multi != nil {
		if values := s.multi[expr]; len(values) > 0 {
			return values
		}
	}
	return []typ.Type{s.TypeOf(expr, p)}
}

func (s observedSummarySynth) FunctionType(*ast.FunctionExpr, *scope.State) *typ.Function {
	return nil
}

func (s observedSummarySynth) ExpandValues(exprs []ast.Expr, needed int, p cfg.Point) []typ.Type {
	values := make([]typ.Type, 0, needed)
	for _, expr := range exprs {
		values = append(values, s.MultiTypeOf(expr, p)...)
		if len(values) >= needed {
			break
		}
	}
	for len(values) < needed {
		values = append(values, typ.Nil)
	}
	return values[:needed]
}

func (s observedSummarySynth) InferIterVars([]ast.Expr, int, cfg.Point) []typ.Type {
	return nil
}

func (s observedSummarySynth) ResolveType(ast.TypeExpr, *scope.State) typ.Type {
	return typ.Unknown
}

func (s observedSummarySynth) ResolveReturnTypes([]ast.TypeExpr, *scope.State) []typ.Type {
	return nil
}
