package transformer

import (
	"context"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFormalPresenceImplicationsUsesCanonicalTransitiveN2WithDescendantBarrier(t *testing.T) {
	reg := standard.Registry()
	base := formalRootInputTestProgram(t, reg)
	body := &base.bodies[0]
	point := body.graph.Entry()
	const x, y, z = symbol.ID(101), symbol.ID(102), symbol.ID(103)
	xPath, yPath, zPath := pathdom.NewPath(x, "x"), pathdom.NewPath(y, "y"), pathdom.NewPath(z, "z")
	absentPath := zPath.Field("gone")
	builder := visibility.NewBuilder()
	builder.Define(point, x, "x")
	builder.Define(point, y, "y")
	builder.Define(point, z, "z")
	resolver := visibility.NewResolver(builder.Build())
	body.keys = resolver.KeySpace()
	body.pathSemantics = factapply.NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
	roots, err := sealRelationRootCarrierWithAmbients(body.plan, body.keys, body.relation.shape, []AmbientRoot{{Symbol: symbol.ID(104)}})
	if err != nil {
		t.Fatal(err)
	}
	body.roots = roots
	falseValue := typevalue.LiteralBool(reg, false)
	presentValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	transaction := factapply.PlanPathValuePresenceImplicationTransaction(factflow.NewFacts(factflow.FactsInput{
		PathValuePresenceImplications: map[cfg.Point]factflow.PathValuePresenceImplicationSet{
			point: factflow.NewPathValuePresenceImplicationSet(
				factflow.NewPathValueRefinementImplication(xPath, falseValue, yPath, falseValue),
				factflow.NewPathValuePresenceImplication(yPath, falseValue, zPath, presence.Present()),
				factflow.NewPathValuePresenceImplication(zPath, presentValue, absentPath, presence.Absent()),
			),
		},
	}), point)
	absentKey, absentOK := visibility.AddressAt(resolver, point, absentPath).RootOrVisibleKeyspaceKey()
	childKey, childOK := body.keys.AppendSegment(absentKey, segment.Segment{Kind: segment.SegmentField, Name: "child"})
	if !absentOK || !childOK {
		t.Fatal("child path is not visible")
	}
	if _, prefix := body.keys.ExactRemainderAfterPrefix(childKey, absentKey); !prefix {
		t.Fatalf("child %q is not a structural descendant of target %q", body.keys.FormatReadOnly(childKey), body.keys.FormatReadOnly(absentKey))
	}
	input := state.Reachable(body.productDomain.Lattice().Bottom()).
		WriteValue(reg, statekey.SymbolValue(x), falseValue).
		WriteValue(reg, statekey.SymbolValue(y), product.Top()).
		WriteValue(reg, statekey.SymbolValue(z), product.Top()).
		WriteLocalPathKey(reg, childKey, typevalue.LiteralString(reg, "stale"))
	body.entrySeedPlan = state.NewEntrySeedPlan([]state.ValueSeed{
		{Slot: statekey.SymbolValue(x), Value: falseValue},
		{Slot: statekey.SymbolValue(y), Value: product.Top()},
		{Slot: statekey.SymbolValue(z), Value: product.Top()},
		{Slot: statekey.SymbolValue(104), Value: product.Top()},
	})
	body.initialStatePlan = testInitialStatePlan(t, body.body, body.graph, state.NewInitialStateSeed(state.InitialCoordinate(point), input))
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepPresenceImplications, point: point, presence: transaction}}, next: 2},
		{kind: relationNodeOutcome, outcome: 1},
	})
	cell := formalRelationCell{Variable: 1, Kind: formalRelationCellStep, Root: 1, Step: 1}
	equation, ok := program.formalTemplate.equation(cell)
	if !ok || equation.Operator.stepCapability != formalRelationStepCapabilityPresenceImplications || equation.Operator.presenceImplications == nil {
		t.Fatal("formal N2 capability was not admitted")
	}
	span := program.formalFibers.spans[0]
	if len(equation.Operator.presenceImplications.readOrdinals) >= span.count ||
		len(equation.Operator.presenceImplications.projectionOrdinals) >= span.count {
		t.Fatalf("formal N2 retained full-product input: read/projection/span=%d/%d/%d",
			len(equation.Operator.presenceImplications.readOrdinals),
			len(equation.Operator.presenceImplications.projectionOrdinals), span.count)
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
	_, formalFactors, err := afterRegions[0].evaluator.productFactors()
	if err != nil {
		t.Fatal(err)
	}
	formalChild, err := body.productDomain.RekeyStructuralKeyFormal(program.formalFibers.spans[0].rekey, childKey)
	if err != nil {
		t.Fatal(err)
	}
	pathFamily, _ := body.productDomain.PathEvidenceCoordinateFamily()
	pathPosition := equation.Operator.presenceImplications.positions[pathFamily.Lane()]
	formalKeys := program.formalFibers.spans[0].keys
	childSlot, err := body.productDomain.PresenceImplicationRefinementCoordinateSlot(formalKeys, formalChild)
	if err != nil {
		t.Fatal(err)
	}
	skeleton, scalars, err := body.productDomain.DecomposeCoordinateFamily(formalFactors[pathPosition], pathFamily, formalKeys)
	if err != nil {
		t.Fatal(err)
	}
	reads, err := body.productDomain.SealCoordinateFactorInventory(formalKeys, []state.CoordinateSlot{childSlot})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := body.productDomain.SealCoordinateFactorInventory(formalKeys, nil)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := state.SealCoordinatePathEvidenceAuthority(
		body.productDomain, formalKeys, nil, nil, reads, empty, false, false,
		func(FormalSlot) bool { return false },
	)
	if err != nil {
		t.Fatal(err)
	}
	carrier, err := state.OpenCoordinatePathEvidenceCarrier(
		body.productDomain, skeleton, scalars, state.ValueFactor[FormalSlot]{}, true,
		authority, state.PathDescendantMutationFactors{},
	)
	if err != nil {
		t.Fatal(err)
	}
	gotChild, valid := carrier.ReadPath(formalChild)
	if !valid || !product.Equal(reg, gotChild, product.Bottom(reg)) {
		t.Fatalf("formal N2 descendant barrier retained stale child evidence: valid=%t formal=%v", valid, product.PresenceOf(gotChild))
	}
	for _, group := range span.groupDescriptors() {
		if group.kind == formalFiberGroupValues || group.lane == pathFamily.Lane() {
			continue
		}
		prior, priorErr := beforeRegions[0].evaluator.laneFactor(group)
		next, nextErr := afterRegions[0].evaluator.laneFactor(group)
		equal, equalErr := body.productDomain.LaneEqual(prior, next)
		if priorErr != nil || nextErr != nil || equalErr != nil || !equal {
			t.Fatalf("formal N2 changed unrelated lane %s: %v/%v/%v", group.lane.ID(), priorErr, nextErr, equalErr)
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	priorContext := execution.algebra.ctx
	priorDecisions := len(execution.algebra.decisions.nodes)
	execution.algebra.ctx = canceled
	rolled, cancelErr := execution.algebra.applyFormalPresenceImplications(equation.Operator, before)
	execution.algebra.ctx = priorContext
	if cancelErr == nil || !rolled.bottom() || len(execution.algebra.decisions.nodes) != priorDecisions {
		t.Fatalf("canceled formal N2 published work: err=%v bottom=%t decisions=%d/%d", cancelErr, rolled.bottom(), len(execution.algebra.decisions.nodes), priorDecisions)
	}
}

func TestFormalPresenceImplicationsContradictionStopsLaterBarrierStages(t *testing.T) {
	reg := standard.Registry()
	base := formalRootInputTestProgram(t, reg)
	body := &base.bodies[0]
	point := body.graph.Entry()
	const x, y, z = symbol.ID(101), symbol.ID(102), symbol.ID(103)
	xPath, yPath, zPath := pathdom.NewPath(x, "x"), pathdom.NewPath(y, "y"), pathdom.NewPath(z, "z")
	builder := visibility.NewBuilder()
	builder.Define(point, x, "x")
	builder.Define(point, y, "y")
	builder.Define(point, z, "z")
	resolver := visibility.NewResolver(builder.Build())
	body.keys = resolver.KeySpace()
	body.pathSemantics = factapply.NewPathSemanticAuthority(resolver, nil, typevalue.NewCache())
	roots, err := sealRelationRootCarrierWithAmbients(body.plan, body.keys, body.relation.shape, []AmbientRoot{{Symbol: symbol.ID(104)}})
	if err != nil {
		t.Fatal(err)
	}
	body.roots = roots
	falseValue := typevalue.LiteralBool(reg, false)
	presentValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	transaction := factapply.PlanPathValuePresenceImplicationTransaction(factflow.NewFacts(factflow.FactsInput{
		PathValuePresenceImplications: map[cfg.Point]factflow.PathValuePresenceImplicationSet{
			point: factflow.NewPathValuePresenceImplicationSet(
				factflow.NewPathValuePresenceImplication(xPath, falseValue, zPath, presence.Absent()),
				// This later descendant-barrier stage would resurrect a partially
				// committed tuple if formal N2 continued after contradiction.
				factflow.NewPathValuePresenceImplication(xPath, falseValue, yPath, presence.Present()),
			),
		},
	}), point)
	input := state.Reachable(body.productDomain.Lattice().Bottom()).
		WriteValue(reg, statekey.SymbolValue(x), falseValue).
		WriteValue(reg, statekey.SymbolValue(y), product.Top()).
		WriteValue(reg, statekey.SymbolValue(z), presentValue)
	body.entrySeedPlan = state.NewEntrySeedPlan([]state.ValueSeed{
		{Slot: statekey.SymbolValue(x), Value: falseValue},
		{Slot: statekey.SymbolValue(y), Value: product.Top()},
		{Slot: statekey.SymbolValue(z), Value: presentValue},
		{Slot: statekey.SymbolValue(104), Value: product.Top()},
	})
	body.initialStatePlan = testInitialStatePlan(t, body.body, body.graph, state.NewInitialStateSeed(state.InitialCoordinate(point), input))
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepPresenceImplications, point: point, presence: transaction}}, next: 2},
		{kind: relationNodeOutcome, outcome: 1},
	})
	cell := formalRelationCell{Variable: 1, Kind: formalRelationCellStep, Root: 1, Step: 1}
	equation, ok := program.formalTemplate.equation(cell)
	if !ok || equation.Operator.presenceImplications == nil {
		t.Fatal("formal contradictory N2 capability was not admitted")
	}
	if len(equation.Operator.presenceImplications.writeOrdinals) == 0 || equation.Operator.presenceImplications.writeOrdinals[0] != 0 {
		t.Fatal("formal contradictory N2 did not declare Care")
	}
	execution, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	after := execution.values[cell]
	if !after.bottom() {
		t.Fatal("formal contradictory N2 did not publish sparse product Bottom through Care")
	}
}
