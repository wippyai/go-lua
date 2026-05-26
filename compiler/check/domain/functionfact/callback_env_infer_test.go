package functionfact

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/trace"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/typ"
)

func TestGlobalSetup(t *testing.T) {
	gs := globalSetup{
		point: cfg.Point(1),
		name:  "testGlobal",
		expr:  nil,
	}
	if gs.name != "testGlobal" {
		t.Errorf("expected name 'testGlobal', got %q", gs.name)
	}
	if gs.point != 1 {
		t.Errorf("expected point 1, got %d", gs.point)
	}
}

func TestGlobalClear(t *testing.T) {
	gc := globalClear{
		point: cfg.Point(3),
		name:  "clearGlobal",
	}
	if gc.name != "clearGlobal" {
		t.Errorf("expected name 'clearGlobal', got %q", gc.name)
	}
}

func TestParamCall(t *testing.T) {
	pc := paramCall{
		point:      cfg.Point(5),
		paramIndex: 2,
	}
	if pc.paramIndex != 2 {
		t.Errorf("expected paramIndex 2, got %d", pc.paramIndex)
	}
}

func evidenceForGraph(graph *cfg.Graph) api.FlowEvidence {
	if graph == nil {
		return api.FlowEvidence{}
	}
	return trace.GraphEvidence(graph, graph.Bindings())
}

func TestInferCallbackEnvOverlays_NilGraph(t *testing.T) {
	result := InferCallbackEnvOverlays(nil, api.FlowEvidence{}, nil, nil, nil)
	if result != nil {
		t.Error("expected nil result for nil graph")
	}
}

func TestInferCallbackEnvOverlays_EmptyParams(t *testing.T) {
	result := InferCallbackEnvOverlays(nil, api.FlowEvidence{}, []cfg.ParamSlot{}, nil, nil)
	if result != nil {
		t.Error("expected nil result for empty params")
	}
}

func TestInferCallbackEnvOverlays_NilSynthExpr(t *testing.T) {
	synthExpr := func(expr ast.Expr, p cfg.Point) typ.Type {
		return nil
	}
	result := InferCallbackEnvOverlays(nil, api.FlowEvidence{}, []cfg.ParamSlot{
		{
			Symbol:      cfg.SymbolID(1),
			SourceIndex: 0,
		},
	}, synthExpr, nil)
	if result != nil {
		t.Error("expected nil result for nil graph with params")
	}
}

func TestInferCallbackEnvOverlays_AssignmentCallSite(t *testing.T) {
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

	result := InferCallbackEnvOverlays(graph, evidenceForGraph(graph), paramSlots, synthExpr, nil)
	if result == nil {
		t.Fatal("expected callback overlay result")
	}
	env := result[0]
	if env == nil {
		t.Fatal("expected overlay for first parameter")
	}
	if got := env["ctx"]; got == nil || !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("expected ctx overlay integer, got %v", got)
	}
}

func TestInferCallbackEnvOverlays_UsesCallEvidenceReturnForSetupCall(t *testing.T) {
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

	result := InferCallbackEnvOverlays(graph, evidence, graph.ParamSlots(), func(ast.Expr, cfg.Point) typ.Type {
		return typ.Unknown
	}, nil)
	if result == nil {
		t.Fatal("expected callback overlay result")
	}
	if got := result[0]["ctx"]; !typ.TypeEquals(got, typ.String) {
		t.Fatalf("expected ctx overlay from setup call return, got %v", got)
	}
}

func TestInferCallbackEnvOverlaysFromSources_ReturnedClosureCallsOuterParam(t *testing.T) {
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
	result := InferCallbackEnvOverlaysFromSources([]CallbackEnvOverlaySource{
		{Graph: graph, Evidence: evidenceForGraph(graph), SynthExpr: synthExpr},
		{Graph: childGraph, Evidence: evidenceForGraph(childGraph), SynthExpr: synthExpr},
	}, graph.ParamSlots(), moduleBindings)
	if result == nil {
		t.Fatal("expected callback overlay result")
	}
	if got := result[0]["ctx"]; !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("expected ctx overlay integer, got %v", got)
	}
}

func TestInferCallbackEnvOverlays_UsesCanonicalCandidatesWhenRawCallSymbolMissing(t *testing.T) {
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

	result := InferCallbackEnvOverlays(graph, evidenceForGraph(graph), paramSlots, synthExpr, nil)
	if result == nil {
		t.Fatal("expected callback overlay result")
	}
	env := result[0]
	if env == nil {
		t.Fatal("expected overlay for first parameter")
	}
	if got := env["ctx"]; got == nil || !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("expected ctx overlay integer, got %v", got)
	}
}

func TestInferCallbackEnvOverlays_UsesModuleBindingNameResolution(t *testing.T) {
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

	result := InferCallbackEnvOverlays(graph, evidenceForGraph(graph), paramSlots, synthExpr, moduleBindings)
	if result == nil {
		t.Fatal("expected callback overlay result")
	}
	env := result[0]
	if env == nil {
		t.Fatal("expected overlay for first parameter")
	}
	if got := env["ctx"]; got == nil || !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("expected ctx overlay integer, got %v", got)
	}
}

func TestInferCallbackEnvOverlays_UsesDirectAliasCandidate(t *testing.T) {
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

	result := InferCallbackEnvOverlays(graph, evidenceForGraph(graph), paramSlots, synthExpr, nil)
	if result == nil {
		t.Fatal("expected callback overlay result")
	}
	env := result[0]
	if env == nil {
		t.Fatal("expected overlay for first parameter")
	}
	if got := env["ctx"]; got == nil || !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("expected ctx overlay integer, got %v", got)
	}
}
