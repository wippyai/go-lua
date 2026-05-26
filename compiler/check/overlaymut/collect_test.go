package overlaymut

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func buildEmptyGraph() *cfg.Graph {
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	return cfg.Build(fn)
}

func TestMapMutatorInfo_ZeroValue(t *testing.T) {
	var info MapMutatorInfo
	if info.KeyType != nil {
		t.Error("expected nil KeyType for zero value MapMutatorInfo")
	}
	if info.ValueType != nil {
		t.Error("expected nil ValueType for zero value MapMutatorInfo")
	}
}

func TestMergeMapMutatorMutations_EmptyInputs(t *testing.T) {
	mapMutators := make(map[cfg.SymbolID][]MapMutatorInfo)
	mutations := make(map[cfg.SymbolID][]MapMutatorInfo)

	MergeMapMutatorMutations(mapMutators, mutations)

	if len(mapMutators) != 0 {
		t.Errorf("expected empty mapMutators after merging empty mutations, got %d", len(mapMutators))
	}
}

func TestMergeMapMutatorMutations_MergesCorrectly(t *testing.T) {
	mapMutators := make(map[cfg.SymbolID][]MapMutatorInfo)
	mapMutators[1] = []MapMutatorInfo{{KeyType: typ.String, ValueType: typ.Integer}}

	mutations := make(map[cfg.SymbolID][]MapMutatorInfo)
	mutations[1] = []MapMutatorInfo{{KeyType: typ.Integer, ValueType: typ.String}}
	mutations[2] = []MapMutatorInfo{{KeyType: typ.Number, ValueType: typ.Boolean}}

	MergeMapMutatorMutations(mapMutators, mutations)

	if len(mapMutators[1]) != 2 {
		t.Errorf("expected 2 infos for symbol 1, got %d", len(mapMutators[1]))
	}
	if len(mapMutators[2]) != 1 {
		t.Errorf("expected 1 info for symbol 2, got %d", len(mapMutators[2]))
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

func TestCollectFunctionFieldAssignments_MethodDefinition(t *testing.T) {
	fnType := typ.Func().Param("self", typ.Any).Returns(typ.String).Build()
	functions := []api.FunctionDefinitionEvidence{{
		Nested: cfg.NestedFunc{Point: 7},
		FuncDef: &cfg.FuncDefInfo{
			TargetKind: cfg.FuncDefMethod,
			FuncExpr:   &ast.FunctionExpr{},
			TargetPath: constraint.Path{
				Symbol: 1,
				Segments: []constraint.Segment{{
					Kind: constraint.SegmentField,
					Name: "from",
				}},
			},
		},
	}}
	got := CollectFunctionFieldAssignments(functions, func(expr ast.Expr, p cfg.Point) typ.Type {
		if p != 7 {
			t.Fatalf("synth point = %d, want 7", p)
		}
		return fnType
	}, nil)
	if !typ.TypeEquals(got[1]["from"], fnType) {
		t.Fatalf("method field type = %v, want %v", got[1]["from"], fnType)
	}
}

func TestCollectMapMutatorAssignments_NilGraph(t *testing.T) {
	result := CollectMapMutatorAssignments(nil, nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil result for nil graph")
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for nil graph, got %d entries", len(result))
	}
}

func TestCollectMapMutatorAssignments_EmptyGraph(t *testing.T) {
	graph := buildEmptyGraph()
	result := CollectMapMutatorAssignments(assignmentsFromGraph(graph), nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectMapMutatorAssignments_WithSynth(t *testing.T) {
	graph := buildEmptyGraph()
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.Integer
	}
	result := CollectMapMutatorAssignments(assignmentsFromGraph(graph), synth, nil, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectMapMutatorAssignments_WithBindings(t *testing.T) {
	graph := buildEmptyGraph()
	bindings := &bind.BindingTable{}
	result := CollectMapMutatorAssignments(assignmentsFromGraph(graph), nil, bindings, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectMapMutatorAssignments_WithFilter(t *testing.T) {
	graph := buildEmptyGraph()
	filter := make(map[cfg.SymbolID]bool)
	filter[1] = true
	result := CollectMapMutatorAssignments(assignmentsFromGraph(graph), nil, nil, filter)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectMapMutatorAssignments_AllParams(t *testing.T) {
	graph := buildEmptyGraph()
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String
	}
	bindings := &bind.BindingTable{}
	filter := make(map[cfg.SymbolID]bool)
	filter[1] = true
	result := CollectMapMutatorAssignments(assignmentsFromGraph(graph), synth, bindings, filter)
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
