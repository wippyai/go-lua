package extract

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
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

func TestInferCallbackEnvOverlays_NilGraph(t *testing.T) {
	result := inferCallbackEnvOverlays(nil, nil, nil)
	if result != nil {
		t.Error("expected nil result for nil graph")
	}
}

func TestInferCallbackEnvOverlays_EmptyParams(t *testing.T) {
	result := inferCallbackEnvOverlays(nil, []cfg.SymbolID{}, nil)
	if result != nil {
		t.Error("expected nil result for empty params")
	}
}

func TestInferCallbackEnvOverlays_NilSynthExpr(t *testing.T) {
	synthExpr := func(expr ast.Expr, p cfg.Point) typ.Type {
		return nil
	}
	result := inferCallbackEnvOverlays(nil, []cfg.SymbolID{1}, synthExpr)
	if result != nil {
		t.Error("expected nil result for nil graph with params")
	}
}
