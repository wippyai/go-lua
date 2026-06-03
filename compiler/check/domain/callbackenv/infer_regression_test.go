package callbackenv

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/trace"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/typ"
)

func evidenceForGraph(graph *cfg.Graph) api.FlowEvidence {
	if graph == nil {
		return api.FlowEvidence{}
	}
	return trace.GraphEvidence(graph, graph.Bindings())
}

func requireOverlayType(t *testing.T, result Overlays, paramIdx int, name string) typ.Type {
	t.Helper()
	if result == nil {
		t.Fatal("expected callback overlay result")
	}
	env, ok := result.ForParam(paramIdx)
	if !ok || len(env) == 0 {
		t.Fatalf("expected overlay for parameter %d: %+v", paramIdx, result)
	}
	got, ok := env.Type(name)
	if !ok || got == nil {
		t.Fatalf("expected %s overlay in %+v", name, env)
	}
	return got
}

func TestInfer_NilGraph(t *testing.T) {
	result := Infer(nil, api.FlowEvidence{}, nil, nil, nil)
	if result != nil {
		t.Error("expected nil result for nil graph")
	}
}

func TestInfer_EmptyParams(t *testing.T) {
	result := Infer(nil, api.FlowEvidence{}, []cfg.ParamSlot{}, nil, nil)
	if result != nil {
		t.Error("expected nil result for empty params")
	}
}

func TestInfer_NilSynthExpr(t *testing.T) {
	synthExpr := func(expr ast.Expr, p cfg.Point) typ.Type {
		return nil
	}
	result := Infer(nil, api.FlowEvidence{}, []cfg.ParamSlot{
		{
			Symbol:      cfg.SymbolID(1),
			SourceIndex: 0,
		},
	}, synthExpr, nil)
	if result != nil {
		t.Error("expected nil result for nil graph with params")
	}
}

func TestInfer_AssignmentCallSite(t *testing.T) {
	code := `
		_G.ctx = 1
		local x = cb()
		_G.ctx = nil
	`
	stmts, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"cb"},
		},
		Stmts: stmts,
	}
	graph := cfg.Build(fn, "_G")
	if graph == nil {
		t.Fatal("expected graph")
	}
	paramSlots := graph.ParamSlots()
	if len(paramSlots) == 0 {
		t.Fatal("expected param slots")
	}

	synthExpr := func(expr ast.Expr, _ cfg.Point) typ.Type {
		if _, ok := expr.(*ast.NumberExpr); ok {
			return typ.Integer
		}
		return typ.Unknown
	}

	result := Infer(graph, evidenceForGraph(graph), paramSlots, synthExpr, nil)
	if got := requireOverlayType(t, result, 0, "ctx"); !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("expected ctx overlay integer, got %v", got)
	}
}

func TestInfer_UsesCallEvidenceReturnForSetupCall(t *testing.T) {
	code := `
		local function make_ctx() end
		_G.ctx = make_ctx()
		cb()
		_G.ctx = nil
	`
	stmts, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"cb"},
		},
		Stmts: stmts,
	}
	graph := cfg.Build(fn, "_G")
	if graph == nil {
		t.Fatal("expected graph")
	}
	evidence := evidenceForGraph(graph)
	for idx := range evidence.Calls {
		info := evidence.Calls[idx].Info
		if info != nil && info.CalleeName == "make_ctx" {
			evidence.Calls[idx].CalleeType = typ.Func().Returns(typ.String).Build()
		}
	}

	result := Infer(graph, evidence, graph.ParamSlots(), func(ast.Expr, cfg.Point) typ.Type {
		return typ.Unknown
	}, nil)
	if got := requireOverlayType(t, result, 0, "ctx"); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("expected ctx overlay from setup call return, got %v", got)
	}
}

func TestInferFromSources_ReturnedClosureCallsOuterParam(t *testing.T) {
	code := `
		return function()
			_G.ctx = 1
			cb()
			_G.ctx = nil
		end
	`
	stmts, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"cb"},
		},
		Stmts: stmts,
	}
	graph := cfg.Build(fn)
	if graph == nil {
		t.Fatal("expected graph")
	}
	moduleBindings := graph.Bindings()
	var returned *ast.FunctionExpr
	graph.EachReturn(func(_ cfg.Point, info *cfg.ReturnInfo) {
		if info == nil || len(info.Exprs) == 0 || returned != nil {
			return
		}
		returned, _ = info.Exprs[0].(*ast.FunctionExpr)
	})
	if returned == nil {
		t.Fatal("expected returned closure")
	}
	childGraph := cfg.BuildWithBindings(returned, moduleBindings)
	if childGraph == nil {
		t.Fatal("expected child graph")
	}

	synthExpr := func(expr ast.Expr, _ cfg.Point) typ.Type {
		if _, ok := expr.(*ast.NumberExpr); ok {
			return typ.Integer
		}
		return typ.Unknown
	}
	result := InferFromSources([]Source{
		{Graph: graph, Evidence: evidenceForGraph(graph), SynthExpr: synthExpr},
		{Graph: childGraph, Evidence: evidenceForGraph(childGraph), SynthExpr: synthExpr},
	}, graph.ParamSlots(), moduleBindings)
	if got := requireOverlayType(t, result, 0, "ctx"); !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("expected ctx overlay integer, got %v", got)
	}
}

func TestInfer_UsesCanonicalCandidatesWhenRawCallSymbolMissing(t *testing.T) {
	code := `
		_G.ctx = 1
		local x = cb()
		_G.ctx = nil
	`
	stmts, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"cb"},
		},
		Stmts: stmts,
	}
	graph := cfg.Build(fn, "_G")
	if graph == nil {
		t.Fatal("expected graph")
	}
	paramSlots := graph.ParamSlots()
	if len(paramSlots) == 0 {
		t.Fatal("expected param slots")
	}

	// Simulate missing raw call symbol at call site; callback detection should
	// still resolve cb via canonical candidates from callee expression/bindings.
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if info != nil {
			info.CalleeSymbol = 0
		}
	})

	synthExpr := func(expr ast.Expr, _ cfg.Point) typ.Type {
		if _, ok := expr.(*ast.NumberExpr); ok {
			return typ.Integer
		}
		return typ.Unknown
	}

	result := Infer(graph, evidenceForGraph(graph), paramSlots, synthExpr, nil)
	if got := requireOverlayType(t, result, 0, "ctx"); !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("expected ctx overlay integer, got %v", got)
	}
}

func TestInfer_UsesModuleBindingNameResolution(t *testing.T) {
	code := `
		_G.ctx = 1
		local x = cb()
		_G.ctx = nil
	`
	stmts, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"cb"},
		},
		Stmts: stmts,
	}
	graph := cfg.Build(fn, "_G")
	if graph == nil {
		t.Fatal("expected graph")
	}
	paramSlots := graph.ParamSlots()
	if len(paramSlots) == 0 || paramSlots[0].Symbol == 0 {
		t.Fatal("expected param slots")
	}

	moduleBindings := bind.NewBindingTable()
	moduleBindings.SetName(paramSlots[0].Symbol, "cb_alias")

	// Force callback identity recovery through module-binding name resolution.
	graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
		if info != nil {
			info.CalleeSymbol = 0
			info.CalleeName = "cb_alias"
			info.Callee = &ast.IdentExpr{Value: "cb_alias"}
		}
	})

	synthExpr := func(expr ast.Expr, _ cfg.Point) typ.Type {
		if _, ok := expr.(*ast.NumberExpr); ok {
			return typ.Integer
		}
		return typ.Unknown
	}

	result := Infer(graph, evidenceForGraph(graph), paramSlots, synthExpr, moduleBindings)
	if got := requireOverlayType(t, result, 0, "ctx"); !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("expected ctx overlay integer, got %v", got)
	}
}

func TestInfer_UsesDirectAliasCandidate(t *testing.T) {
	code := `
		local f = cb
		_G.ctx = 1
		local x = f()
		_G.ctx = nil
	`
	stmts, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"cb"},
		},
		Stmts: stmts,
	}
	graph := cfg.Build(fn, "_G")
	if graph == nil {
		t.Fatal("expected graph")
	}
	paramSlots := graph.ParamSlots()
	if len(paramSlots) == 0 {
		t.Fatal("expected param slots")
	}

	synthExpr := func(expr ast.Expr, _ cfg.Point) typ.Type {
		if _, ok := expr.(*ast.NumberExpr); ok {
			return typ.Integer
		}
		return typ.Unknown
	}

	result := Infer(graph, evidenceForGraph(graph), paramSlots, synthExpr, nil)
	if got := requireOverlayType(t, result, 0, "ctx"); !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("expected ctx overlay integer, got %v", got)
	}
}
