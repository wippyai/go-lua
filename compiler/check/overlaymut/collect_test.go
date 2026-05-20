package overlaymut

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

func buildEmptyGraph() *cfg.Graph {
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	return cfg.Build(fn)
}

func TestIndexerInfo_ZeroValue(t *testing.T) {
	var info IndexerInfo
	if info.KeyType != nil {
		t.Error("expected nil KeyType for zero value IndexerInfo")
	}
	if info.ValType != nil {
		t.Error("expected nil ValType for zero value IndexerInfo")
	}
}

func TestMergeIndexerMutations_EmptyInputs(t *testing.T) {
	indexers := make(map[cfg.SymbolID][]IndexerInfo)
	mutations := make(map[cfg.SymbolID][]IndexerInfo)

	MergeIndexerMutations(indexers, mutations)

	if len(indexers) != 0 {
		t.Errorf("expected empty indexers after merging empty mutations, got %d", len(indexers))
	}
}

func TestMergeIndexerMutations_MergesCorrectly(t *testing.T) {
	indexers := make(map[cfg.SymbolID][]IndexerInfo)
	indexers[1] = []IndexerInfo{{KeyType: typ.String, ValType: typ.Integer}}

	mutations := make(map[cfg.SymbolID][]IndexerInfo)
	mutations[1] = []IndexerInfo{{KeyType: typ.Integer, ValType: typ.String}}
	mutations[2] = []IndexerInfo{{KeyType: typ.Number, ValType: typ.Boolean}}

	MergeIndexerMutations(indexers, mutations)

	if len(indexers[1]) != 2 {
		t.Errorf("expected 2 infos for symbol 1, got %d", len(indexers[1]))
	}
	if len(indexers[2]) != 1 {
		t.Errorf("expected 1 info for symbol 2, got %d", len(indexers[2]))
	}
}

func TestCollectFieldAssignments_NilGraph(t *testing.T) {
	result := CollectFieldAssignments(nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil result for nil graph")
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for nil graph, got %d entries", len(result))
	}
}

func TestCollectFieldAssignments_EmptyGraph(t *testing.T) {
	graph := buildEmptyGraph()
	result := CollectFieldAssignments(assignmentsFromGraph(graph), nil, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectFieldAssignments_WithSynth(t *testing.T) {
	graph := buildEmptyGraph()
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String
	}
	result := CollectFieldAssignments(assignmentsFromGraph(graph), synth, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectFieldAssignments_WithFilter(t *testing.T) {
	graph := buildEmptyGraph()
	filter := make(map[cfg.SymbolID]bool)
	filter[1] = true
	result := CollectFieldAssignments(assignmentsFromGraph(graph), nil, filter)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectIndexerAssignments_NilGraph(t *testing.T) {
	result := CollectIndexerAssignments(nil, nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil result for nil graph")
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for nil graph, got %d entries", len(result))
	}
}

func TestCollectIndexerAssignments_EmptyGraph(t *testing.T) {
	graph := buildEmptyGraph()
	result := CollectIndexerAssignments(assignmentsFromGraph(graph), nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectIndexerAssignments_WithSynth(t *testing.T) {
	graph := buildEmptyGraph()
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.Integer
	}
	result := CollectIndexerAssignments(assignmentsFromGraph(graph), synth, nil, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectIndexerAssignments_WithBindings(t *testing.T) {
	graph := buildEmptyGraph()
	bindings := &bind.BindingTable{}
	result := CollectIndexerAssignments(assignmentsFromGraph(graph), nil, bindings, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectIndexerAssignments_WithFilter(t *testing.T) {
	graph := buildEmptyGraph()
	filter := make(map[cfg.SymbolID]bool)
	filter[1] = true
	result := CollectIndexerAssignments(assignmentsFromGraph(graph), nil, nil, filter)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectIndexerAssignments_AllParams(t *testing.T) {
	graph := buildEmptyGraph()
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String
	}
	bindings := &bind.BindingTable{}
	filter := make(map[cfg.SymbolID]bool)
	filter[1] = true
	result := CollectIndexerAssignments(assignmentsFromGraph(graph), synth, bindings, filter)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func assignmentsFromGraph(graph *cfg.Graph) []api.AssignmentEvidence {
	if graph == nil {
		return nil
	}
	var assignments []api.AssignmentEvidence
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info != nil {
			assignments = append(assignments, api.AssignmentEvidence{Point: p, Info: info})
		}
	})
	return assignments
}
