package transformer

import (
	"context"
	"errors"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFormalEffectPathReplacementPreservesLexicalRepeatedWriteOrder(t *testing.T) {
	reg := standard.Registry()
	first := typevalue.LiteralString(reg, "first")
	second := typevalue.LiteralString(reg, "second")
	program := formalPathReplacementTestProgram(t, nil, first, second)
	result, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	firstCell := formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
	secondCell := formalRelationCell{Variable: 1, Root: 1, Step: 2, Kind: formalRelationCellStep}
	formalEffectTestPathValue(t, result, program.formalTemplate.equations[firstCellIndex(t, program, firstCell)].Operator, result.values[firstCell], first)
	formalEffectTestPathValue(t, result, program.formalTemplate.equations[firstCellIndex(t, program, secondCell)].Operator, result.values[secondCell], second)
}

func TestFormalEffectPathReplacementBottomStaysBottom(t *testing.T) {
	reg := standard.Registry()
	program := formalPathReplacementTestProgram(t, nil, typevalue.LiteralString(reg, "value"))
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	cell := formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
	equation, ok := program.formalTemplate.equation(cell)
	if !ok {
		t.Fatal("Effect equation")
	}
	got := evaluateFormalRelationEquation(algebra, equation, func(formalRelationCell) formalRelationTuple { return formalRelationTuple{} })
	if !got.bottom() || algebra.err() != nil {
		t.Fatalf("Bottom Effect = %#v, err=%v", got, algebra.err())
	}
}

func TestFormalEffectPathReplacementRetainsGuardCorrelation(t *testing.T) {
	reg := standard.Registry()
	value := typevalue.LiteralString(reg, "written")
	guarded := true
	program := formalPathReplacementTestProgram(t, &guarded, value)
	result, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	cell := formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
	equation, ok := program.formalTemplate.equation(cell)
	if !ok {
		t.Fatal("guarded Effect equation")
	}
	regions, err := result.algebra.tupleLeafRegions(result.values[cell])
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) < 2 {
		t.Fatalf("guarded Effect regions = %d, want both execute and skip", len(regions))
	}
	step := equation.Operator.code.nodes[equation.Operator.root].steps[equation.Operator.step-1]
	seenTrue, seenFalse := false, false
	for _, region := range regions {
		canTrue, canFalse, exact := region.evaluator.exactGuardPossibilities(1, equation.Operator.code.terms, equation.Operator.scope, step.guard)
		if !exact || canTrue == canFalse {
			t.Fatalf("guard region true=%t false=%t exact=%t", canTrue, canFalse, exact)
		}
		got := formalEffectTestLeafPathValue(t, result.algebra, equation.Operator, region.evaluator)
		if canTrue {
			seenTrue = true
			if !product.Equal(reg, got, value) {
				t.Fatalf("true guard path value = %#v, want written", got)
			}
		} else {
			seenFalse = true
			if product.Equal(reg, got, value) {
				t.Fatal("false guard executed path replacement")
			}
		}
	}
	if !seenTrue || !seenFalse {
		t.Fatalf("guard alternatives seen true=%t false=%t", seenTrue, seenFalse)
	}
}

func TestFormalEffectFalseGuardReusesPhysicalInputRoot(t *testing.T) {
	reg := standard.Registry()
	guarded := true
	program := formalPathReplacementTestProgram(t, &guarded, typevalue.LiteralString(reg, "unreachable-write"))
	execution, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	cell := formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
	equation, ok := program.formalTemplate.equation(cell)
	if !ok || equation.Operator.pathReplacement == nil {
		t.Fatal("guarded Effect equation")
	}
	operands, err := partitionFormalRelationStepOperands(equation)
	if err != nil {
		t.Fatal(err)
	}
	input := execution.values[operands.Flow.Source.cell]
	step := equation.Operator.code.nodes[equation.Operator.root].steps[equation.Operator.step-1]
	guard, err := execution.algebra.decisionForGuard(input.variable, equation.Operator.scope, equation.Operator.code.terms, step.guard)
	if err != nil {
		t.Fatal(err)
	}
	falseGuard, err := formalDecisionBooleanNot(execution.algebra, guard)
	if err != nil {
		t.Fatal(err)
	}
	input, err = execution.algebra.restrictTupleCare(input, falseGuard)
	if err != nil || input.bottom() {
		t.Fatalf("false-guard input is unavailable: bottom=%t err=%v", input.bottom(), err)
	}
	got, err := execution.algebra.applyFormalPathReplacement(equation.Operator, input)
	if err != nil {
		t.Fatal(err)
	}
	if got.variable != input.variable || got.root.owner != input.root.owner || got.root.ref != input.root.ref {
		t.Fatalf("false-guard Effect rebuilt its input root: before=%#v after=%#v", input, got)
	}
}

func TestFormalEffectPathReplacementPartitionsValueSelectGuard(t *testing.T) {
	reg := standard.Registry()
	whenTrue := typevalue.LiteralString(reg, "selected-true")
	whenFalse := typevalue.LiteralString(reg, "selected-false")
	program := formalPathReplacementTestProgram(t, nil, whenTrue)
	body := &program.bodies[0]
	code, arena, effects := body.relation.code, body.relation.arena, body.relation.effects
	arena.sealed, effects.sealed, code.sealed = false, false, false
	guard := arena.Truthy(arena.Root(Root{Kind: RootParam}))
	selected := arena.SelectValue(guard, arena.Constant(whenTrue), arena.Constant(whenFalse))
	if guard == 0 || selected == 0 {
		t.Fatal("selected Effect value")
	}
	step := &code.nodes[1].steps[0]
	effects.nodes[step.effect].pathStoreAssignment.Value = selected
	arena.Seal()
	effects.Seal()
	code.sealed = true
	refreezeFormalEffectTestProgram(t, program)

	result, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	cell := formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
	equation, ok := program.formalTemplate.equation(cell)
	if !ok {
		t.Fatal("selected Effect equation")
	}
	regions, err := result.algebra.tupleLeafRegionsWithGuardDemands(result.values[cell], equation.Operator.pathReplacement.demands)
	if err != nil {
		t.Fatal(err)
	}
	seenTrue, seenFalse := false, false
	for _, region := range regions {
		canTrue, canFalse, exact := region.evaluator.exactGuardPossibilities(1, arena, equation.Operator.scope, guard)
		if !exact || canTrue == canFalse {
			t.Fatalf("select guard region true=%t false=%t exact=%t", canTrue, canFalse, exact)
		}
		got := formalEffectTestLeafPathValue(t, result.algebra, equation.Operator, region.evaluator)
		if canTrue {
			seenTrue = true
			if !product.Equal(reg, got, whenTrue) {
				t.Fatalf("true select path value = %#v", got)
			}
		} else {
			seenFalse = true
			if !product.Equal(reg, got, whenFalse) {
				t.Fatalf("false select path value = %#v", got)
			}
		}
	}
	if !seenTrue || !seenFalse {
		t.Fatalf("select alternatives seen true=%t false=%t", seenTrue, seenFalse)
	}
}

func TestFormalEffectStaticPathStoreFreezesOneRegisteredTransaction(t *testing.T) {
	reg := standard.Registry()
	program := formalPathReplacementTestProgram(t, nil, typevalue.LiteralString(reg, "value"))
	code := program.bodies[0].relation.code
	step := &code.nodes[1].steps[0]
	node := &code.effects.nodes[step.effect]
	node.pathStoreStatic = node.pathStoreAssignment
	node.pathStoreHasStatic = true
	program.formalTemplate = nil
	template, err := freezeFormalRelationTemplate(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalTemplate = template
	cell := formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
	equation, ok := program.formalTemplate.equation(cell)
	if !ok || equation.Operator.stepCapability != formalRelationStepCapabilityPathReplacement ||
		equation.Operator.pathReplacement == nil || !equation.Operator.pathReplacement.hasAssignment ||
		!equation.Operator.pathReplacement.hasStatic || !equation.Operator.pathReplacement.staticPlan.Valid() {
		t.Fatalf("combined PathStore transaction = %#v", equation.Operator)
	}
}

func TestFormalEffectObjectMaterializationUsesRegisteredSparseGraphLaw(t *testing.T) {
	reg := standard.Registry()
	program := formalPathReplacementTestProgram(t, nil, typevalue.LiteralString(reg, "discarded"))
	body := &program.bodies[0]
	code, arena, effects := body.relation.code, body.relation.arena, body.relation.effects
	arena.sealed, effects.sealed, code.sealed = false, false, false
	id := identity.ID{Kind: "formal.effect.object", Site: "materialization", Index: 1}
	root := identityvalue.Present(reg, id)
	member := typevalue.LiteralString(reg, "member")
	effect, err := effects.ObjectMaterialization(PathStoreObjectConfig{Heaps: []PathStoreHeapObjectConfig{{
		Root: arena.Constant(root), StableShape: true,
		Members: []PathStoreHeapMemberConfig{{Suffix: []segment.Segment{{Kind: segment.SegmentField, Name: "value"}}, Value: arena.Constant(member)}},
	}}}, EffectSite{Owner: 1, Ordinal: 1})
	if err != nil {
		t.Fatal(err)
	}
	code.nodes[1].steps = []boundaryStep{{kind: boundaryStepEffect, effect: effect}}
	arena.Seal()
	effects.Seal()
	code.sealed = true
	refreezeFormalTestStaticTopology(t, program)
	refreezeFormalEffectTestProgram(t, program)

	execution, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	cell := formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
	equation, ok := program.formalTemplate.equation(cell)
	if !ok || equation.Operator.objectMaterialization == nil || equation.Operator.stepCapability != formalRelationStepCapabilityObjectMaterialization {
		t.Fatal("formal object materialization adapter is absent")
	}
	regions, err := execution.algebra.tupleLeafRegions(execution.values[cell])
	if err != nil || len(regions) != 1 {
		t.Fatalf("object materialization regions=%d err=%v", len(regions), err)
	}
	span, _, _, ok := execution.algebra.span(1)
	if !ok {
		t.Fatal("formal object span")
	}
	objectGroups := make([]formalFiberGroupDescriptor, 0)
	participants := body.productDomain.ObjectMutationParticipantLanes()
	for _, group := range span.groupDescriptors() {
		for _, lane := range participants {
			if group.lane == lane {
				objectGroups = append(objectGroups, group)
			}
		}
	}
	if got := len(objectGroups); got != len(participants) || got != 2 {
		t.Fatalf("object materialization sparse width=%d, want exact 2", got)
	}
	objectTerm := equation.Operator.objectMaterialization.objects[0].identity
	symbolicRoot := identityvalue.WithExactTerm(reg, root, objectTerm)
	constructor, err := body.productDomain.PrepareObjectConstructorPlan(span.keys, []state.ObjectConstructorShape{{
		Identity: objectTerm, StableShape: true,
		MemberSuffixes: [][]segment.Segment{{{Kind: segment.SegmentField, Name: "value"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want, err := body.productDomain.ApplyObjectConstructor(constructor, []state.ObjectConstructorValues{{Root: symbolicRoot, Members: []product.Value{member}}}, state.Reachable(state.State{}))
	if err != nil {
		t.Fatal(err)
	}
	wantFactors, err := body.productDomain.DecomposeLanes(want, body.productDomain.ObjectMutationParticipantLanes())
	if err != nil {
		t.Fatal(err)
	}
	objectLanes := make(map[state.ProductLane]struct{}, len(objectGroups))
	sawHeap := false
	complete, err := regions[0].evaluator.completeLeaves()
	if err != nil {
		t.Fatal(err)
	}
	for groupIndex, group := range objectGroups {
		objectLanes[group.lane] = struct{}{}
		factor, err := execution.algebra.materializeFormalEffectLane(regions[0].evaluator.authority, span, group, complete)
		if err != nil {
			t.Fatal(err)
		}
		equal, err := body.productDomain.LaneEqual(factor, wantFactors[groupIndex])
		if err != nil || !equal {
			t.Fatalf("formal object lane %s differs from concrete factor: equal=%t err=%v", group.lane.ID(), equal, err)
		}
		if group.lane.ID() != state.LaneHeapTableIdentity {
			continue
		}
		_, roots, members, err := body.productDomain.DecomposeHeapTableIdentity(factor, span.keys)
		if err != nil {
			t.Fatal(err)
		}
		if len(roots) != 1 || roots[0].IdentityTerm() != objectTerm || !product.Equal(reg, roots[0].Value(), symbolicRoot) || len(members) != 1 {
			t.Fatalf("formal object graph roots=%d members=%d", len(roots), len(members))
		}
		sawHeap = true
	}
	if !sawHeap {
		t.Fatal("formal object heap factor absent")
	}

	operands, err := partitionFormalRelationStepOperands(equation)
	if err != nil {
		t.Fatal(err)
	}
	input := execution.values[operands.Flow.Source.cell]
	inputRegions, err := execution.algebra.tupleLeafRegions(input)
	if err != nil || len(inputRegions) != 1 {
		t.Fatalf("object materialization input regions=%d err=%v", len(inputRegions), err)
	}
	for _, group := range span.groupDescriptors() {
		if _, written := objectLanes[group.lane]; written && group.kind != formalFiberGroupValues {
			continue
		}
		if group.kind == formalFiberGroupValues {
			before, beforeErr := inputRegions[0].evaluator.valuesFactor()
			after, afterErr := regions[0].evaluator.valuesFactor()
			if beforeErr != nil || afterErr != nil || !state.ValueFactorLattice[FormalSlot](reg).Equal(before, after) {
				t.Fatalf("object materialization changed Values: before=%v after=%v", beforeErr, afterErr)
			}
			continue
		}
		before, beforeErr := inputRegions[0].evaluator.laneFactor(group)
		after, afterErr := regions[0].evaluator.laneFactor(group)
		equal, equalErr := body.productDomain.LaneEqual(before, after)
		if beforeErr != nil || afterErr != nil || equalErr != nil || !equal {
			t.Fatalf("object materialization changed residual lane %s: before=%v after=%v equal=%v", group.lane.ID(), beforeErr, afterErr, equalErr)
		}
	}
}

func TestFormalEffectObjectMaterializationCancellationPublishesNothing(t *testing.T) {
	reg := standard.Registry()
	program := formalPathReplacementTestProgram(t, nil, typevalue.LiteralString(reg, "discarded"))
	body := &program.bodies[0]
	code, arena, effects := body.relation.code, body.relation.arena, body.relation.effects
	arena.sealed, effects.sealed, code.sealed = false, false, false
	id := identity.ID{Kind: "formal.effect.object", Site: "cancel", Index: 1}
	effect, err := effects.ObjectMaterialization(PathStoreObjectConfig{Heaps: []PathStoreHeapObjectConfig{{
		Root: arena.Constant(identityvalue.Present(reg, id)), StableShape: true,
		Members: []PathStoreHeapMemberConfig{{Suffix: []segment.Segment{{Kind: segment.SegmentField, Name: "value"}}, Value: arena.Constant(typevalue.LiteralString(reg, "member"))}},
	}}}, EffectSite{Owner: 1, Ordinal: 1})
	if err != nil {
		t.Fatal(err)
	}
	code.nodes[1].steps = []boundaryStep{{kind: boundaryStepEffect, effect: effect}}
	arena.Seal()
	effects.Seal()
	code.sealed = true
	refreezeFormalTestStaticTopology(t, program)
	refreezeFormalEffectTestProgram(t, program)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := executeFormalRelation(ctx, program)
	if result != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled object materialization published %#v, err=%v", result, err)
	}
}

func TestFormalEffectPathInvalidationMatchesConcreteSparseTransaction(t *testing.T) {
	for _, test := range []struct {
		name  string
		scope InvalidationScope
		width int
	}{{"subtree", InvalidationScopeSubtree, 4}, {"descendants", InvalidationScopeDescendants, 3}} {
		t.Run(test.name, func(t *testing.T) {
			program := formalPathInvalidationTestProgram(t, test.scope)
			execution, err := executeFormalRelation(context.Background(), program)
			if err != nil {
				t.Fatal(err)
			}
			cell := formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
			equation, ok := program.formalTemplate.equation(cell)
			if !ok || equation.Operator.pathInvalidation == nil || equation.Operator.stepCapability != formalRelationStepCapabilityPathInvalidation {
				t.Fatal("formal path invalidation adapter is absent")
			}
			plan := equation.Operator.pathInvalidation
			if len(plan.lanes) != test.width {
				t.Fatalf("path invalidation sparse width=%d, want exact %d", len(plan.lanes), test.width)
			}
			span, _, _, ok := execution.algebra.span(1)
			if !ok {
				t.Fatal("formal invalidation span")
			}
			groups := make(map[state.ProductLane]formalFiberGroupDescriptor, len(plan.lanes))
			for _, group := range span.groupDescriptors() {
				groups[group.lane] = group
			}
			operands, err := partitionFormalRelationStepOperands(equation)
			if err != nil {
				t.Fatal(err)
			}
			inputRegions, err := execution.algebra.tupleLeafRegions(execution.values[operands.Flow.Source.cell])
			if err != nil || len(inputRegions) != 1 {
				t.Fatalf("path invalidation input regions=%d err=%v", len(inputRegions), err)
			}
			outputRegions, err := execution.algebra.tupleLeafRegions(execution.values[cell])
			if err != nil || len(outputRegions) != 1 {
				t.Fatalf("path invalidation output regions=%d err=%v", len(outputRegions), err)
			}
			var ownerFactor state.LaneFactor
			for _, lane := range plan.lanes {
				if lane == plan.owner.Lane() {
					ownerFactor, err = inputRegions[0].evaluator.laneFactor(groups[lane])
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			ownerSkeleton, ownerScalars, err := program.bodies[0].productDomain.DecomposeCoordinateFamily(ownerFactor, plan.owner, span.keys)
			if err != nil {
				t.Fatal(err)
			}
			var subtree state.PathSubtreeMutation
			var descendants state.PathDescendantMutation
			if test.scope == InvalidationScopeSubtree {
				subtree, err = program.bodies[0].productDomain.PrepareCoordinatePathSubtreeMutation(ownerSkeleton, ownerScalars, plan.target)
			} else {
				descendants, err = program.bodies[0].productDomain.PrepareCoordinatePathDescendantMutation(ownerSkeleton, ownerScalars, plan.target)
			}
			if err != nil {
				t.Fatal(err)
			}
			var subtreeWant map[state.LaneOrdinal]state.LaneFactor
			if test.scope == InvalidationScopeSubtree {
				subtreeWant = make(map[state.LaneOrdinal]state.LaneFactor, len(plan.lanes))
				for _, lane := range plan.lanes {
					factor, materializeErr := inputRegions[0].evaluator.laneFactor(groups[lane])
					if materializeErr != nil {
						t.Fatal(materializeErr)
					}
					subtreeWant[lane.Ordinal()] = factor
				}
				bound, bindErr := program.bodies[0].productDomain.BindPathSubtreeMutationFactors(span.keys, func(lane state.ProductLane) (state.LaneFactor, bool) {
					factor, present := subtreeWant[lane.Ordinal()]
					return factor, present && factor.Lane() == lane
				})
				if bindErr != nil {
					t.Fatal(bindErr)
				}
				bound, bindErr = program.bodies[0].productDomain.ApplyPathSubtreeMutationFactors(subtree, bound)
				if bindErr != nil {
					t.Fatal(bindErr)
				}
				for _, factor := range bound.LaneFactors() {
					subtreeWant[factor.Lane().Ordinal()] = factor
				}
				for _, factor := range bound.CoordinateFactors() {
					lane := factor.Family().Lane()
					base := subtreeWant[lane.Ordinal()]
					base, bindErr = program.bodies[0].productDomain.ReplaceCoordinateFamily(base, factor.Skeleton(), factor.Scalars())
					if bindErr != nil {
						t.Fatal(bindErr)
					}
					subtreeWant[lane.Ordinal()] = base
				}
			}
			written := make(map[state.ProductLane]bool, len(plan.lanes))
			for _, lane := range plan.lanes {
				written[lane] = true
				group := groups[lane]
				before, materializeErr := inputRegions[0].evaluator.laneFactor(group)
				if materializeErr != nil {
					t.Fatal(materializeErr)
				}
				var want state.LaneFactor
				if test.scope == InvalidationScopeSubtree {
					want = subtreeWant[lane.Ordinal()]
				} else {
					want, err = program.bodies[0].productDomain.ApplyPathDescendantMutationLane(descendants, before)
				}
				if err != nil {
					t.Fatal(err)
				}
				got, materializeErr := outputRegions[0].evaluator.laneFactor(group)
				if materializeErr != nil {
					t.Fatal(materializeErr)
				}
				equal, equalErr := program.bodies[0].productDomain.LaneEqual(got, want)
				if equalErr != nil || !equal {
					t.Fatalf("formal path invalidation lane %s differs from concrete: equal=%t err=%v", lane.ID(), equal, equalErr)
				}
			}
			for _, group := range span.groupDescriptors() {
				if written[group.lane] && group.kind != formalFiberGroupValues {
					continue
				}
				if group.kind == formalFiberGroupValues {
					before, beforeErr := inputRegions[0].evaluator.valuesFactor()
					after, afterErr := outputRegions[0].evaluator.valuesFactor()
					if beforeErr != nil || afterErr != nil || !state.ValueFactorLattice[FormalSlot](program.registry).Equal(before, after) {
						t.Fatalf("path invalidation changed Values: before=%v after=%v", beforeErr, afterErr)
					}
					continue
				}
				before, beforeErr := inputRegions[0].evaluator.laneFactor(group)
				after, afterErr := outputRegions[0].evaluator.laneFactor(group)
				equal, equalErr := program.bodies[0].productDomain.LaneEqual(before, after)
				if beforeErr != nil || afterErr != nil || equalErr != nil || !equal {
					t.Fatalf("path invalidation changed residual lane %s: before=%v after=%v equal=%v", group.lane.ID(), beforeErr, afterErr, equalErr)
				}
			}
		})
	}
}

func TestFormalEffectPathInvalidationCancellationPublishesNothing(t *testing.T) {
	program := formalPathInvalidationTestProgram(t, InvalidationScopeSubtree)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := executeFormalRelation(ctx, program)
	if result != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled path invalidation published %#v, err=%v", result, err)
	}
}

func TestFormalEffectIndexMutationMatchesCanonicalOrderedTransaction(t *testing.T) {
	reg := standard.Registry()
	keyValue := typevalue.LiteralString(reg, "member")
	storedValue := typevalue.LiteralString(reg, "written")
	program := formalIndexMutationTestProgram(t, keyValue, storedValue)
	body := &program.bodies[0]
	execution, err := executeFormalRootRelation(context.Background(), program, body.body, state.State{})
	if err != nil {
		t.Fatal(err)
	}
	cell := formalRelationCell{Variable: 1, Root: 1, Step: 1, Kind: formalRelationCellStep}
	equation, ok := program.formalTemplate.equation(cell)
	if !ok || equation.Operator.indexMutation == nil || equation.Operator.stepCapability != formalRelationStepCapabilityIndexMutation {
		t.Fatal("formal IndexMutation adapter is absent")
	}
	operands, err := partitionFormalRelationStepOperands(equation)
	if err != nil {
		t.Fatal(err)
	}
	inputRegions, err := execution.algebra.tupleLeafRegions(execution.values[operands.Flow.Source.cell])
	if err != nil || len(inputRegions) != 1 {
		t.Fatalf("formal IndexMutation input regions=%d err=%v", len(inputRegions), err)
	}
	outputRegions, err := execution.algebra.tupleLeafRegions(execution.values[cell])
	if err != nil || len(outputRegions) != 1 {
		t.Fatalf("formal IndexMutation output regions=%d err=%v", len(outputRegions), err)
	}
	beforeValues, _, err := inputRegions[0].evaluator.productFactors()
	if err != nil {
		t.Fatal(err)
	}
	afterValues, _, err := outputRegions[0].evaluator.productFactors()
	if err != nil {
		t.Fatal(err)
	}
	if !state.ValueFactorLattice[FormalSlot](reg).Equal(beforeValues, afterValues) {
		t.Fatal("formal IndexMutation changed the Values axis")
	}
	domain := body.productDomain
	publication, err := execution.Publication(body.body)
	if err != nil {
		t.Fatal(err)
	}

	graph := body.graph
	point := cfg.Point(1)
	before, present, err := publication.PointInput(context.Background(), point, 0)
	if err != nil || !present {
		t.Fatalf("formal IndexMutation point input = present:%t err:%v", present, err)
	}
	tablePath := pathdom.Path{Symbol: symbol.ID(101)}
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 6101, HasExpr: true}
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 6102, HasExpr: true}
	write := factflow.NewDynamicIndexWrite(factflow.NewDynamicIndexTarget(tablePath, keySource, nil), valueSource,
		dynamicindex.AdmissionAdmitted, factflow.DynamicIndexReadbackKeyAndValue)
	invalidation := factflow.NewPathDescendantInvalidation(tablePath).WithDynamicTarget(write.TargetRef())
	facts := factflow.NewFacts(factflow.FactsInput{
		DynamicIndexWrites:          map[cfg.Point]factflow.DynamicIndexWrite{point: write},
		PathDescendantInvalidations: map[cfg.Point]factflow.PathDescendantInvalidation{point: invalidation},
	})
	address, err := body.pathSemantics.FreezePathAddress(point, tablePath)
	if err != nil {
		t.Fatal(err)
	}
	concreteAfter, err := body.pathSemantics.ApplyBoundaryIndexMutation(
		context.Background(), reg, graph, facts, point, keyValue, storedValue, before, before, address, true,
	)
	if err != nil {
		t.Fatal(err)
	}
	publishedAfter, present, err := publication.PlannedNodeOutput(context.Background(), point, 0)
	if err != nil || !present {
		t.Fatalf("formal IndexMutation point output = present:%t err:%v", present, err)
	}
	for _, lane := range domain.NonValuesLaneInventory() {
		left := mustFormalEffectLaneFactor(t, domain, publishedAfter, lane)
		right := mustFormalEffectLaneFactor(t, domain, concreteAfter, lane)
		equal, equalErr := domain.LaneEqual(left, right)
		if equalErr != nil || !equal {
			t.Errorf("formal IndexMutation lane %s differs from canonical transaction: equal=%t err=%v", lane.ID(), equal, equalErr)
		}
	}
}

func TestFormalEffectIndexMutationCancellationPublishesNothing(t *testing.T) {
	reg := standard.Registry()
	program := formalIndexMutationTestProgram(t, typevalue.LiteralString(reg, "member"), typevalue.LiteralString(reg, "written"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := executeFormalRelation(ctx, program)
	if result != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled formal IndexMutation published %#v, err=%v", result, err)
	}
}

func formalIndexMutationTestProgram(t *testing.T, keyValue, storedValue product.Value) *RelationProgram {
	t.Helper()
	program := formalPathReplacementTestProgram(t, nil, storedValue)
	body := &program.bodies[0]
	code, arena, effects := body.relation.code, body.relation.arena, body.relation.effects
	arena.sealed, effects.sealed, code.sealed = false, false, false
	table := arena.Path(Root{Kind: RootParam})
	effect, err := effects.IndexMutation(IndexMutationConfig{
		Invalidation: InvalidatePathConfig{
			Target: PathEffectTarget(table), Scope: InvalidationScopeDescendants,
			PreserveStructuralWitness: true, PreserveDynamicValueMemberships: true,
		},
		Table: PathEffectTarget(table), Key: arena.Constant(keyValue), Value: arena.Constant(storedValue),
		Admission: dynamicindex.AdmissionAdmitted, Readback: factflow.DynamicIndexReadbackKeyAndValue,
		Site: EffectSite{Owner: 1, Ordinal: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	code.nodes[1].steps = []boundaryStep{{kind: boundaryStepEffect, point: 1, effect: effect}}
	arena.Seal()
	effects.Seal()
	code.sealed = true
	refreezeFormalTestStaticTopology(t, program)
	refreezeFormalEffectTestProgram(t, program)
	return program
}

func mustFormalEffectLaneFactor(t *testing.T, domain state.ProductDomain, value state.State, lane state.ProductLane) state.LaneFactor {
	t.Helper()
	factors, err := domain.DecomposeLanes(value, []state.ProductLane{lane})
	if err != nil || len(factors) != 1 {
		t.Fatalf("decompose lane %s: factors=%d err=%v", lane.ID(), len(factors), err)
	}
	return factors[0]
}

func assertFormalAllocationTemplateDifferential(t *testing.T, program *RelationProgram) {
	t.Helper()
	var cell formalRelationCell
	found := false
	for root := relationRootRef(1); root < relationRootRef(len(program.bodies[0].relation.code.nodes)); root++ {
		for index, step := range program.bodies[0].relation.code.nodes[root].steps {
			if step.kind == boundaryStepEffect && program.bodies[0].relation.effects.Kind(step.effect) == EffectAllocationTemplate {
				cell = formalRelationCell{Variable: 1, Root: root, Step: uint32(index + 1), Kind: formalRelationCellStep}
				found = true
			}
		}
	}
	if !found {
		t.Fatal("formal AllocationTemplate cell absent")
	}
	execution, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	equation, ok := program.formalTemplate.equation(cell)
	if !ok || equation.Operator.allocationTemplate == nil || equation.Operator.stepCapability != formalRelationStepCapabilityAllocationTemplate {
		t.Fatal("formal AllocationTemplate adapter absent")
	}
	plan := equation.Operator.allocationTemplate
	groups := make([]formalFiberGroupDescriptor, 0, len(equation.Operator.effectGroups))
	for _, group := range equation.Operator.effectGroups {
		if group.kind != formalFiberGroupValues {
			groups = append(groups, group)
		}
	}
	if len(groups) != 2 {
		t.Fatalf("formal AllocationTemplate sparse width=%d, want exact 2", len(groups))
	}
	span, _, _, ok := execution.algebra.span(1)
	if !ok {
		t.Fatal("formal AllocationTemplate span")
	}
	wantWrites := 0
	for _, group := range groups {
		wantWrites += len(group.members)
	}
	if len(equation.Operator.effectWriteOrdinals) != wantWrites || len(equation.Operator.effectWriteOrdinals) == 0 || len(equation.Operator.effectWriteOrdinals) >= span.count {
		t.Fatalf("formal AllocationTemplate write footprint=%d, want exact participant width %d below span %d", len(equation.Operator.effectWriteOrdinals), wantWrites, span.count)
	}
	operands, err := partitionFormalRelationStepOperands(equation)
	if err != nil {
		t.Fatal(err)
	}
	inputRegions, err := execution.algebra.tupleLeafRegions(execution.values[operands.Flow.Source.cell])
	if err != nil || len(inputRegions) != 1 {
		t.Fatalf("formal AllocationTemplate input regions=%d err=%v", len(inputRegions), err)
	}
	outputRegions, err := execution.algebra.tupleLeafRegions(execution.values[cell])
	if err != nil || len(outputRegions) != 1 {
		t.Fatalf("formal AllocationTemplate output regions=%d err=%v", len(outputRegions), err)
	}
	reapplied, err := execution.algebra.applyFormalAllocationTemplate(equation.Operator, execution.values[cell])
	if err != nil {
		t.Fatal(err)
	}
	if after := execution.values[cell]; reapplied.variable != after.variable || reapplied.root.owner != after.root.owner || reapplied.root.ref != after.root.ref {
		t.Fatalf("idempotent AllocationTemplate rebuilt its physical root: before=%#v after=%#v", after, reapplied)
	}
	written := make(map[state.ProductLane]bool, len(groups))
	for _, group := range groups {
		written[group.lane] = true
		before, materializeErr := inputRegions[0].evaluator.laneFactor(group)
		if materializeErr != nil {
			t.Fatal(materializeErr)
		}
		want, applyErr := program.bodies[0].productDomain.ApplyObjectGraphMutationFactor(plan.graph, before)
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		got, materializeErr := outputRegions[0].evaluator.laneFactor(group)
		if materializeErr != nil {
			t.Fatal(materializeErr)
		}
		equal, equalErr := program.bodies[0].productDomain.LaneEqual(got, want)
		if equalErr != nil || !equal {
			t.Fatalf("formal AllocationTemplate lane %s differs from object-graph law: equal=%t err=%v", group.lane.ID(), equal, equalErr)
		}
	}
	for _, group := range span.groupDescriptors() {
		if written[group.lane] && group.kind != formalFiberGroupValues {
			continue
		}
		if group.kind == formalFiberGroupValues {
			before, beforeErr := inputRegions[0].evaluator.valuesFactor()
			after, afterErr := outputRegions[0].evaluator.valuesFactor()
			if beforeErr != nil || afterErr != nil || !state.ValueFactorLattice[FormalSlot](program.registry).Equal(before, after) {
				t.Fatalf("formal AllocationTemplate changed Values: before=%v after=%v", beforeErr, afterErr)
			}
			continue
		}
		before, beforeErr := inputRegions[0].evaluator.laneFactor(group)
		after, afterErr := outputRegions[0].evaluator.laneFactor(group)
		equal, equalErr := program.bodies[0].productDomain.LaneEqual(before, after)
		if beforeErr != nil || afterErr != nil || equalErr != nil || !equal {
			t.Fatalf("formal AllocationTemplate changed residual lane %s: before=%v after=%v equal=%v", group.lane.ID(), beforeErr, afterErr, equalErr)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := executeFormalRelation(ctx, program)
	if result != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled formal AllocationTemplate published %#v, err=%v", result, err)
	}
}

func formalPathInvalidationTestProgram(t *testing.T, scope InvalidationScope) *RelationProgram {
	t.Helper()
	reg := standard.Registry()
	program := formalPathReplacementTestProgram(t, nil, typevalue.LiteralString(reg, "discarded"))
	body := &program.bodies[0]
	code, arena, effects := body.relation.code, body.relation.arena, body.relation.effects
	arena.sealed, effects.sealed, code.sealed = false, false, false
	target := arena.Path(Root{Kind: RootParam})
	if scope == InvalidationScopeSubtree {
		target = arena.Path(Root{Kind: RootParam}, segment.Segment{Kind: segment.SegmentField, Name: "member"})
	}
	effect, err := effects.InvalidatePath(InvalidatePathConfig{Target: PathEffectTarget(target), Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	code.nodes[1].steps = []boundaryStep{{kind: boundaryStepEffect, effect: effect}}
	arena.Seal()
	effects.Seal()
	code.sealed = true
	refreezeFormalEffectTestProgram(t, program)
	return program
}

func formalPathReplacementTestProgram(t *testing.T, guarded *bool, values ...product.Value) *RelationProgram {
	t.Helper()
	program := formalRootInputTestProgram(t, standard.Registry())
	body := &program.bodies[0]
	roots, err := sealRelationRootCarrierWithAmbients(body.plan, body.keys, body.relation.shape, []AmbientRoot{{Symbol: symbol.ID(104)}})
	if err != nil {
		t.Fatal(err)
	}
	body.roots = roots
	code, arena, effects := body.relation.code, body.relation.arena, body.relation.effects
	arena.sealed, effects.sealed, code.sealed = false, false, false
	targetTerm := arena.Path(Root{Kind: RootParam}, segment.Segment{Kind: segment.SegmentField, Name: "member"})
	if targetTerm == 0 {
		t.Fatal("Effect target term")
	}
	steps := make([]boundaryStep, len(values))
	for index, value := range values {
		term := arena.Constant(value)
		effect, err := effects.PathStore(PathStoreConfig{
			Assignment: PathStoreWriteConfig{Target: targetTerm, Value: term}, HasAssignment: true,
			Site: EffectSite{Owner: 1, Ordinal: uint32(index + 1)},
		})
		if err != nil {
			t.Fatal(err)
		}
		steps[index] = boundaryStep{kind: boundaryStepEffect, effect: effect}
	}
	if guarded != nil && *guarded && len(steps) != 0 {
		steps[0].guard = arena.Truthy(arena.Root(Root{Kind: RootParam}))
	}
	code.nodes = []relationNode{{}, {kind: relationNodeSequence, steps: steps, next: 2}, {kind: relationNodeOutcome, outcome: 1}}
	arena.Seal()
	effects.Seal()
	code.sealed = true

	concreteTarget := body.keys.FromPath(pathdom.Path{Symbol: symbol.ID(101), Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "member"}}})
	if concreteTarget.Kind == 0 || body.keys.FormatReadOnly(concreteTarget) == "" {
		t.Fatal("concrete Effect target")
	}
	old := typevalue.LiteralString(program.registry, "old")
	initial := state.Reachable(state.State{}).WriteLocalPathKey(program.registry, concreteTarget, old)
	body.initialStatePlan = testInitialStatePlan(t, body.body, body.graph,
		state.NewInitialStateSeed(state.InitialCoordinate(body.graph.Entry()), initial))

	refreezeFormalTestStaticTopology(t, program)
	refreezeFormalEffectTestProgram(t, program)
	return program
}

func refreezeFormalEffectTestProgram(t *testing.T, program *RelationProgram) {
	t.Helper()
	components, err := freezeFormalComponentTerminalSchema(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalComponents = components
	guards, err := freezeFormalGuardVocabulary(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalGuards = guards
	template, err := freezeFormalRelationTemplate(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalTemplate = template
}

func firstCellIndex(t *testing.T, program *RelationProgram, cell formalRelationCell) int {
	t.Helper()
	index, ok := program.formalRegion.plan.CanonicalIndex(cell)
	if !ok {
		t.Fatalf("formal cell %+v", cell)
	}
	return index
}

func formalEffectTestPathValue(t *testing.T, execution *formalRelationExecution, operator formalRelationOperatorRef, tuple formalRelationTuple, want product.Value) {
	t.Helper()
	regions, err := execution.algebra.tupleLeafRegions(tuple)
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 1 {
		t.Fatalf("unguarded Effect regions = %d", len(regions))
	}
	got := formalEffectTestLeafPathValue(t, execution.algebra, operator, regions[0].evaluator)
	if !product.Equal(execution.algebra.program.registry, got, want) {
		t.Fatalf("Effect path value = %#v, want %#v", got, want)
	}
}

func formalEffectTestLeafPathValue(t *testing.T, algebra *formalTupleAlgebra, operator formalRelationOperatorRef, evaluator formalTupleLeafEvaluator) product.Value {
	t.Helper()
	span, _, _, ok := algebra.span(evaluator.variable)
	if !ok || operator.pathReplacement == nil {
		t.Fatal("Effect path replacement adapter")
	}
	for _, group := range span.groupDescriptors() {
		if group.lane.ID() != state.LanePathEvidence {
			continue
		}
		complete, err := evaluator.completeLeaves()
		if err != nil {
			t.Fatal(err)
		}
		factor, err := algebra.materializeFormalEffectLane(evaluator.authority, span, group, complete)
		if err != nil {
			t.Fatal(err)
		}
		residual, err := evaluator.authority.product.ComposeSparse([]state.LaneFactor{factor})
		if err != nil {
			t.Fatal(err)
		}
		return residual.ReadLocalPathKey(algebra.program.registry, operator.pathReplacement.target)
	}
	t.Fatal("Effect path-evidence group")
	return product.Value{}
}
