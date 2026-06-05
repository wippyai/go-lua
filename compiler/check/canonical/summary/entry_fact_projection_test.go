package summary_test

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/typ"
)

func TestDirectCallEntryFactsProjectsLengthBoundsToParamPaths(t *testing.T) {
	source := constraint.NewPath(cfg.SymbolID(10), "graph")
	nodeOrder := source.Field("node_order")
	num := numeric.NewState()
	num.ApplyLenGeConst(flow.SymbolPathKey(nodeOrder.Symbol, nodeOrder.Segments), 1)

	got := summary.DirectCallEntryFacts(summary.DirectCallEntryFactInput{
		Call:   &ast.FuncCallExpr{Args: []ast.Expr{&ast.IdentExpr{Value: "graph"}}},
		Callee: summary.FuncRef{GraphID: 7},
		ParamSlot: func(summary.FuncRef, *ast.FuncCallExpr, int) (int, int, bool) {
			return 0, 0, true
		},
		ArgPath: func(int, ast.Expr) (constraint.Path, bool) {
			return source, true
		},
		Num: num,
	})

	bounds := got.LengthLowerBounds()
	if len(bounds) != 1 {
		t.Fatalf("length bounds = %#v, want one projected bound", bounds)
	}
	want := flow.BoundaryPath{
		Kind:     flow.BoundaryPathParam,
		Index:    0,
		Segments: nodeOrder.Segments,
	}
	if !boundaryPathEqualForTest(bounds[0].Target, want) || bounds[0].Lower != 1 {
		t.Fatalf("length bound = %#v, want %#v >= 1", bounds[0], want)
	}
}

func TestDirectCallEntryFactsProjectsIndexWritesToParamPaths(t *testing.T) {
	source := constraint.NewPath(cfg.SymbolID(11), "graph")
	table := source.Field("nodes")
	key := source.Field("last_node_id")
	value := product.FromType(typ.String)

	got := summary.DirectCallEntryFacts(summary.DirectCallEntryFactInput{
		Call:   &ast.FuncCallExpr{Args: []ast.Expr{&ast.IdentExpr{Value: "graph"}}},
		Callee: summary.FuncRef{GraphID: 8},
		ParamSlot: func(summary.FuncRef, *ast.FuncCallExpr, int) (int, int, bool) {
			return 0, 0, true
		},
		ArgPath: func(int, ast.Expr) (constraint.Path, bool) {
			return source, true
		},
		IndexWrites: flow.IndexWriteAdmissionFacts{}.With(flow.IndexWriteAdmissionFact{
			Target:  flow.IndexWriteAdmissionPathKey(table),
			KeyPath: flow.IndexWriteAdmissionPathKey(key),
			Key:     product.FromType(typ.String),
			Value:   value,
		}),
	})

	writes := got.IndexWrites()
	if len(writes) != 1 {
		t.Fatalf("index writes = %#v, want one projected write", writes)
	}
	wantTable := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0, Segments: table.Segments}
	wantKey := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0, Segments: key.Segments}
	if !boundaryPathEqualForTest(writes[0].Table, wantTable) ||
		!boundaryPathEqualForTest(writes[0].Key, wantKey) ||
		!product.Domain.Equal(writes[0].Value, value) {
		t.Fatalf("index write = %#v, want table %#v key %#v value %s", writes[0], wantTable, wantKey, value.ProjectValue())
	}
}

func boundaryPathEqualForTest(a, b flow.BoundaryPath) bool {
	return a.Kind == b.Kind &&
		a.Index == b.Index &&
		slices.Equal(a.Segments, b.Segments)
}
