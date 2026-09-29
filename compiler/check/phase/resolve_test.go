package phase

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
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

func TestCreateTypeResolutionEngine_NilGraph(t *testing.T) {
	result := CreateTypeResolutionEngine(PhaseEnv{}, nil, nil)
	if result == nil {
		t.Error("expected non-nil engine even with nil graph")
	}
}

func TestCreateTypeResolutionEngine_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	graph := cfg.Build(fn)
	result := CreateTypeResolutionEngine(PhaseEnv{Graph: graph}, nil, nil)
	if result == nil {
		t.Error("expected non-nil engine")
	}
}
