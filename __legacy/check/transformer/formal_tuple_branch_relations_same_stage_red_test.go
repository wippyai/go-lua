package transformer

import (
	"context"
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestFormalBranchRelationsSerializedFamilyReconciliationsAccumulate(t *testing.T) {
	reg := standard.Registry()
	base := formalRootInputTestProgram(t, reg)
	body := &base.bodies[0]
	point := body.graph.Entry()
	x, y := symbol.ID(101), symbol.ID(102)
	xPath, yPath := pathdom.NewPath(x, "param"), pathdom.NewPath(y, "capture")
	resolver := visibility.NewResolver(nil)
	body.keys = resolver.KeySpace()
	body.pathSemantics = factapply.NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
	roots, err := sealRelationRootCarrierWithAmbients(body.plan, body.keys, body.relation.shape, []AmbientRoot{{Symbol: symbol.ID(104)}})
	if err != nil {
		t.Fatal(err)
	}
	body.roots = roots
	body.entrySeedPlan = state.NewEntrySeedPlan([]state.ValueSeed{
		{Slot: statekey.SymbolValue(101), Value: product.Top()},
		{Slot: statekey.SymbolValue(102), Value: product.Top()},
		{Slot: statekey.SymbolValue(103), Value: product.Top()},
		{Slot: statekey.SymbolValue(104), Value: product.Top()},
	})
	body.initialStatePlan = testInitialStatePlan(t, body.body, body.graph,
		state.NewInitialStateSeed(state.InitialCoordinate(point), state.Reachable(state.State{})))

	rows := factflow.NewBranchRefinementSet().WithNumFloorRefinements(
		factflow.NewBranchNumFloorRefinementOnEdge(xPath, 4, true),
		factflow.NewBranchNumFloorRefinementOnEdge(yPath, 9, true),
	)
	facts := factflow.NewFacts(factflow.FactsInput{
		BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{point: rows},
	})
	transaction := factapply.PlanBranchRelationTransaction(facts, point, true)
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepBranchRelations, branch: transaction}}, next: 2},
		{kind: relationNodeOutcome, outcome: 1},
	})
	cell := formalRelationCell{Variable: 1, Kind: formalRelationCellStep, Root: 1, Step: 1}
	equation, ok := program.formalTemplate.equation(cell)
	if !ok || equation.Operator.branchRelations == nil {
		t.Fatal("formal BranchRelations capability was not admitted")
	}
	step := equation.Operator.branchRelations
	if len(step.stages) != 2 || len(step.stages[0].Factors()) != 1 || len(step.stages[1].Factors()) != 1 {
		t.Fatalf("family reconciliation stages = %#v, want two serialized one-factor stages", step.stages)
	}
	if len(step.stageFactorGroups) != 2 || len(step.stageFactorGroups[0]) != 1 || len(step.stageFactorGroups[0][0]) != 1 ||
		len(step.stageFactorGroups[1]) != 1 || len(step.stageFactorGroups[1][0]) != 1 {
		t.Fatalf("family reconciliation groups = %#v, want one factor group per serialized stage", step.stageFactorGroups)
	}
	execution, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	operands, err := partitionFormalRelationStepOperands(equation)
	if err != nil {
		t.Fatal(err)
	}
	before := execution.values[operands.Flow.Source.cell]
	after := execution.values[cell]
	beforeRegions, err := execution.algebra.tupleLeafRegions(before)
	if err != nil || len(beforeRegions) != 1 {
		t.Fatalf("before regions=%d err=%v", len(beforeRegions), err)
	}
	afterRegions, err := execution.algebra.tupleLeafRegions(after)
	if err != nil || len(afterRegions) != 1 {
		t.Fatalf("after regions=%d err=%v", len(afterRegions), err)
	}
	factorIndexes := append(step.stages[0].Factors(), step.stages[1].Factors()...)
	for _, factorIndex := range factorIndexes {
		plan := step.plans[factorIndex].current.coordinates[0]
		if plan.publication != factapply.BranchRelationCoordinatePublicationReconcile {
			t.Fatalf("factor %d publication = %v, want family reconciliation", factorIndex, plan.publication)
		}
		prior, materializeErr := execution.algebra.materializeFormalBranchCoordinateOperands(beforeRegions[0].evaluator, plan)
		if materializeErr != nil {
			t.Fatal(materializeErr)
		}
		next, materializeErr := execution.algebra.materializeFormalBranchCoordinateOperands(afterRegions[0].evaluator, plan)
		if materializeErr != nil {
			t.Fatal(materializeErr)
		}
		equal, equalErr := body.productDomain.CoordinateScalarEqual(prior.Scalars[0], next.Scalars[0])
		if equalErr != nil || equal {
			t.Fatalf("serialized family factor %d was erased by its sibling: equal=%t err=%v", factorIndex, equal, equalErr)
		}
	}
	// A guard attached only to an unselected sibling must not multiply the
	// semantic factor regions. The sibling remains an exact structural carrier
	// through canonical rootwise reconciliation.
	guardedPlan := step.plans[factorIndexes[0]]
	guardedCoordinate := guardedPlan.current.coordinates[0]
	_, directory, _, ok := execution.algebra.span(after.variable)
	if !ok {
		t.Fatal("family reconciliation span")
	}
	carrier := formalBranchRelationCoordinateCarrier{}
	prior := formalFiberValue(0)
	for _, candidate := range guardedCoordinate.carriers {
		value, valueErr := directory.valueAt(after.root, candidate.ordinal)
		if valueErr != nil {
			t.Fatal(valueErr)
		}
		if value != 0 {
			carrier, prior = candidate, value
			break
		}
	}
	if carrier.ordinal == 0 {
		t.Fatal("family reconciliation fixture has no explicit preservation sibling")
	}
	binary := execution.algebra.decisions.branch(0, decisionFalse, decisionRef(prior))
	delta, err := directory.sealDelta([]formalFiberWrite{{ordinal: carrier.ordinal, value: formalFiberValue(binary)}})
	if err != nil {
		t.Fatal(err)
	}
	guardedRoot, _, err := directory.applyDelta(after.root, delta)
	if err != nil {
		t.Fatal(err)
	}
	guarded := execution.algebra.normalize(formalRelationTuple{variable: after.variable, root: guardedRoot})
	regions, err := execution.algebra.partitionSparseLeafViewsUnderCare([]formalSparseTupleProjection{{
		tuple: guarded, ordinals: guardedPlan.currentProjectionOrdinals,
	}}, nil)
	if err != nil || len(regions) != 1 {
		t.Fatalf("family reconciliation semantic regions = %d, %v; unrelated sibling guard entered correlation", len(regions), err)
	}
	result, err := execution.algebra.applyFormalBranchRelationFactor(guarded, guarded, step, factorIndexes[0])
	if err != nil {
		t.Fatal(err)
	}
	completed, err := execution.algebra.applyFormalBranchRelationFactorResult(guarded, result)
	if err != nil {
		t.Fatal(err)
	}
	carried, err := directory.valueAt(completed.root, carrier.ordinal)
	if err != nil || carried != formalFiberValue(binary) {
		t.Fatalf("family reconciliation sibling carrier = %d, %v; want exact root %d", carried, err, binary)
	}
	group := step.plans[factorIndexes[0]].current.coordinates[0].group
	if _, err := afterRegions[0].evaluator.laneFactor(group); err != nil {
		t.Fatalf("final accumulated factor spelling is not producer-registered: %v", err)
	}
}
