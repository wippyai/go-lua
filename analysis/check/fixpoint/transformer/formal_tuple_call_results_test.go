package transformer

import (
	"context"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFormalCallResultsUsesCanonicalCombinedN3Program(t *testing.T) {
	reg := standard.Registry()
	base := formalRootInputTestProgram(t, reg)
	body := &base.bodies[0]
	point := body.graph.Entry()
	const leftID, rightID, targetID = symbol.ID(101), symbol.ID(102), symbol.ID(103)
	left, right, target := pathdom.NewPath(leftID, "param"), pathdom.NewPath(rightID, "capture"), pathdom.NewPath(targetID, "global")
	builder := visibility.NewBuilder()
	builder.Define(point, leftID, "param")
	builder.Define(point, rightID, "capture")
	builder.Define(point, targetID, "global")
	resolver := visibility.NewResolver(builder.Build())
	body.keys = resolver.KeySpace()
	body.pathSemantics = factapply.NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
	roots, err := sealRelationRootCarrierWithAmbients(body.plan, body.keys, body.relation.shape, []AmbientRoot{{Symbol: symbol.ID(104)}})
	if err != nil {
		t.Fatal(err)
	}
	body.roots = roots
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	facts := factflow.NewFacts(factflow.FactsInput{
		PostconditionRefinements: map[cfg.Point]factflow.PostconditionRefinementSet{point: factflow.NewPostconditionRefinementSet(
			factflow.NewPostconditionRefinement(left, factflow.NewValueConstraint(present)),
		)},
		PostconditionPathRelations: map[cfg.Point][]factflow.PostconditionPathRelation{point: {factflow.NewPostconditionPathEquality(left, right)}},
	})
	transaction := factapply.PlanCallResultTransaction(facts, point)
	leftKey, leftOK := visibility.AddressAt(resolver, point, left).RootOrVisibleKeyspaceKey()
	targetKey, targetOK := visibility.AddressAt(resolver, point, target).RootOrVisibleKeyspaceKey()
	if !leftOK || !targetOK {
		t.Fatal("presence endpoints")
	}
	row := pathevidence.NewPathPresenceImplication(leftKey, presence.Present(), targetKey, presence.Present())
	input := state.Reachable(body.productDomain.Lattice().Bottom()).
		WriteValue(reg, statekey.SymbolValue(leftID), product.Top()).
		WriteValue(reg, statekey.SymbolValue(rightID), present).
		WriteValue(reg, statekey.SymbolValue(targetID), product.Top()).
		AddPathPresenceImplication(row)
	body.entrySeedPlan = state.NewEntrySeedPlan([]state.ValueSeed{
		{Slot: statekey.SymbolValue(leftID), Value: product.Top()},
		{Slot: statekey.SymbolValue(rightID), Value: present},
		{Slot: statekey.SymbolValue(targetID), Value: product.Top()},
		{Slot: statekey.SymbolValue(104), Value: product.Top()},
	})
	body.initialStatePlan = testInitialStatePlan(t, body.body, body.graph, state.NewInitialStateSeed(state.InitialCoordinate(point), input))

	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepCallResults, result: transaction, resultPhase: factapply.CallResultPhasePostconditions}}, next: 2},
		{kind: relationNodeOutcome, outcome: 1},
	})
	cell := formalRelationCell{Variable: 1, Kind: formalRelationCellStep, Root: 1, Step: 1}
	equation, ok := program.formalTemplate.equation(cell)
	if !ok || equation.Operator.stepCapability != formalRelationStepCapabilityCallResults || equation.Operator.callResults == nil {
		t.Fatal("formal CallResults N3 capability was not admitted")
	}
	span := program.formalFibers.spans[0]
	if len(equation.Operator.callResults.readOrdinals) >= span.count {
		t.Fatalf("CallResults N3 width=%d product=%d", len(equation.Operator.callResults.readOrdinals), span.count)
	}
	execution, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	operands, err := partitionFormalRelationStepOperands(equation)
	if err != nil {
		t.Fatal(err)
	}
	before, after := execution.values[operands.Flow.Source.cell], execution.values[cell]
	beforeRegions, beforeErr := execution.algebra.tupleLeafRegions(before)
	afterRegions, afterErr := execution.algebra.tupleLeafRegions(after)
	if beforeErr != nil || afterErr != nil || len(beforeRegions) != 1 || len(afterRegions) != 1 {
		t.Fatalf("formal regions before/after=%d/%d err=%v/%v", len(beforeRegions), len(afterRegions), beforeErr, afterErr)
	}
	formalValues, err := afterRegions[0].evaluator.valuesFactor()
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct {
		name string
		slot statekey.Value
		want product.Value
	}{
		{"param", statekey.SymbolValue(leftID), present},
		{"capture", statekey.SymbolValue(rightID), present},
		{"global", statekey.SymbolValue(targetID), present},
	} {
		formalSlot, found := formalMiddleSlotForStateKey(program, body, pair.slot)
		if !found {
			t.Fatal("formal Values slot")
		}
		got := product.Bottom(reg)
		if formalValues.Top {
			got = product.Top()
		} else if value, present := formalValues.Values[formalSlot]; present {
			got = value
		}
		if !product.Equal(reg, got, pair.want) {
			t.Fatalf("formal N3 Values mismatch for %s: presence=%v/%v formalTop=%t", pair.name, product.PresenceOf(got), product.PresenceOf(pair.want), formalValues.Top)
		}
	}
	owned := make(map[state.LaneID]struct{})
	for _, lane := range equation.Operator.callResults.program.Lanes() {
		owned[lane.ID()] = struct{}{}
	}
	for _, group := range span.groupDescriptors() {
		if group.kind == formalFiberGroupValues {
			continue
		}
		if _, changes := owned[group.lane.ID()]; changes {
			continue
		}
		prior, priorErr := beforeRegions[0].evaluator.laneFactor(group)
		next, nextErr := afterRegions[0].evaluator.laneFactor(group)
		equal, equalErr := body.productDomain.LaneEqual(prior, next)
		if priorErr != nil || nextErr != nil || equalErr != nil || !equal {
			t.Fatalf("CallResults N3 changed residual lane %s: %v/%v/%v", group.lane.ID(), priorErr, nextErr, equalErr)
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	priorContext := execution.algebra.ctx
	priorDecisions := len(execution.algebra.decisions.nodes)
	execution.algebra.ctx = canceled
	rolled, cancelErr := execution.algebra.applyFormalCallResults(equation.Operator, before)
	execution.algebra.ctx = priorContext
	if cancelErr == nil || !rolled.bottom() || len(execution.algebra.decisions.nodes) != priorDecisions {
		t.Fatalf("canceled formal N3 published work: err=%v bottom=%t decisions=%d/%d", cancelErr, rolled.bottom(), len(execution.algebra.decisions.nodes), priorDecisions)
	}
}
