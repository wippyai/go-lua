package transformer

import (
	"context"
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestFormalEnvironmentWriteExecutesCanonicalStep(t *testing.T) {
	base := formalRootInputTestProgram(t, standard.Registry())
	formalEnvironmentWriteSealRootCarrier(t, base)
	arena := base.bodies[0].relation.code.terms
	guard := arena.Truthy(arena.Root(Root{Kind: RootParam, Index: 0}))
	value := arena.SelectValue(guard,
		arena.Root(Root{Kind: RootCapture, Index: 0}),
		arena.Root(Root{Kind: RootGlobal, Index: 0}),
	)
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{
			kind: boundaryStepEnvironmentWrite, slot: statekey.SymbolValue(101), value: value,
		}}, next: 2},
		{kind: relationNodeBottom},
	})
	if result, err := executeFormalRelation(context.Background(), program); err != nil || result == nil {
		t.Fatalf("formal EnvironmentWrite = %#v, %v", result, err)
	}
}

func TestFormalEnvironmentWritePreservesGuardCorrelationAndOnlyTargetSlotChanges(t *testing.T) {
	fixture := formalEnvironmentWriteFixture(t)
	plan := fixture.operator.environmentWrite
	span, directory, _, _ := fixture.algebra.span(1)
	values := formalValuesFiberGroup{descriptor: plan.values}
	selected := make(map[formalFiberOrdinal]struct{}, len(plan.readOrdinals))
	for _, ordinal := range plan.readOrdinals {
		selected[ordinal] = struct{}{}
	}
	want := make(map[formalFiberOrdinal]struct{})
	for _, member := range []formalFiberGroupMember{plan.valuesTop, plan.target} {
		ordinal, ok := member.address(member.group)
		if !ok {
			t.Fatal("EnvironmentWrite member has no ordinal")
		}
		want[ordinal] = struct{}{}
	}
	readSlots, err := fixture.algebra.program.bodies[0].valueTermReadSlots(plan.value.value.term)
	if err != nil {
		t.Fatal(err)
	}
	for _, concrete := range readSlots {
		slot, ok := formalMiddleSlotForStateKey(fixture.algebra.program, &fixture.algebra.program.bodies[0], concrete)
		if !ok {
			t.Fatalf("source slot %d has no formal identity", concrete)
		}
		member, ok := values.slot(slot)
		if !ok {
			t.Fatalf("source slot %d has no Values member", concrete)
		}
		ordinal, _ := member.address(member.group)
		want[ordinal] = struct{}{}
	}
	if len(selected) != len(want) || len(selected) >= span.count {
		t.Fatalf("EnvironmentWrite correlated width = %d/%d, want exact sparse %d", len(selected), span.count, len(want))
	}
	t.Logf("EnvironmentWrite correlated width %d -> %d descriptors", span.count, len(selected))
	for ordinal := range want {
		if _, present := selected[ordinal]; !present {
			t.Fatalf("EnvironmentWrite omitted required ordinal %d", ordinal)
		}
	}
	result, err := fixture.algebra.applyFormalEnvironmentWrite(fixture.operator, fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	condition, err := fixture.algebra.decisionForGuard(1, fixture.operator.scope, fixture.arena, fixture.guard)
	if err != nil {
		t.Fatal(err)
	}
	complement, err := formalDecisionBooleanNot(fixture.algebra, condition)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		care  decisionRef
		value product.Value
	}{{"true", condition, fixture.left}, {"false", complement, fixture.right}} {
		t.Run(test.name, func(t *testing.T) {
			guarded, restrictErr := fixture.algebra.restrictTupleCare(result, test.care)
			if restrictErr != nil {
				t.Fatal(restrictErr)
			}
			got := formalEnvironmentWriteReadTarget(t, fixture, guarded)
			if !product.Equal(fixture.algebra.program.registry, got, test.value) {
				t.Fatalf("guarded target = %v, want %v", got, test.value)
			}
		})
	}

	target, _ := fixture.operator.environmentWrite.target.address(fixture.operator.environmentWrite.target.group)
	for ordinal := 0; ordinal < span.count; ordinal++ {
		before, beforeErr := directory.valueAt(fixture.input.root, formalFiberOrdinal(ordinal))
		after, afterErr := directory.valueAt(result.root, formalFiberOrdinal(ordinal))
		if beforeErr != nil || afterErr != nil {
			t.Fatalf("descriptor %d read = %v/%v", ordinal, beforeErr, afterErr)
		}
		if formalFiberOrdinal(ordinal) == target {
			if before == after {
				t.Fatal("EnvironmentWrite did not change its exact target slot")
			}
			continue
		}
		if before != after {
			t.Fatalf("EnvironmentWrite changed unrelated descriptor %d: %d -> %d", ordinal, before, after)
		}
	}
}

func TestFormalEnvironmentWriteBottomTopAndMalformedOwnership(t *testing.T) {
	fixture := formalEnvironmentWriteFixture(t)
	if got, err := fixture.algebra.applyFormalEnvironmentWrite(fixture.operator, formalRelationTuple{}); err != nil || !got.bottom() {
		t.Fatalf("Bottom EnvironmentWrite = %#v, %v", got, err)
	}

	span, _, _, _ := fixture.algebra.span(1)
	values, _ := span.valuesGroup()
	top, err := fixture.algebra.writeValuesFactor(fixture.input, values, state.ValueFactor[FormalSlot]{Top: true})
	if err != nil {
		t.Fatal(err)
	}
	topResult, err := fixture.algebra.applyFormalEnvironmentWrite(fixture.operator, top)
	if err != nil || !fixture.algebra.same(top, topResult) {
		t.Fatalf("Values Top absorbed write = %#v, %v", topResult, err)
	}

	malformed := fixture.operator
	plan := *malformed.environmentWrite
	plan.target = formalFiberGroupMember{}
	malformed.environmentWrite = &plan
	if got, err := fixture.algebra.applyFormalEnvironmentWrite(malformed, fixture.input); err == nil || !got.bottom() {
		t.Fatalf("foreign target published %#v, %v", got, err)
	}

	badScope := fixture.operator
	plan = *badScope.environmentWrite
	plan.value.scope = loopMuTerm(999)
	badScope.environmentWrite = &plan
	if got, err := fixture.algebra.applyFormalEnvironmentWrite(badScope, fixture.input); err == nil || !got.bottom() {
		t.Fatalf("foreign scope published %#v, %v", got, err)
	}
}

func TestFormalTupleGuardDemandsRejectForeignEqualOrdinalArena(t *testing.T) {
	left := formalEnvironmentWriteFixture(t)
	right := formalEnvironmentWriteFixture(t)
	foreign := formalQualifiedGuardDemand{owner: 1, scope: right.operator.scope, arena: right.arena, guard: right.guard}
	if regions, err := left.algebra.tupleLeafRegionsWithGuardDemands(left.input, []formalQualifiedGuardDemand{foreign}); err == nil || regions != nil {
		t.Fatalf("foreign equal-ordinal guard demand = %#v, %v", regions, err)
	}
	valid := formalQualifiedGuardDemand{owner: 1, scope: left.operator.scope, arena: left.arena, guard: left.guard}
	regions, err := left.algebra.tupleLeafRegionsWithGuardDemands(left.input, []formalQualifiedGuardDemand{valid, valid})
	if err != nil || len(regions) != 2 || len(left.algebra.guards) != 1 {
		t.Fatalf("deduplicated guard demand = %d regions/%d cache, %v", len(regions), len(left.algebra.guards), err)
	}
}

type formalEnvironmentWriteTestFixture struct {
	algebra          *formalTupleAlgebra
	operator         formalRelationOperatorRef
	input            formalRelationTuple
	arena            *Arena
	guard            Guard
	target           FormalSlot
	left, right, old product.Value
}

func formalEnvironmentWriteFixture(t *testing.T) formalEnvironmentWriteTestFixture {
	t.Helper()
	reg := standard.Registry()
	base := formalRootInputTestProgram(t, reg)
	formalEnvironmentWriteSealRootCarrier(t, base)
	arena := base.bodies[0].relation.code.terms
	guard := arena.Truthy(arena.Root(Root{Kind: RootParam, Index: 0}))
	value := arena.SelectValue(guard,
		arena.Root(Root{Kind: RootCapture, Index: 0}),
		arena.Root(Root{Kind: RootGlobal, Index: 0}),
	)
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeSequence, steps: []boundaryStep{{
			kind: boundaryStepEnvironmentWrite, slot: statekey.SymbolValue(101), value: value,
		}}, next: 2},
		{kind: relationNodeBottom},
	})
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	rootEquation, _ := program.formalTemplate.equation(program.formalRegion.roots[0])
	input, err := algebra.instantiateRootEquation(rootEquation)
	if err != nil {
		t.Fatal(err)
	}
	span, _, _, _ := algebra.span(1)
	values, ok := span.valuesGroup()
	if !ok {
		t.Fatal("missing Values group")
	}
	body := &program.bodies[0]
	param, _ := formalMiddleSlotForStateKey(program, body, statekey.SymbolValue(101))
	capture, _ := formalMiddleSlotForStateKey(program, body, statekey.SymbolValue(102))
	global, _ := formalMiddleSlotForStateKey(program, body, statekey.SymbolValue(103))
	ambient, _ := formalMiddleSlotForStateKey(program, body, statekey.SymbolValue(104))
	middle, _ := arena.middleRoot(statekey.SymbolValue(101))
	target, _ := program.formalSlots.Slot(body.body, middle)
	left := typevalue.LiteralString(reg, "left")
	right := typevalue.LiteralString(reg, "right")
	old := typevalue.LiteralString(reg, "old")
	input, err = algebra.writeValuesFactor(input, values, state.ValueFactor[FormalSlot]{Values: map[FormalSlot]product.Value{
		param: typevalue.LiteralString(reg, "condition"), capture: left, global: right,
		ambient: typevalue.LiteralString(reg, "untouched"), target: old,
	}})
	if err != nil {
		t.Fatal(err)
	}
	stepCell := formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
	stepEquation, ok := program.formalTemplate.equation(stepCell)
	if !ok || stepEquation.Operator.environmentWrite == nil {
		t.Fatal("missing formal EnvironmentWrite operator")
	}
	return formalEnvironmentWriteTestFixture{
		algebra: algebra, operator: stepEquation.Operator, input: input, arena: arena, guard: guard,
		target: target, left: left, right: right, old: old,
	}
}

func formalEnvironmentWriteSealRootCarrier(t *testing.T, program *RelationProgram) {
	t.Helper()
	body := &program.bodies[0]
	roots, err := sealRelationRootCarrierWithAmbients(body.plan, body.keys, body.relation.shape, []AmbientRoot{{Symbol: symbol.ID(104)}})
	if err != nil {
		t.Fatal(err)
	}
	body.roots = roots
}

func formalEnvironmentWriteReadTarget(t *testing.T, fixture formalEnvironmentWriteTestFixture, tuple formalRelationTuple) product.Value {
	t.Helper()
	regions, err := fixture.algebra.tupleLeafRegions(tuple)
	if err != nil || len(regions) != 1 {
		t.Fatalf("target regions = %d, %v", len(regions), err)
	}
	group := fixture.operator.environmentWrite.target.group
	leaves, err := regions[0].evaluator.leaves.group(group)
	if err != nil {
		t.Fatal(err)
	}
	factor, err := fixture.algebra.materializeValuesGroup(regions[0].evaluator.authority, group, leaves)
	if err != nil {
		t.Fatal(err)
	}
	if factor.Top {
		return product.Top()
	}
	return factor.Values[fixture.target]
}
