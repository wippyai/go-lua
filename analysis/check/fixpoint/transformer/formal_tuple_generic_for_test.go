package transformer

import (
	"context"
	"sort"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

type formalGenericForTestAuthority struct {
	config state.GenericForFactorConfig
}

func (a formalGenericForTestAuthority) PrepareGenericForFactorTransaction(_ transfer.NodeContext, _ factapply.GenericForOperation, domain state.ProductDomain) (state.GenericForFactorTransaction, error) {
	return domain.PrepareGenericForFactorTransaction(a.config)
}

func TestFormalGenericForUsesCanonicalSparseFactorTransaction(t *testing.T) {
	reg := standard.Registry()
	base := formalRootInputTestProgram(t, reg)
	body := &base.bodies[0]
	point := body.graph.Exit()
	const target = symbol.ID(101)
	op, ok := operationplan.NewGenericForOperation(0, target, target, nil, nil)
	if !ok {
		t.Fatal("generic-for operation")
	}
	body.plan = body.plan.WithExtensions([]operationplan.ExtensionInput{{
		Point: point, Kind: operationplan.BodyGenericFor, GenericFor: op,
	}})
	body.nodeReads = make([][]cfg.Point, body.graph.Size())
	body.genericForMembership = formalGenericForTestAuthority{config: state.GenericForFactorConfig{
		Keys: body.keys, VariableIndex: 0, Target: body.keys.FromPath(pathdom.Path{Symbol: target}),
	}}
	old := typevalue.LiteralString(reg, "old")
	want := typevalue.LiteralString(reg, "iterator-result")
	body.entrySeedPlan = state.NewEntrySeedPlan([]state.ValueSeed{
		{Slot: statekey.SymbolValue(101), Value: old},
		{Slot: statekey.SymbolValue(102), Value: want},
	})
	seed := state.Reachable(state.State{}).
		WriteValue(reg, statekey.SymbolValue(101), old).
		WriteValue(reg, statekey.SymbolValue(102), want)
	body.initialStatePlan = testInitialStatePlan(t, body.body, body.graph,
		state.NewInitialStateSeed(state.InitialCoordinate(body.graph.Entry()), seed))
	arena := body.relation.code.terms
	projection, present := arena.middleValue(statekey.SymbolValue(102))
	if !present || projection == 0 {
		t.Fatal("capture Middle projection")
	}
	fallback, present := arena.middleValue(statekey.SymbolValue(target))
	if !present || fallback == 0 {
		t.Fatal("capture GenericFor target carry")
	}
	// This executor fixture predates the sealed compiler and intentionally uses
	// a direct Middle projection to isolate the factor transaction. Production
	// descriptors are created only by sealGenericForIdentityPublication beside
	// the canonical GenericFor term constructor.
	identityPublication := frozenGenericForIdentityPublication{
		target: statekey.SymbolValue(target), projection: projection, finiteSources: []ValueTerm{fallback},
		projectionIdentity: genericForProjectionIdentityNoFinite, sealed: true,
	}
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{
			kind: boundaryStepGenericFor, point: point, access: []valueAccessTerm{{term: projection}}, genericIdentity: identityPublication,
		}}, next: 2},
		{kind: relationNodeOutcome, outcome: 1},
	})
	execution, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	cell := formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
	equation, ok := program.formalTemplate.equation(cell)
	if !ok || equation.Operator.stepCapability != formalRelationStepCapabilityGenericFor || equation.Operator.genericFor == nil {
		t.Fatal("formal GenericFor capability was not admitted")
	}
	span, _, _, ok := execution.algebra.span(cell.Variable)
	if !ok {
		t.Fatal("formal GenericFor span")
	}
	sparseWidth := len(equation.Operator.genericFor.currentOrdinals) +
		len(equation.Operator.genericFor.sourceOrdinals) + len(equation.Operator.genericFor.demands)
	fullWidth := 2*span.count + len(equation.Operator.genericFor.demands)
	if sparseWidth >= fullWidth {
		t.Fatalf("GenericFor sparse correlation width = %d, full width = %d", sparseWidth, fullWidth)
	}
	t.Logf("GenericFor correlation width: %d sparse roots, %d full-product roots", sparseWidth, fullWidth)
	regions, err := execution.algebra.tupleLeafRegions(execution.values[cell])
	if err != nil || len(regions) != 1 {
		t.Fatalf("GenericFor regions = %d, %v", len(regions), err)
	}
	values, err := regions[0].evaluator.valuesFactor()
	if err != nil {
		t.Fatal(err)
	}
	got := values.Values[equation.Operator.genericFor.target]
	if !product.Equal(reg, got, want) || product.Equal(reg, got, old) {
		t.Fatalf("formal GenericFor target = %#v, want %#v", got, want)
	}
	operands, err := partitionFormalRelationStepOperands(equation)
	if err != nil {
		t.Fatal(err)
	}
	inputRegions, err := execution.algebra.tupleLeafRegions(execution.values[operands.Flow.Source.cell])
	if err != nil || len(inputRegions) != 1 {
		t.Fatalf("input regions = %d, %v", len(inputRegions), err)
	}
	for _, group := range regions[0].evaluator.layout.nonValues {
		before, beforeErr := inputRegions[0].evaluator.laneFactor(group)
		after, afterErr := regions[0].evaluator.laneFactor(group)
		equal, equalErr := regions[0].evaluator.authority.product.LaneEqual(before, after)
		if beforeErr != nil || afterErr != nil || equalErr != nil || !equal {
			t.Fatalf("GenericFor changed unrelated lane %q: %v/%v/%v", group.lane.ID(), beforeErr, afterErr, equalErr)
		}
	}
}

func TestFormalGenericForProjectionUsesRegisteredFactorAccess(t *testing.T) {
	fixture := newFormalTupleLeafEvaluatorFixture(t)
	span, ok := fixture.program.formalFibers.span(1)
	if !ok {
		t.Fatal("formal span")
	}
	values, ok := span.valuesGroup()
	if !ok {
		t.Fatal("Values group")
	}
	valuesTop, ok := values.top()
	if !ok {
		t.Fatal("Values Top")
	}
	access, groups, err := freezeFormalValueFactorAccess(fixture.program, 1, fixture.unsupported)
	if err != nil {
		t.Fatal(err)
	}
	if access.Lanes.Len() == 0 || len(groups) != access.Lanes.Len() {
		t.Fatalf("GenericFor projection factor access = %#v/%d", access, len(groups))
	}
	ordinals := append([]formalFiberOrdinal(nil), values.descriptor.members...)
	for _, group := range groups {
		ordinals = append(ordinals, group.members...)
	}
	sort.Slice(ordinals, func(i, j int) bool { return ordinals[i] < ordinals[j] })
	plan := &formalGenericForStep{
		projection: formalQualifiedBinding{
			value: relationArenaValueRef{owner: 1, arena: fixture.callerArena, term: fixture.unsupported},
		},
		projectionAccess: access, projectionFactors: groups,
		values: values.descriptor, valuesTop: valuesTop,
		sourceOrdinals: ordinals,
	}
	regions, err := fixture.algebra.partitionSparseLeafViewsUnderCare([]formalSparseTupleProjection{{
		tuple: fixture.callerTuple, ordinals: plan.sourceOrdinals,
	}}, plan.demands)
	if err != nil || len(regions) != 1 || len(regions[0].views) != 1 {
		t.Fatalf("GenericFor projection regions = %#v, %v", regions, err)
	}
	got, err := regions[0].views[0].evaluateGenericFor(plan)
	ground, concrete := got.concrete()
	if err != nil || !concrete || !product.BelongsToRegistry(fixture.reg, ground) {
		t.Fatalf("factor-backed GenericFor projection = %#v, %v", got, err)
	}
}
