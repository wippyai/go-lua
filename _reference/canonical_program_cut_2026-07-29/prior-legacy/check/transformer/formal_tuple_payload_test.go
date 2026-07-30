package transformer

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestFormalTupleDescriptorDefaultIsSemanticNotPhysicalIdentity(t *testing.T) {
	program := formalTupleTestProgram(t, "descriptor-default")
	algebra := formalTupleTestAlgebra(t, program)
	span, directory, authority, ok := algebra.span(1)
	if !ok {
		t.Fatal("formal tuple span is missing")
	}

	implicit := formalTupleTestLive(t, algebra, 1)
	descriptors := span.forest.descriptors[span.first : span.first+span.count]
	var descriptor formalFiberDescriptor
	found := false
	for _, candidate := range descriptors {
		if candidate.role == formalFiberOrdinaryLane {
			descriptor, found = candidate, true
			break
		}
	}
	if !found {
		t.Fatal("registered product has no descriptor with a typed Bottom")
	}
	defaultValue, err := authority.defaultFor(context.Background(), descriptor)
	if err != nil || defaultValue.kind != formalComponentDefaultTerminal {
		t.Fatalf("descriptor default = %#v, %v", defaultValue, err)
	}
	explicitRoot := algebra.decisions.terminal(defaultValue.leaf)
	explicit, err := algebra.writeScalar(implicit, descriptor, explicitRoot)
	if err != nil {
		t.Fatal(err)
	}
	if implicit.root.owner != directory || explicit.root.owner != directory || implicit.root.ref == explicit.root.ref {
		t.Fatal("fixture did not produce distinct physical directory roots")
	}
	if algebra.same(implicit, explicit) {
		t.Fatal("semantic default and explicit Bottom are physically Same")
	}
	if !algebra.equal(implicit, explicit) || !algebra.lessOrEq(implicit, explicit) || !algebra.lessOrEq(explicit, implicit) {
		t.Fatalf("semantic default relation = equal:%t implicit<=explicit:%t explicit<=implicit:%t err:%v",
			algebra.equal(implicit, explicit), algebra.lessOrEq(implicit, explicit), algebra.lessOrEq(explicit, implicit), algebra.err())
	}
	joined := algebra.combine(formalComponentJoin, implicit, explicit)
	if algebra.err() != nil || !algebra.equal(joined, implicit) {
		t.Fatalf("default join differs semantically: err=%v", algebra.err())
	}
}

func TestFormalTupleConditionalZeroUsesDescriptorDefault(t *testing.T) {
	program := formalTupleTestProgram(t, "conditional-default")
	algebra := formalTupleTestAlgebra(t, program)
	span, _, authority, _ := algebra.span(1)
	implicit := formalTupleTestLive(t, algebra, 1)
	descriptors := span.forest.descriptors[span.first : span.first+span.count]
	for index, descriptor := range descriptors {
		if descriptor.role != formalFiberOrdinaryLane {
			continue
		}
		defaultValue, err := authority.defaultFor(context.Background(), descriptor)
		if err != nil || defaultValue.kind != formalComponentDefaultTerminal {
			t.Fatalf("descriptor default = %#v, %v", defaultValue, err)
		}
		typed := algebra.decisions.terminal(defaultValue.leaf)
		conditional := algebra.decisions.branch(17, decisionFalse, typed)
		explicit, err := algebra.writeScalar(implicit, descriptor, conditional)
		if err != nil {
			t.Fatal(err)
		}
		if !algebra.equal(implicit, explicit) || algebra.err() != nil {
			t.Fatalf("conditional explicit Bottom differs from implicit default: equal=%t err=%v", algebra.equal(implicit, explicit), algebra.err())
		}
		_ = index
		return
	}
	t.Fatal("registered product has no descriptor with a typed Bottom")
}

func TestFormalTupleRejectsForeignAndNonCanonicalRoots(t *testing.T) {
	program := formalTupleTestProgram(t, "ownership")
	left := formalTupleTestAlgebra(t, program)
	right := formalTupleTestAlgebra(t, program)
	foreign := formalTupleTestLive(t, left, 1)

	if right.same(foreign, foreign) || right.err() == nil {
		t.Fatalf("foreign Same = true or no error: %v", right.err())
	}
	right = formalTupleTestAlgebra(t, program)
	if right.equal(foreign, foreign) || right.err() == nil {
		t.Fatalf("foreign Equal = true or no error: %v", right.err())
	}
	right = formalTupleTestAlgebra(t, program)
	if got := right.combine(formalComponentJoin, formalRelationTuple{}, foreign); !got.bottom() || right.err() == nil {
		t.Fatalf("Bottom join foreign = %#v, err=%v", got, right.err())
	}
	right = formalTupleTestAlgebra(t, program)
	malformed := formalRelationTuple{root: foreign.root}
	if right.same(malformed, malformed) || right.err() == nil {
		t.Fatalf("non-canonical Bottom was accepted: %v", right.err())
	}
}

func TestFormalTupleWritesAreTypedAndDependentGroupsAreAtomic(t *testing.T) {
	program := formalTupleTestProgram(t, "typed-writes")
	algebra := formalTupleTestAlgebra(t, program)
	tuple := formalTupleTestLive(t, algebra, 1)
	span, _, authority, _ := algebra.span(1)
	descriptors := span.forest.descriptors[span.first : span.first+span.count]
	var ordinary, coordinate, ground formalFiberDescriptor
	for _, descriptor := range descriptors {
		switch descriptor.role {
		case formalFiberOrdinaryLane:
			ordinary = descriptor
		case formalFiberCoordinate:
			coordinate = descriptor
		case formalFiberGroundValue:
			ground = descriptor
		}
	}
	if ordinary.role == formalFiberInvalid || coordinate.role == formalFiberInvalid || ground.role == formalFiberInvalid {
		t.Fatal("registered product fixture lacks ordinary, coordinate, or Values descriptors")
	}
	termLeaf, err := authority.internBinding(formalQualifiedBinding{value: relationArenaValueRef{
		owner: authority.variable,
		arena: authority.terms,
		term:  authority.terms.Root(Root{Kind: RootParam, Index: 0}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := algebra.writeScalar(tuple, ordinary, algebra.decisions.terminal(termLeaf)); err == nil {
		t.Fatal("ordinary lane accepted a symbolic ValueTerm terminal")
	}
	if _, err := algebra.writeScalar(tuple, coordinate, decisionFalse); err == nil {
		t.Fatal("coordinate fiber accepted a partial scalar write")
	}
	if _, err := algebra.writeScalar(tuple, ground, decisionFalse); err == nil {
		t.Fatal("Values fiber accepted a partial scalar write")
	}
	if _, err := algebra.writeCare(tuple, algebra.decisions.terminal(termLeaf)); err == nil {
		t.Fatal("care accepted a non-Boolean terminal")
	}
}

func TestFormalTuplePathBearingSymbolRemainsFailClosedUntilAtomicBindingSet(t *testing.T) {
	program := formalTupleTestProgram(t, "path-correlation-fence")
	algebra := formalTupleTestAlgebra(t, program)
	tuple := formalTupleTestLive(t, algebra, 1)
	span, _, authority, _ := algebra.span(1)
	var pathDescriptor formalFiberDescriptor
	for _, descriptor := range span.forest.descriptors[span.first : span.first+span.count] {
		if descriptor.role == formalFiberMiddlePath {
			pathDescriptor = descriptor
			break
		}
	}
	if pathDescriptor.role == formalFiberInvalid {
		t.Fatal("symbolic fixture has no Middle path descriptor")
	}
	path := authority.terms.middleSymbolPath(1)
	leaf, err := authority.internPathTerm(formalQualifiedPathTerm{arena: authority.terms, term: path})
	if err != nil {
		t.Fatal(err)
	}
	_, err = algebra.writeScalar(tuple, pathDescriptor, algebra.decisions.terminal(leaf))
	if !errors.Is(err, errFormalSymbolicPathCorrelation) {
		t.Fatalf("independent symbolic path publication error = %v", err)
	}
}

func TestFormalTupleSymbolicValueSetsCloseUnderJoinAndRejectUnprovenNarrow(t *testing.T) {
	program := formalTupleTestProgram(t, "symbolic-value-set")
	algebra := formalTupleTestAlgebra(t, program)
	empty := formalTupleTestLive(t, algebra, 1)
	span, _, authority, _ := algebra.span(1)
	var valueDescriptor formalFiberDescriptor
	for _, descriptor := range span.forest.descriptors[span.first : span.first+span.count] {
		if descriptor.role == formalFiberMiddleValue {
			valueDescriptor = descriptor
			break
		}
	}
	if valueDescriptor.role == formalFiberInvalid {
		t.Fatal("symbolic fixture has no Middle value descriptor")
	}
	write := func(term ValueTerm) formalRelationTuple {
		t.Helper()
		leaf, err := authority.internBinding(formalQualifiedBinding{value: relationArenaValueRef{owner: authority.variable, arena: authority.terms, term: term}})
		if err != nil {
			t.Fatal(err)
		}
		tuple, err := algebra.writeScalar(empty, valueDescriptor, algebra.decisions.terminal(leaf))
		if err != nil {
			t.Fatal(err)
		}
		return tuple
	}
	left := write(authority.terms.Root(Root{Kind: RootParam, Index: 0}))
	right := write(authority.terms.Root(Root{Kind: RootParam, Index: 1}))
	joined := algebra.combine(formalComponentJoin, left, right)
	if algebra.err() != nil || !algebra.lessOrEq(left, joined) || !algebra.lessOrEq(right, joined) {
		t.Fatalf("formal symbolic tuple join failed: %v", algebra.err())
	}
	if narrowed := algebra.combine(formalComponentNarrow, joined, left); !narrowed.bottom() || !errors.Is(algebra.err(), errFormalSymbolicMeetUnproven) {
		t.Fatalf("unproven formal symbolic narrow = %#v, error %v", narrowed, algebra.err())
	}
}

func TestFormalTupleValuesGroupUsesCanonicalDependentLattice(t *testing.T) {
	program := formalTupleTestProgram(t, "values-group")
	algebra := formalTupleTestAlgebra(t, program)
	span, _, _, _ := algebra.span(1)
	group, ok := span.valuesGroup()
	if !ok || len(group.descriptor.valueSlots) == 0 {
		t.Fatal("formal Values group is missing")
	}
	slot := group.descriptor.valueSlots[0].slot
	top, err := algebra.writeValuesFactor(formalTupleTestLive(t, algebra, 1), group, state.ValueFactor[FormalSlot]{Top: true})
	if err != nil {
		t.Fatal(err)
	}
	finiteFactor := state.ValueFactor[FormalSlot]{Values: map[FormalSlot]product.Value{slot: product.Top()}}
	finite, err := algebra.writeValuesFactor(formalTupleTestLive(t, algebra, 1), group, finiteFactor)
	if err != nil {
		t.Fatal(err)
	}
	if !algebra.lessOrEq(finite, top) || algebra.lessOrEq(top, finite) || algebra.err() != nil {
		t.Fatalf("Values order finite<=Top=%t Top<=finite=%t err=%v", algebra.lessOrEq(finite, top), algebra.lessOrEq(top, finite), algebra.err())
	}
	meet := algebra.combine(formalComponentMeet, top, finite)
	if algebra.err() != nil || !algebra.equal(meet, finite) {
		t.Fatalf("Values Meet(Top, finite) differs: err=%v", algebra.err())
	}
	join := algebra.combine(formalComponentJoin, top, finite)
	if algebra.err() != nil || !algebra.equal(join, top) {
		t.Fatalf("Values Join(Top, finite) differs: err=%v", algebra.err())
	}
	for _, valueSlot := range group.descriptor.valueSlots {
		root, err := algebra.directories[0].valueAt(join.root, valueSlot.ordinal)
		if err != nil || decisionRef(root) != decisionFalse {
			t.Fatalf("Values Top retained observable slot root %d: %d/%v", valueSlot.ordinal, root, err)
		}
	}
	widen := algebra.combine(formalComponentWiden, finite, top)
	if algebra.err() != nil || !algebra.equal(widen, top) {
		t.Fatalf("Values Widen(finite, Top) differs: err=%v", algebra.err())
	}
	narrow := algebra.combine(formalComponentNarrow, top, finite)
	if algebra.err() != nil || !algebra.equal(narrow, top) {
		t.Fatalf("Values Narrow(Top, finite) differs from registered previous-retaining law: err=%v", algebra.err())
	}
}

func TestFormalTupleValuesTopQuotientsUnobservableSlotSpelling(t *testing.T) {
	program := formalTupleTestProgram(t, "values-top-quotient")
	algebra := formalTupleTestAlgebra(t, program)
	span, directory, authority, _ := algebra.span(1)
	group, ok := span.valuesGroup()
	if !ok || len(group.descriptor.valueSlots) == 0 {
		t.Fatal("formal Values group is missing")
	}
	canonical, err := algebra.writeValuesFactor(formalTupleTestLive(t, algebra, 1), group, state.ValueFactor[FormalSlot]{Top: true})
	if err != nil {
		t.Fatal(err)
	}
	roots, err := algebra.groupRoots(canonical, group.descriptor)
	if err != nil {
		t.Fatal(err)
	}
	valueLeaf, err := authority.internGroundValue(product.Top())
	if err != nil {
		t.Fatal(err)
	}
	slot := group.descriptor.valueSlots[0]
	roots[slot.position] = algebra.decisions.terminal(valueLeaf)
	redundantRoot, err := algebra.applyGroupRoots(span, directory, canonical.root, authority, group.descriptor, roots)
	if err != nil {
		t.Fatal(err)
	}
	redundant := formalRelationTuple{variable: 1, root: redundantRoot}
	if algebra.same(canonical, redundant) || !algebra.equal(canonical, redundant) ||
		!algebra.lessOrEq(canonical, redundant) || !algebra.lessOrEq(redundant, canonical) || algebra.err() != nil {
		t.Fatalf("Values Top quotient failed: same=%t equal=%t order=%t/%t err=%v",
			algebra.same(canonical, redundant), algebra.equal(canonical, redundant),
			algebra.lessOrEq(canonical, redundant), algebra.lessOrEq(redundant, canonical), algebra.err())
	}
	joined := algebra.combine(formalComponentJoin, canonical, redundant)
	if algebra.err() != nil || !algebra.same(joined, canonical) {
		t.Fatalf("Values Top join did not erase redundant slot spelling: same=%t err=%v", algebra.same(joined, canonical), algebra.err())
	}
}

func TestFormalTupleValuesJoinDoesNotCorrelateUnrelatedSlots(t *testing.T) {
	program := formalTupleTestProgram(t, "values-pointwise-join")
	algebra := formalTupleTestAlgebra(t, program)
	span, directory, authority, _ := algebra.span(1)
	group, ok := span.valuesGroup()
	if !ok || len(group.descriptor.valueSlots) < 2 {
		t.Fatal("formal Values group needs two slots")
	}
	left := formalTupleTestLive(t, algebra, 1)
	right := formalTupleTestLive(t, algebra, 1)
	leftRoots, err := algebra.groupRoots(left, group.descriptor)
	if err != nil {
		t.Fatal(err)
	}
	rightRoots := append([]decisionRef(nil), leftRoots...)
	first, second := group.descriptor.valueSlots[0], group.descriptor.valueSlots[1]
	firstLeaf, err := authority.internGroundValue(product.Top())
	if err != nil {
		t.Fatal(err)
	}
	secondLeaf := firstLeaf
	leftRoots[first.position] = algebra.decisions.branch(17, decisionFalse, algebra.decisions.terminal(firstLeaf))
	rightRoots[first.position] = algebra.decisions.branch(18, decisionFalse, algebra.decisions.terminal(firstLeaf))
	leftRoots[second.position] = algebra.decisions.terminal(secondLeaf)
	rightRoots[second.position] = algebra.decisions.terminal(secondLeaf)
	leftRoot, err := algebra.applyGroupRoots(span, directory, left.root, authority, group.descriptor, leftRoots)
	if err != nil {
		t.Fatal(err)
	}
	rightRoot, err := algebra.applyGroupRoots(span, directory, right.root, authority, group.descriptor, rightRoots)
	if err != nil {
		t.Fatal(err)
	}
	joined := algebra.combine(formalComponentJoin,
		formalRelationTuple{variable: 1, root: leftRoot},
		formalRelationTuple{variable: 1, root: rightRoot},
	)
	joinedRoots, err := algebra.groupRoots(joined, group.descriptor)
	if err != nil || algebra.err() != nil {
		t.Fatalf("Values pointwise join failed: %v/%v", err, algebra.err())
	}
	wantFirst, err := algebra.decisions.apply(algebra.ctx, uint8(formalComponentJoin), true,
		leftRoots[first.position], rightRoots[first.position], func(left, right decisionLeaf) (decisionLeaf, error) {
			if left == 0 {
				return right, nil
			}
			if right == 0 {
				return left, nil
			}
			return authority.combine(algebra.ctx, formalComponentJoin, left, right)
		})
	if err != nil {
		t.Fatal(err)
	}
	if joinedRoots[first.position] != wantFirst {
		t.Fatalf("first Values slot root = %d, want independent join %d", joinedRoots[first.position], wantFirst)
	}
	if joinedRoots[second.position] != leftRoots[second.position] {
		t.Fatalf("unrelated Values slot root changed: got %d want identity %d", joinedRoots[second.position], leftRoots[second.position])
	}
}

func TestFormalTupleFailureIsStationaryUntilTransactionRejects(t *testing.T) {
	program := formalTupleTestProgram(t, "failed-transaction-stationary")
	algebra := formalTupleTestAlgebra(t, program)
	live := formalTupleTestLive(t, algebra, 1)
	want := errors.New("formal transaction failed")
	algebra.fail(want)
	if !algebra.equal(live, live) || !algebra.equal(formalRelationTuple{}, formalRelationTuple{}) {
		t.Fatal("failed formal lattice lost reflexive equality")
	}
	if got := algebra.combine(formalComponentJoin, live, formalRelationTuple{}); !algebra.same(got, live) {
		t.Fatal("failed formal lattice replaced its current iterate")
	}
	if !errors.Is(algebra.err(), want) {
		t.Fatalf("failed formal lattice hid its transaction error: %v", algebra.err())
	}
}

func TestFormalTupleOrdinaryGroupsUseRegisteredLaneAlgebra(t *testing.T) {
	program := formalTupleTestProgram(t, "ordinary-group")
	algebra := formalTupleTestAlgebra(t, program)
	span, _, authority, _ := algebra.span(1)
	var group formalOrdinaryLaneFiberGroup
	var bottomFactor, topFactor state.LaneFactor
	for _, descriptor := range span.groupDescriptors() {
		if descriptor.kind != formalFiberGroupOrdinaryLane {
			continue
		}
		bottom, err := authority.product.LaneBottom(descriptor.lane)
		if err != nil {
			t.Fatal(err)
		}
		top, err := authority.product.LaneTop(descriptor.lane)
		if err != nil {
			t.Fatal(err)
		}
		equal, err := authority.product.LaneEqual(bottom, top)
		if err != nil {
			t.Fatal(err)
		}
		if !equal {
			group, bottomFactor, topFactor = formalOrdinaryLaneFiberGroup{descriptor: descriptor}, bottom, top
			break
		}
	}
	if !group.valid() {
		t.Fatal("formal product has no nontrivial ordinary lane group")
	}
	member, ok := group.member()
	if !ok || member.position != 0 {
		t.Fatal("ordinary lane lacks its exact one-member capability")
	}
	bottom, err := algebra.writeOrdinaryFactor(formalTupleTestLive(t, algebra, 1), group, bottomFactor)
	if err != nil {
		t.Fatal(err)
	}
	top, err := algebra.writeOrdinaryFactor(formalTupleTestLive(t, algebra, 1), group, topFactor)
	if err != nil {
		t.Fatal(err)
	}
	if !algebra.lessOrEq(bottom, top) || algebra.lessOrEq(top, bottom) || algebra.err() != nil {
		t.Fatalf("ordinary order Bottom<=Top=%t Top<=Bottom=%t err=%v", algebra.lessOrEq(bottom, top), algebra.lessOrEq(top, bottom), algebra.err())
	}
	for _, sample := range []struct {
		name        string
		op          formalComponentBinaryOp
		left, right formalRelationTuple
		want        state.LaneFactor
	}{
		{name: "join", op: formalComponentJoin, left: bottom, right: top, want: topFactor},
		{name: "meet", op: formalComponentMeet, left: top, right: bottom, want: bottomFactor},
	} {
		got := algebra.combine(sample.op, sample.left, sample.right)
		want, err := algebra.writeOrdinaryFactor(formalTupleTestLive(t, algebra, 1), group, sample.want)
		if err != nil || algebra.err() != nil || !algebra.equal(got, want) {
			t.Fatalf("ordinary %s differs from registered lane law: write=%v algebra=%v", sample.name, err, algebra.err())
		}
	}
	widenFactor, err := authority.product.LaneWiden(bottomFactor, topFactor)
	if err != nil {
		t.Fatal(err)
	}
	widenWant, err := algebra.writeOrdinaryFactor(formalTupleTestLive(t, algebra, 1), group, widenFactor)
	if err != nil {
		t.Fatal(err)
	}
	if got := algebra.combine(formalComponentWiden, bottom, top); algebra.err() != nil || !algebra.equal(got, widenWant) {
		t.Fatalf("ordinary widen differs from registered lane law: %v", algebra.err())
	}
	narrowFactor, err := authority.product.LaneNarrow(topFactor, bottomFactor)
	if err != nil {
		t.Fatal(err)
	}
	narrowWant, err := algebra.writeOrdinaryFactor(formalTupleTestLive(t, algebra, 1), group, narrowFactor)
	if err != nil {
		t.Fatal(err)
	}
	if got := algebra.combine(formalComponentNarrow, top, bottom); algebra.err() != nil || !algebra.equal(got, narrowWant) {
		t.Fatalf("ordinary narrow differs from registered lane law: %v", algebra.err())
	}
}

func TestFormalTupleCoordinateGroupUsesWholeRegisteredLaneAlgebra(t *testing.T) {
	program := formalTupleTestProgram(t, "coordinate-group")
	algebra := formalTupleTestAlgebra(t, program)
	span, _, authority, _ := algebra.span(1)
	var group formalCoordinateLaneFiberGroup
	for _, descriptor := range span.groupDescriptors() {
		if descriptor.kind == formalFiberGroupCoordinateLane {
			group = formalCoordinateLaneFiberGroup{descriptor: descriptor}
			break
		}
	}
	if !group.valid() {
		t.Fatal("formal coordinate lane group is missing")
	}
	bottomFactor, err := authority.product.LaneBottom(group.descriptor.lane)
	if err != nil {
		t.Fatal(err)
	}
	topFactor, err := authority.product.LaneTop(group.descriptor.lane)
	if err != nil {
		t.Fatal(err)
	}
	bottom, err := algebra.writeCoordinateFactor(formalTupleTestLive(t, algebra, 1), group, bottomFactor)
	if err != nil {
		t.Fatal(err)
	}
	top, err := algebra.writeCoordinateFactor(formalTupleTestLive(t, algebra, 1), group, topFactor)
	if err != nil {
		t.Fatal(err)
	}
	if !algebra.lessOrEq(bottom, top) || algebra.lessOrEq(top, bottom) || algebra.err() != nil {
		t.Fatalf("coordinate order Bottom<=Top=%t Top<=Bottom=%t err=%v", algebra.lessOrEq(bottom, top), algebra.lessOrEq(top, bottom), algebra.err())
	}
	join := algebra.combine(formalComponentJoin, bottom, top)
	if algebra.err() != nil || !algebra.equal(join, top) {
		t.Fatalf("coordinate Join(Bottom, Top) differs: err=%v", algebra.err())
	}
	meet := algebra.combine(formalComponentMeet, top, bottom)
	if algebra.err() != nil || !algebra.equal(meet, bottom) {
		t.Fatalf("coordinate Meet(Top, Bottom) differs: err=%v", algebra.err())
	}
	for _, family := range group.descriptor.coordinateFamilies {
		value, err := algebra.directories[0].valueAt(bottom.root, family.skeleton)
		if err != nil || decisionRef(value) != decisionFalse {
			t.Fatalf("coordinate Bottom retained explicit skeleton %d/%v", value, err)
		}
	}
	widen := algebra.combine(formalComponentWiden, bottom, top)
	if algebra.err() != nil || !algebra.equal(widen, top) {
		t.Fatalf("coordinate Widen(Bottom, Top) differs: err=%v", algebra.err())
	}
	narrow := algebra.combine(formalComponentNarrow, top, bottom)
	wantNarrowFactor, err := authority.product.LaneNarrow(topFactor, bottomFactor)
	if err != nil {
		t.Fatal(err)
	}
	wantNarrow, err := algebra.writeCoordinateFactor(formalTupleTestLive(t, algebra, 1), group, wantNarrowFactor)
	if err != nil {
		t.Fatal(err)
	}
	if algebra.err() != nil || !algebra.equal(narrow, wantNarrow) {
		t.Fatalf("coordinate Narrow(Top, Bottom) differs from registered lane law: err=%v", algebra.err())
	}
}

func TestFormalTupleRunOwnedArenasAreConcurrentAndIsolated(t *testing.T) {
	program := formalTupleTestProgram(t, "run-isolation")
	const runs = 16
	results := make(chan *formalTupleAlgebra, runs)
	errs := make(chan error, runs)
	var wait sync.WaitGroup
	for index := 0; index < runs; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			algebra, err := newFormalTupleAlgebra(context.Background(), program)
			if err != nil {
				errs <- err
				return
			}
			results <- algebra
		}()
	}
	wait.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	seenComponents := make(map[*formalComponentTerminalArena]bool, runs)
	seenDirectories := make(map[*formalFiberDirectoryArena]bool, runs)
	var firstTuple formalRelationTuple
	count := 0
	for algebra := range results {
		count++
		if seenComponents[algebra.components] || seenDirectories[algebra.directories[0]] {
			t.Fatal("concurrent tuple runs shared mutable component or directory authority")
		}
		seenComponents[algebra.components] = true
		seenDirectories[algebra.directories[0]] = true
		if len(algebra.decisions.nodes) != 2 || algebra.directories[0].nodeCount() != 0 {
			t.Fatal("new tuple run inherited mutable decision or directory history")
		}
		tuple := formalTupleTestLive(t, algebra, 1)
		span, _, authority, _ := algebra.span(1)
		var laneDescriptor formalFiberDescriptor
		for _, descriptor := range span.forest.descriptors[span.first : span.first+span.count] {
			if descriptor.role == formalFiberOrdinaryLane {
				laneDescriptor = descriptor
				break
			}
		}
		if laneDescriptor.role == formalFiberInvalid {
			t.Fatal("registered product has no independent scalar lane")
		}
		defaultValue, err := authority.defaultFor(context.Background(), laneDescriptor)
		if err != nil || defaultValue.kind != formalComponentDefaultTerminal {
			t.Fatalf("run-local descriptor default = %#v, %v", defaultValue, err)
		}
		tuple, err = algebra.writeScalar(tuple, laneDescriptor, algebra.decisions.terminal(defaultValue.leaf))
		if err != nil || tuple.root.owner != algebra.directories[0] || algebra.directories[0].nodeCount() == 0 || len(algebra.components.terminals) <= 2 {
			t.Fatalf("run-local mutation was not retained by its owner: tuple=%#v err=%v", tuple, err)
		}
		if count == 1 {
			firstTuple = tuple
		} else if tuple.root.owner == firstTuple.root.owner {
			t.Fatal("mutated tuple roots share a run-owned directory")
		}
	}
	if count != runs {
		t.Fatalf("constructed runs = %d, want %d", count, runs)
	}
}

func formalTupleTestProgram(t *testing.T, label string) *RelationProgram {
	t.Helper()
	body := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("formal-tuple-" + label)))
	program := formalComponentTestProgram(t, []lexicalidentity.StableLexicalBodyID{body})
	slots, err := freezeSlotSpace(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalSlots = slots
	fibers, err := freezeFormalFiberInventoryWithSlots(program, slots)
	if err != nil {
		t.Fatal(err)
	}
	program.formalFibers = fibers
	components, err := freezeFormalComponentTerminalSchema(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalComponents = components
	return program
}

func formalTupleTestAlgebra(t *testing.T, program *RelationProgram) *formalTupleAlgebra {
	t.Helper()
	algebra, err := newFormalTupleAlgebra(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	return algebra
}

func formalTupleTestLive(t *testing.T, algebra *formalTupleAlgebra, variable relationVar) formalRelationTuple {
	t.Helper()
	_, directory, _, ok := algebra.span(variable)
	if !ok {
		t.Fatal("formal tuple span is missing")
	}
	tuple := formalRelationTuple{variable: variable, root: directory.defaultRoot()}
	tuple, err := algebra.writeCare(tuple, decisionTrue)
	if err != nil {
		t.Fatal(err)
	}
	return tuple
}
