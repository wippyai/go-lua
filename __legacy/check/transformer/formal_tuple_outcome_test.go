package transformer

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

func TestFormalOutcomeN5PublishesOutputOnceBeforeOccurrenceDeterministically(t *testing.T) {
	reg := standard.Registry()
	base := formalRootInputTestProgram(t, reg)
	body := &base.bodies[0]
	body.relation.shape.Results = 1
	body.relation.code.shape.Results = 1
	source := body.relation.arena.Root(Root{Kind: RootParam, Index: 0})
	body.relation.code.outcomes[1].returnTransaction = testReturnTransactionTerm(t, 41, source)
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{{}, {kind: relationNodeOutcome, outcome: 1}})
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	span, directory, _, ok := algebra.span(1)
	if !ok {
		t.Fatal("formal N5 span")
	}
	values, ok := span.valuesGroup()
	if !ok {
		t.Fatal("formal N5 Values group")
	}
	param, ok := program.formalSlots.Slot(body.body, Root{Kind: RootParam, Index: 0})
	if !ok {
		t.Fatal("formal N5 param slot")
	}
	resultSlot, ok := program.formalSlots.Slot(body.body, Root{Kind: RootResult, Index: 0})
	if !ok {
		t.Fatal("formal N5 Output slot")
	}
	unrelatedSlot, ok := program.formalSlots.Slot(body.body, Root{Kind: RootCapture, Index: 0})
	if !ok {
		t.Fatal("formal N5 unrelated slot")
	}
	want := typevalue.String(reg)
	unrelated := typevalue.LiteralNumber(reg, 9)
	predecessor, err := algebra.writeValuesFactor(formalTupleTestLive(t, algebra, 1), values, state.ValueFactor[FormalSlot]{
		Values: map[FormalSlot]product.Value{param: want, unrelatedSlot: unrelated},
	})
	if err != nil {
		t.Fatal(err)
	}
	equation, ok := program.formalTemplate.equation(program.formalRegion.outcomes[0][0])
	if !ok || equation.Operator.outcomeTransaction == nil {
		t.Fatal("formal N5 equation")
	}
	plan := equation.Operator.outcomeTransaction
	paramMember, _ := values.slot(param)
	resultMember, _ := values.slot(resultSlot)
	containsOrdinal := func(ordinals []formalFiberOrdinal, want formalFiberOrdinal) bool {
		for _, ordinal := range ordinals {
			if ordinal == want {
				return true
			}
		}
		return false
	}
	bindingReads := plan.bindingLift.roles[0].reads
	if !containsOrdinal(bindingReads, plan.bindingValues.top.ordinal) || !containsOrdinal(bindingReads, paramMember.ordinal) ||
		containsOrdinal(bindingReads, resultMember.ordinal) || !containsOrdinal(plan.bindingLift.writes, resultMember.ordinal) {
		t.Fatalf("formal N5 binding Values read/write = %v/%v", bindingReads, plan.bindingLift.writes)
	}
	beforeUnrelated, err := directory.valueAt(predecessor.root, values.descriptor.valueSlots[values.descriptor.valueSlotPosition[unrelatedSlot]].ordinal)
	if err != nil {
		t.Fatal(err)
	}
	first, err := algebra.projectOutcome(equation.Operator, predecessor)
	if err != nil {
		t.Fatal(err)
	}
	second, err := algebra.projectOutcome(equation.Operator, predecessor)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("formal N5 physical result changed across identical execution: %#v != %#v", first, second)
	}
	regions, err := algebra.tupleLeafRegions(first)
	if err != nil || len(regions) != 1 {
		t.Fatalf("formal N5 result regions=%d err=%v", len(regions), err)
	}
	got, err := regions[0].evaluator.valuesFactor()
	if err != nil {
		t.Fatal(err)
	}
	if !product.Equal(reg, got.Values[resultSlot], want) {
		t.Fatalf("formal N5 Output = %v, want source value", got.Values[resultSlot])
	}
	if !product.Equal(reg, got.Values[unrelatedSlot], unrelated) {
		t.Fatalf("formal N5 changed unrelated Values slot: %v", got.Values[unrelatedSlot])
	}
	afterUnrelated, err := directory.valueAt(first.root, values.descriptor.valueSlots[values.descriptor.valueSlotPosition[unrelatedSlot]].ordinal)
	if err != nil || afterUnrelated != beforeUnrelated {
		t.Fatalf("unrelated Values root was not structural carry: %d -> %d, %v", beforeUnrelated, afterUnrelated, err)
	}
	formalOutcomeTestHasOccurrence(t, algebra, first, 1)
}

func TestFormalOutcomeValuesTopSuppressesResultSlotPublication(t *testing.T) {
	reg := standard.Registry()
	base := formalRootInputTestProgram(t, reg)
	body := &base.bodies[0]
	body.relation.shape.Results = 1
	body.relation.code.shape.Results = 1
	source := body.relation.arena.Root(Root{Kind: RootParam, Index: 0})
	body.relation.code.outcomes[1].returnTransaction = testReturnTransactionTerm(t, 41, source)
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{{}, {kind: relationNodeOutcome, outcome: 1}})
	algebra := formalTupleTestAlgebra(t, program)
	span, directory, _, ok := algebra.span(1)
	if !ok {
		t.Fatal("formal N5 Top span")
	}
	values, _ := span.valuesGroup()
	predecessor, err := algebra.writeValuesFactor(formalTupleTestLive(t, algebra, 1), values, state.ValueFactor[FormalSlot]{Top: true})
	if err != nil {
		t.Fatal(err)
	}
	equation, _ := program.formalTemplate.equation(program.formalRegion.outcomes[0][0])
	resultMember, ok := values.slot(mustFormalOutcomeResultSlot(t, program, body))
	if !ok {
		t.Fatal("formal N5 Top result member")
	}
	before, err := directory.valueAt(predecessor.root, resultMember.ordinal)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := algebra.projectOutcome(equation.Operator, predecessor)
	if err != nil {
		t.Fatal(err)
	}
	resultRegions, err := algebra.tupleLeafRegions(projected)
	if err != nil || len(resultRegions) != 1 {
		t.Fatalf("formal N5 Top result regions=%d err=%v", len(resultRegions), err)
	}
	factor, err := resultRegions[0].evaluator.valuesFactor()
	if err != nil || !factor.Top {
		t.Fatalf("formal N5 changed Values Top: %#v, %v", factor, err)
	}
	after, err := directory.valueAt(projected.root, resultMember.ordinal)
	if err != nil || before != after {
		t.Fatalf("Values Top changed dormant result root: %d -> %d, %v", before, after, err)
	}
}

func mustFormalOutcomeResultSlot(t *testing.T, program *RelationProgram, body *relationProgramBody) FormalSlot {
	t.Helper()
	slot, ok := program.formalSlots.Slot(body.body, Root{Kind: RootResult, Index: 0})
	if !ok {
		t.Fatal("formal N5 result slot")
	}
	return slot
}

func TestFormalSparseLeafDirectPositionsDistinguishBottomAbsenceAndShareProjection(t *testing.T) {
	ordinals := []formalFiberOrdinal{1, 4, 9}
	positions, err := sealFormalOrdinalPositions(12, ordinals)
	if err != nil {
		t.Fatal(err)
	}
	first := formalSparseLeafView{
		ordinals: ordinals, positions: positions,
		leaves: []decisionLeaf{7, 0, 11},
	}
	second := formalSparseLeafView{
		ordinals: ordinals, positions: positions,
		leaves: []decisionLeaf{13, 17, 19},
	}
	if len(first.positions.positions) == 0 || &first.positions.positions[0] != &second.positions.positions[0] {
		t.Fatal("sparse regions did not share their projection position directory")
	}
	if leaf, present := first.leaf(4); !present || leaf != 0 {
		t.Fatalf("selected physical Bottom = (%d,%t), want (0,true)", leaf, present)
	}
	if leaf, present := first.leaf(5); present || leaf != 0 {
		t.Fatalf("absent ordinal = (%d,%t), want (0,false)", leaf, present)
	}
	if !first.setLeaf(9, 23) {
		t.Fatal("declared direct position rejected")
	}
	if leaf, present := first.leaf(9); !present || leaf != 23 {
		t.Fatalf("direct set/leaf = (%d,%t), want (23,true)", leaf, present)
	}
	if first.setLeaf(8, 29) {
		t.Fatal("undeclared direct position accepted")
	}
	if _, err := sealFormalOrdinalPositions(12, []formalFiberOrdinal{1, 4, 4}); err == nil {
		t.Fatal("duplicate projection ordinal accepted")
	}
}

func TestFormalOutcomeJoinsTwoReturnPathsWithCorrelatedGuards(t *testing.T) {
	base := formalRootInputTestProgram(t, standard.Registry())
	arena := base.bodies[0].relation.code.terms
	guard := arena.Truthy(arena.Root(Root{Kind: RootParam, Index: 0}))
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{},
		{kind: relationNodeChoice, guard: guard, whenTrue: 2, whenFalse: 3},
		{kind: relationNodeOutcome, outcome: 1},
		{kind: relationNodeOutcome, outcome: 2},
	})
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	span, directory, authority, ok := algebra.span(1)
	if !ok {
		t.Fatal("formal Outcome span")
	}
	var payload formalFiberDescriptor
	for _, descriptor := range span.descriptors() {
		if descriptor.role == formalFiberMiddleValue {
			payload = descriptor
			break
		}
	}
	if payload.role == formalFiberInvalid {
		t.Fatal("formal Outcome fixture has no symbolic payload fiber")
	}
	leftLeaf, err := authority.internBinding(formalQualifiedBinding{value: relationArenaValueRef{
		owner: 1, arena: authority.terms, term: authority.terms.Root(Root{Kind: RootParam, Index: 0}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	rightLeaf, err := authority.internBinding(formalQualifiedBinding{value: relationArenaValueRef{
		owner: 1, arena: authority.terms, term: authority.terms.Root(Root{Kind: RootCapture, Index: 0}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	leftCare, err := algebra.decisionForGuard(1, 0, arena, guard)
	if err != nil {
		t.Fatal(err)
	}
	rightCare, err := formalDecisionBooleanNot(algebra, leftCare)
	if err != nil {
		t.Fatal(err)
	}
	left := formalTupleTestLive(t, algebra, 1)
	left, err = algebra.writeCare(left, leftCare)
	if err == nil {
		left, err = algebra.writeScalar(left, payload, algebra.decisions.terminal(leftLeaf))
	}
	if err != nil {
		t.Fatal(err)
	}
	right := formalTupleTestLive(t, algebra, 1)
	right, err = algebra.writeCare(right, rightCare)
	if err == nil {
		right, err = algebra.writeScalar(right, payload, algebra.decisions.terminal(rightLeaf))
	}
	if err != nil {
		t.Fatal(err)
	}

	leftEquation, leftOK := program.formalTemplate.equation(program.formalRegion.outcomes[0][0])
	rightEquation, rightOK := program.formalTemplate.equation(program.formalRegion.outcomes[0][1])
	if !leftOK || !rightOK || len(leftEquation.Inputs) != 1 || len(rightEquation.Inputs) != 1 {
		t.Fatalf("unique Outcome equation inputs = %d/%d", len(leftEquation.Inputs), len(rightEquation.Inputs))
	}
	reads := make(map[formalRelationCell]int)
	read := func(cell formalRelationCell) formalRelationTuple {
		reads[cell]++
		switch cell.Root {
		case 2:
			return left
		case 3:
			return right
		default:
			t.Fatalf("Outcome read undeclared source %+v", cell)
			return formalRelationTuple{}
		}
	}
	leftOutcome := evaluateFormalRelationEquation(algebra, leftEquation, read)
	rightOutcome := evaluateFormalRelationEquation(algebra, rightEquation, read)
	joined := algebra.combine(formalComponentJoin, leftOutcome, rightOutcome)
	if algebra.err() != nil || joined.bottom() || len(reads) != 2 {
		t.Fatalf("two-path Outcome = %#v, reads=%#v err=%v", joined, reads, algebra.err())
	}
	care, err := algebra.care(joined)
	if err != nil || care != decisionTrue {
		t.Fatalf("complementary return Care = %d, %v; want true", care, err)
	}
	formalOutcomeTestHasOccurrence(t, algebra, joined, 1)
	formalOutcomeTestHasOccurrence(t, algebra, joined, 2)

	ordinal, ok := span.ordinal(payload)
	if !ok {
		t.Fatal("payload ordinal")
	}
	value, err := directory.valueAt(joined.root, ordinal)
	if err != nil {
		t.Fatal(err)
	}
	payloadRoot := decisionRef(value)
	wantLeft, err := algebra.decisions.restrict(context.Background(), leftCare, algebra.decisions.terminal(leftLeaf))
	if err != nil {
		t.Fatal(err)
	}
	wantRight, err := algebra.decisions.restrict(context.Background(), rightCare, algebra.decisions.terminal(rightLeaf))
	if err != nil {
		t.Fatal(err)
	}
	gotLeft, err := algebra.decisions.restrict(context.Background(), leftCare, payloadRoot)
	if err != nil {
		t.Fatal(err)
	}
	gotRight, err := algebra.decisions.restrict(context.Background(), rightCare, payloadRoot)
	if err != nil {
		t.Fatal(err)
	}
	if gotLeft != wantLeft || gotRight != wantRight {
		t.Fatalf("guarded return correlation lost: left=%d/%d right=%d/%d", gotLeft, wantLeft, gotRight, wantRight)
	}
}

func TestFormalOutcomeUnreachableReturnStaysBottom(t *testing.T) {
	program := formalRelationExecutorTestProgram(t, []relationNode{
		{},
		{kind: relationNodeBottom},
		{kind: relationNodeOutcome, outcome: 1},
	})
	result, err := executeFormalRelation(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	outcome := result.values[program.formalRegion.outcomes[0][0]]
	if !outcome.bottom() {
		t.Fatalf("unreachable Outcome = %#v, want Bottom", outcome)
	}
}

func TestFormalOutcomePreservesEveryRegisteredAxisByStructuralSharing(t *testing.T) {
	program := formalRelationExecutorTestProgram(t, []relationNode{
		{},
		{kind: relationNodeSequence, next: 2},
		{kind: relationNodeOutcome, outcome: 1},
	})
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	predecessor := formalTupleTestLive(t, algebra, 1)
	span, _, authority, ok := algebra.span(1)
	if !ok {
		t.Fatal("formal Outcome span")
	}
	groups := span.groupDescriptors()
	for _, group := range groups {
		switch group.kind {
		case formalFiberGroupValues:
			predecessor, err = algebra.writeValuesFactor(predecessor, formalValuesFiberGroup{descriptor: group}, state.ValueFactor[FormalSlot]{Top: true})
		case formalFiberGroupOrdinaryLane:
			var top state.LaneFactor
			top, err = authority.product.LaneTop(group.lane)
			if err == nil {
				predecessor, err = algebra.writeOrdinaryFactor(predecessor, formalOrdinaryLaneFiberGroup{descriptor: group}, top)
			}
		case formalFiberGroupCoordinateLane:
			var top state.LaneFactor
			top, err = authority.product.LaneTop(group.lane)
			if err == nil {
				predecessor, err = algebra.writeCoordinateFactor(predecessor, formalCoordinateLaneFiberGroup{descriptor: group}, top)
			}
		default:
			t.Fatalf("unknown registered formal group kind %d", group.kind)
		}
		if err != nil {
			t.Fatalf("populate registered lane %q: %v", group.lane.ID(), err)
		}
	}
	equation, ok := program.formalTemplate.equation(program.formalRegion.outcomes[0][0])
	if !ok || equation.Operator.outcomeTransaction == nil {
		t.Fatal("formal Outcome fixture has no frozen N5 operator")
	}
	projected, err := algebra.projectOutcome(equation.Operator, predecessor)
	if err != nil {
		t.Fatal(err)
	}
	formalOutcomeTestExactProjection(t, algebra, predecessor, projected, 1)
	wantLanes := program.bodies[0].productDomain.LaneInventory()
	if len(groups) != len(wantLanes) {
		t.Fatalf("registered group count = %d, want one per %d lanes", len(groups), len(wantLanes))
	}
	for index, group := range groups {
		if group.lane != wantLanes[index] {
			t.Fatalf("registered group %d lane = %q, want %q", index, group.lane.ID(), wantLanes[index].ID())
		}
	}
}

func TestFormalOutcomeEmptyN6ContributesNoCovariantCarrier(t *testing.T) {
	program := formalRelationExecutorTestProgram(t, []relationNode{{}, {kind: relationNodeOutcome, outcome: 1}})
	equation, ok := program.formalTemplate.equation(program.formalRegion.outcomes[0][0])
	if !ok || equation.Operator.outcomeTransaction == nil {
		t.Fatal("formal Outcome equation")
	}
	plan := equation.Operator.outcomeTransaction
	if plan.covariant.HasStateSteps() || len(plan.covariantBindings) != 0 || len(plan.covariantGroups) != 0 || plan.covariantTopology.Len() != 0 {
		t.Fatalf("empty N6 retained carrier: steps=%t bindings=%d lanes=%d topology=%d",
			plan.covariant.HasStateSteps(), len(plan.covariantBindings), len(plan.covariantGroups), plan.covariantTopology.Len())
	}
}

func TestFormalOutcomeIdentityPlanExcludesUnregisteredSkeletonOnlyFamily(t *testing.T) {
	program := formalRelationExecutorTestProgram(t, []relationNode{{}, {kind: relationNodeOutcome, outcome: 1}})
	equation, ok := program.formalTemplate.equation(program.formalRegion.outcomes[0][0])
	if !ok || equation.Operator.outcomeTransaction == nil {
		t.Fatal("formal Outcome equation")
	}
	plan := equation.Operator.outcomeTransaction
	found := false
	span, _ := program.formalFibers.span(1)
	for _, group := range span.groupDescriptors() {
		for _, family := range group.coordinateFamilies {
			if len(family.scalars) != 0 {
				continue
			}
			found = true
			for _, observer := range plan.identity.observers {
				if observer.ordinal == family.skeleton && !coordinateFamilySame(observer.family, family.family) {
					t.Fatalf("identity observer aliased skeleton-only family %q", family.family.ID())
				}
			}
		}
	}
	if !found {
		t.Fatal("formal Outcome fixture has no skeleton-only N5 family")
	}
}

func TestFormalOutcomeSparseProjectionIgnoresAndCarriesUnrelatedBinaryFiber(t *testing.T) {
	base := formalRootInputTestProgram(t, standard.Registry())
	arena := base.bodies[0].relation.code.terms
	guard := arena.Truthy(arena.Root(Root{Kind: RootParam, Index: 0}))
	program := formalRelationExecutorTestProgramFromBase(t, base, []relationNode{
		{}, {kind: relationNodeChoice, guard: guard, whenTrue: 2, whenFalse: 3},
		{kind: relationNodeOutcome, outcome: 1}, {kind: relationNodeBottom},
	})
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	span, directory, authority, ok := algebra.span(1)
	if !ok {
		t.Fatal("formal Outcome span")
	}
	var unrelated formalFiberDescriptor
	for _, descriptor := range span.descriptors() {
		if descriptor.role == formalFiberMiddleValue {
			unrelated = descriptor
			break
		}
	}
	if unrelated.role == formalFiberInvalid {
		t.Fatal("fixture has no unrelated symbolic fiber")
	}
	unrelatedOrdinal, ok := span.ordinal(unrelated)
	if !ok {
		t.Fatal("unrelated fiber ordinal")
	}
	equation, ok := program.formalTemplate.equation(program.formalRegion.outcomes[0][0])
	if !ok || equation.Operator.outcomeTransaction == nil {
		t.Fatal("formal Outcome fixture has no frozen N5 operator")
	}
	plan := equation.Operator.outcomeTransaction
	for _, lift := range []formalClosedFactorLift{plan.bindingLift, plan.presenceLift, plan.covariantLift} {
		if !lift.sealed {
			continue
		}
		for _, role := range lift.roles {
			for _, ordinal := range role.reads {
				if ordinal == unrelatedOrdinal {
					t.Fatalf("unrelated fiber %d entered an active N5 read projection", ordinal)
				}
			}
		}
		for _, ordinal := range lift.writes {
			if ordinal == unrelatedOrdinal {
				t.Fatalf("unrelated fiber %d entered an active N5 publication", ordinal)
			}
		}
	}

	condition, err := algebra.decisionForGuard(1, 0, arena, guard)
	if err != nil {
		t.Fatal(err)
	}
	left, err := authority.internBinding(formalQualifiedBinding{value: relationArenaValueRef{
		owner: 1, arena: arena, term: arena.Root(Root{Kind: RootCapture, Index: 0}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	right, err := authority.internBinding(formalQualifiedBinding{value: relationArenaValueRef{
		owner: 1, arena: arena, term: arena.Root(Root{Kind: RootGlobal, Index: 0}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	binary, err := algebra.decisions.condition(context.Background(), condition, algebra.decisions.terminal(left), algebra.decisions.terminal(right))
	if err != nil {
		t.Fatal(err)
	}
	predecessor, err := algebra.writeScalar(formalTupleTestLive(t, algebra, 1), unrelated, binary)
	if err != nil {
		t.Fatal(err)
	}
	before, err := directory.valueAt(predecessor.root, unrelatedOrdinal)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := algebra.projectOutcome(equation.Operator, predecessor)
	if err != nil {
		t.Fatal(err)
	}
	after, err := directory.valueAt(projected.root, unrelatedOrdinal)
	if err != nil {
		t.Fatal(err)
	}
	if before != formalFiberValue(binary) || after != before {
		t.Fatalf("unrelated binary fiber was not structural carry: before=%d after=%d want=%d", before, after, binary)
	}
}

func formalOutcomeTestExactProjection(t *testing.T, algebra *formalTupleAlgebra, predecessor, projected formalRelationTuple, outcome boundaryOutcomeRef) {
	t.Helper()
	if predecessor.bottom() || projected.bottom() || predecessor.variable != projected.variable {
		t.Fatalf("formal Outcome ownership = predecessor %#v projected %#v", predecessor, projected)
	}
	span, directory, _, ok := algebra.span(predecessor.variable)
	if !ok || predecessor.root.owner != directory || projected.root.owner != directory {
		t.Fatal("formal Outcome did not retain lexical directory ownership")
	}
	differences := 0
	err := directory.visitDifferences(predecessor.root, projected.root, func(ordinal formalFiberOrdinal, before, after formalFiberValue) error {
		descriptors := span.descriptors()
		if int(ordinal) < 0 || int(ordinal) >= len(descriptors) {
			t.Fatalf("formal Outcome changed out-of-range fiber %d", ordinal)
		}
		descriptor := descriptors[ordinal]
		if descriptor.role != formalFiberOutcome || descriptor.outcome != outcome || before != 0 || after == 0 {
			t.Fatalf("formal Outcome changed nonterminal fiber %d: descriptor=%#v before=%d after=%d", ordinal, descriptor, before, after)
		}
		differences++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if differences != 1 {
		t.Fatalf("formal Outcome changed %d fibers, want exactly its occurrence fiber", differences)
	}
	formalOutcomeTestHasOccurrence(t, algebra, projected, outcome)
}

func formalOutcomeTestHasOccurrence(t *testing.T, algebra *formalTupleAlgebra, tuple formalRelationTuple, outcome boundaryOutcomeRef) {
	t.Helper()
	span, directory, authority, ok := algebra.span(tuple.variable)
	if !ok {
		t.Fatal("formal Outcome span")
	}
	for _, descriptor := range span.descriptors() {
		if descriptor.role != formalFiberOutcome || descriptor.outcome != outcome {
			continue
		}
		ordinal, _ := span.ordinal(descriptor)
		value, err := directory.valueAt(tuple.root, ordinal)
		if err != nil {
			t.Fatal(err)
		}
		root := decisionRef(value)
		care, err := algebra.care(tuple)
		if err != nil {
			t.Fatal(err)
		}
		root, err = algebra.decisions.restrict(context.Background(), care, root)
		if err != nil {
			t.Fatal(err)
		}
		seen := false
		_, err = algebra.decisions.mapLeavesTransient(context.Background(), root, func(leaf decisionLeaf) (decisionLeaf, error) {
			if leaf == 0 {
				return 0, nil
			}
			terminal, terminalErr := authority.terminal(leaf)
			if terminalErr != nil || terminal.kind != formalComponentOutcomeOccurrence || terminal.outcome.code != authority.code || terminal.outcome.ref != outcome {
				t.Fatalf("formal Outcome terminal = %#v, %v", terminal, terminalErr)
			}
			seen = true
			return leaf, nil
		})
		if err != nil || !seen {
			t.Fatalf("formal Outcome occurrence missing: seen=%t err=%v", seen, err)
		}
		return
	}
	t.Fatalf("formal Outcome descriptor %d missing", outcome)
}
