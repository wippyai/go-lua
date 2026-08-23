package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

func TestGraphCatalogActivationReadRetainsSealedRowOutsideOrdinaryGeometry(t *testing.T) {
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(999_101))
	readForm, readOK := factor.ExactRead()
	family, familyOK := DeclareSchemaActivationFamily(builder, coldKey(999_102))
	rule, ruleOK := DeclareSchemaActivationRule(builder, SchemaStructuralRuleSpec{
		Semantic: coldKey(999_103), Activation: family, Inputs: 1,
	})
	input, inputOK := rule.Input(0)
	read, activationReadOK := SchemaActivationRead(rule, readForm, input)
	schema, schemaOK := builder.Seal()
	binding := NewSchemaBinding(schema)
	if !factorOK || !readOK || !familyOK || !ruleOK || !inputOK || !activationReadOK || !schemaOK || schema == nil ||
		!BindFactor(binding, factor, hotUintFactorSpec()) {
		t.Fatal("activation read schema")
	}
	if _, bound := BindActivationRuleWithExactRead[uint64, uint64](binding, rule, read, factor, HotActivationSpec{Fold: func(ActivationFrame) ActivationResult { return Activated(ActivationFrame{}) }}); !bound {
		t.Fatal("activation read binding")
	}
	if !binding.Seal() {
		t.Fatal("activation read seal")
	}
	ordinary, activations, mapsOK := buildSealedGraphRuleMaps(binding.state)
	if !mapsOK || len(ordinary) != 0 || len(activations) != 1 {
		t.Fatalf("sealed rule maps ordinary=%d activations=%d ok=%v", len(ordinary), len(activations), mapsOK)
	}
	key := schema.ruleSemanticAt(0)
	cell := activations[key]
	if cell == nil || !activationGraphCellReady(binding.state, cell) {
		t.Fatal("activation cell not ready")
	}
	if _, structural := any(cell).(sealedRuleGeometry); structural {
		t.Fatal("activation cell implements sealed rule geometry")
	}
	row := cell.schemaRuleReadAt(0)
	if row == nil || row.kind != composition.ReadExact || row.factor != schema.factorSemanticAt(0) || !row.sealed() {
		t.Fatal("activation read row was not retained as sealed direct geometry")
	}
}

func TestGraphCarryClosureSCCPreservesDirectTargetsAndRouteBit(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 3, nil, nil)
	graph := fixture.solver.runtime.graph
	if graph == nil || graph.PointCount() < 3 {
		t.Fatal("carry SCC graph points")
	}
	points := make([]equation.Point, 3)
	for index := range points {
		point, ok := graph.PointAt(schedule.Node(index))
		if !ok {
			t.Fatalf("point %d", index)
		}
		points[index] = point
	}
	factor := fixture.binding.state.schema.factorSemanticAt(0)
	direct := equation.Surface{Factor: factor, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}
	plan := &graphCarryFactorClosure{
		nodes: []*graphCarryClosureNode{
			{point: points[0], direct: []equation.Surface{direct}},
			{point: points[1], predecessors: []composition.Key{points[0].Key(), points[2].Key()}},
			{point: points[2], route: true, predecessors: []composition.Key{points[1].Key()}},
		},
		byPoint: map[composition.Key]int{points[0].Key(): 0, points[1].Key(): 1, points[2].Key(): 2},
	}
	closures := make(map[graphCarryClosureKey]graphCarryClosure)
	if !buildGraphCarryFactorClosures(closures, factor, plan) {
		t.Fatal("carry SCC closure")
	}
	for index, point := range points {
		closure, ok := closures[graphCarryClosureKey{factor: factor, point: point.Key()}]
		if !ok || len(closure.targets) != 1 || closure.targets[0] != direct {
			t.Fatalf("point %d closure targets=%#v ok=%v", index, closure.targets, ok)
		}
		if index == 0 && closure.route {
			t.Fatal("direct predecessor acquired route bit")
		}
		if index != 0 && !closure.route {
			t.Fatalf("SCC point %d lost route bit", index)
		}
	}
}
