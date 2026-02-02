package assign

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

func buildEmptyGraph() *cfg.Graph {
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	return cfg.Build(fn)
}

func TestCollectSpecNarrowedTypes_NilGraph(t *testing.T) {
	result := CollectSpecNarrowedTypes(nil, nil, nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil result for nil graph")
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for nil graph, got %d entries", len(result))
	}
}

func TestCollectSpecNarrowedTypes_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	graph := cfg.Build(fn)
	scopes := make(map[cfg.Point]*scope.State)
	result := CollectSpecNarrowedTypes(graph, scopes, nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestBuildReceiverDependencies_NilGraph(t *testing.T) {
	result := BuildReceiverDependencies(nil)
	if result == nil {
		t.Error("expected non-nil result for nil graph")
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for nil graph, got %d entries", len(result))
	}
}

func TestBuildReceiverDependencies_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	graph := cfg.Build(fn)
	result := BuildReceiverDependencies(graph)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestSortPoints_Empty(t *testing.T) {
	var points []cfg.Point
	sortPoints(points)
	if len(points) != 0 {
		t.Error("expected empty slice to remain empty")
	}
}

func TestSortPoints_Single(t *testing.T) {
	points := []cfg.Point{5}
	sortPoints(points)
	if points[0] != 5 {
		t.Errorf("expected 5, got %d", points[0])
	}
}

func TestSortPoints_Multiple(t *testing.T) {
	points := []cfg.Point{3, 1, 4, 1, 5, 9, 2, 6}
	sortPoints(points)
	for i := 1; i < len(points); i++ {
		if points[i] < points[i-1] {
			t.Errorf("not sorted at index %d: %d < %d", i, points[i], points[i-1])
		}
	}
}

func TestIsUnknownOrNil_Nil(t *testing.T) {
	if !isUnknownOrNil(nil) {
		t.Error("expected true for nil")
	}
}

func TestIsUnknownOrNil_Unknown(t *testing.T) {
	if !isUnknownOrNil(typ.Unknown) {
		t.Error("expected true for typ.Unknown")
	}
}

func TestIsUnknownOrNil_ValidType(t *testing.T) {
	if isUnknownOrNil(typ.String) {
		t.Error("expected false for typ.String")
	}
}

func TestNarrowReturnTypeBySpec_NilCallInfo(t *testing.T) {
	result := NarrowReturnTypeBySpec(nil, nil, nil, 0, nil)
	if result != nil {
		t.Errorf("expected nil for nil callInfo, got %v", result)
	}
}

func TestNarrowReturnTypeBySpec_NilSynth(t *testing.T) {
	callInfo := &cfg.CallInfo{}
	result := NarrowReturnTypeBySpec(callInfo, nil, nil, 0, nil)
	if result != nil {
		t.Errorf("expected nil for nil synth, got %v", result)
	}
}

func TestNarrowReturnTypeBySpec_WithSynth(t *testing.T) {
	callInfo := &cfg.CallInfo{
		Callee: &ast.IdentExpr{Value: "fn"},
	}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String
	}
	result := NarrowReturnTypeBySpec(callInfo, nil, synth, 0, nil)
	// String type has no spec, so result should be nil
	if result != nil {
		t.Errorf("expected nil for type without spec, got %v", result)
	}
}

func TestNarrowReturnTypeBySpec_WithSymResolver(t *testing.T) {
	callInfo := &cfg.CallInfo{
		CalleeSymbol: 1,
	}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return nil
	}
	symResolver := func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		return typ.Integer, true
	}
	result := NarrowReturnTypeBySpec(callInfo, nil, synth, 0, symResolver)
	// Integer type has no spec, so result should be nil
	if result != nil {
		t.Errorf("expected nil for type without spec, got %v", result)
	}
}
