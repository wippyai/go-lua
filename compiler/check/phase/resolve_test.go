package phase

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestRunResolve_NilGraph(t *testing.T) {
	input := ResolveInput{
		PhaseEnv: PhaseEnv{Graph: nil},
	}
	output := RunResolve(input)
	if output.TypeResolver != nil {
		t.Error("expected nil TypeResolver for nil graph")
	}
}

func TestRunResolve_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	input := ResolveInput{
		PhaseEnv: PhaseEnv{Graph: graph},
	}
	output := RunResolve(input)
	if output.TypeResolver == nil {
		t.Error("expected non-nil TypeResolver")
	}
}

func TestBuildInitialSymbolTypes_NilGraph(t *testing.T) {
	result := BuildInitialSymbolTypes(nil, nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil graph, got %v", result)
	}
}

func TestBuildInitialSymbolTypes_EmptyTypes(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	result := BuildInitialSymbolTypes(graph, nil, nil)
	if result != nil {
		t.Errorf("expected nil for empty types, got %v", result)
	}
}

func TestBuildInitialSymbolTypes_WithGlobals(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	globals := map[string]typ.Type{"print": typ.Any}
	result := BuildInitialSymbolTypes(graph, globals, nil)
	// Result depends on whether 'print' is visible at any CFG point
	if result == nil {
		t.Skip("print not visible in empty function graph")
	}
}

func TestBuildDeclaredTypesFromSymbolTypes_NilSymbolTypes(t *testing.T) {
	result := BuildDeclaredTypesFromSymbolTypes(nil, nil)
	if result != nil {
		t.Errorf("expected nil for nil symbolTypes, got %v", result)
	}
}

func TestBuildDeclaredTypesFromSymbolTypes_EmptySymbolTypes(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	symbolTypes := make(flow.SymbolTypes)
	result := BuildDeclaredTypesFromSymbolTypes(graph, symbolTypes)
	if result != nil {
		t.Errorf("expected nil for empty symbolTypes, got %v", result)
	}
}

func TestBuildDeclaredTypesFromSymbolTypes_WithTypes(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	entry := graph.Entry()
	symbolTypes := flow.SymbolTypes{
		entry: {
			cfg.SymbolID(1): flow.TypedValue{Type: typ.Number, State: flow.StateResolved},
		},
	}
	result := BuildDeclaredTypesFromSymbolTypes(graph, symbolTypes)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result[cfg.SymbolID(1)] != typ.Number {
		t.Errorf("expected Number for symbol 1, got %v", result[cfg.SymbolID(1)])
	}
}

func TestCreateTypeResolutionEngine_NilGraph(t *testing.T) {
	result := CreateTypeResolutionEngine(nil, nil, nil, nil, nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil engine even with nil graph")
	}
}

func TestCreateTypeResolutionEngine_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	result := CreateTypeResolutionEngine(nil, graph, nil, nil, nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil engine")
	}
}
