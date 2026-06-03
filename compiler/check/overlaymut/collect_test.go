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

func TestMapWriteInfo_ZeroValue(t *testing.T) {
	var info MapWriteInfo
	if info.KeyType != nil {
		t.Error("expected nil KeyType for zero value MapWriteInfo")
	}
	if info.ValueType != nil {
		t.Error("expected nil ValueType for zero value MapWriteInfo")
	}
}

func TestMergeMapWriteMutations_EmptyInputs(t *testing.T) {
	mapWrites := make(map[cfg.SymbolID][]MapWriteInfo)
	mutations := make(map[cfg.SymbolID][]MapWriteInfo)

	MergeMapWriteMutations(mapWrites, mutations)

	if len(mapWrites) != 0 {
		t.Errorf("expected empty mapWrites after merging empty mutations, got %d", len(mapWrites))
	}
}

func TestMergeMapWriteMutations_MergesCorrectly(t *testing.T) {
	mapWrites := make(map[cfg.SymbolID][]MapWriteInfo)
	mapWrites[1] = []MapWriteInfo{{KeyType: typ.String, ValueType: typ.Integer}}

	mutations := make(map[cfg.SymbolID][]MapWriteInfo)
	mutations[1] = []MapWriteInfo{{KeyType: typ.Integer, ValueType: typ.String}}
	mutations[2] = []MapWriteInfo{{KeyType: typ.Number, ValueType: typ.Boolean}}

	MergeMapWriteMutations(mapWrites, mutations)

	if len(mapWrites[1]) != 2 {
		t.Errorf("expected 2 infos for symbol 1, got %d", len(mapWrites[1]))
	}
	if len(mapWrites[2]) != 1 {
		t.Errorf("expected 1 info for symbol 2, got %d", len(mapWrites[2]))
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
	if fieldType := projectedField(got[1], "from"); !typ.TypeEquals(fieldType, fnType) {
		t.Fatalf("method field type = %v, want %v", fieldType, fnType)
	}
}

func TestCollectMapWriteAssignments_NilGraph(t *testing.T) {
	result := CollectMapWriteAssignments(nil, nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil result for nil graph")
	}
	if len(result) != 0 {
		t.Errorf("expected empty result for nil graph, got %d entries", len(result))
	}
}

func TestCollectMapWriteAssignments_EmptyGraph(t *testing.T) {
	graph := buildEmptyGraph()
	result := CollectMapWriteAssignments(assignmentsFromGraph(graph), nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectMapWriteAssignments_WithSynth(t *testing.T) {
	graph := buildEmptyGraph()
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.Integer
	}
	result := CollectMapWriteAssignments(assignmentsFromGraph(graph), synth, nil, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectMapWriteAssignments_WithBindings(t *testing.T) {
	graph := buildEmptyGraph()
	bindings := &bind.BindingTable{}
	result := CollectMapWriteAssignments(assignmentsFromGraph(graph), nil, bindings, nil)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectMapWriteAssignments_WithFilter(t *testing.T) {
	graph := buildEmptyGraph()
	filter := make(map[cfg.SymbolID]bool)
	filter[1] = true
	result := CollectMapWriteAssignments(assignmentsFromGraph(graph), nil, nil, filter)
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestCollectMapWriteAssignments_AllParams(t *testing.T) {
	graph := buildEmptyGraph()
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.String
	}
	bindings := &bind.BindingTable{}
	filter := make(map[cfg.SymbolID]bool)
	filter[1] = true
	result := CollectMapWriteAssignments(assignmentsFromGraph(graph), synth, bindings, filter)
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
