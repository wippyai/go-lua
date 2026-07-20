package transformer

import (
	"testing"
)

func TestFormalTupleGuardCompositionUsesRegisteredProductJoin(t *testing.T) {
	program, boundary, sourceRank := formalTupleGuardTestProgram(t, "product-join")
	algebra := formalTupleTestAlgebra(t, program)
	span, directory, authority, ok := algebra.span(1)
	if !ok {
		t.Fatal("formal tuple span is missing")
	}
	var group formalOrdinaryLaneFiberGroup
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
			group = formalOrdinaryLaneFiberGroup{descriptor: descriptor}
			bottomTuple, err := algebra.writeOrdinaryFactor(formalTupleTestLive(t, algebra, 1), group, bottom)
			if err != nil {
				t.Fatal(err)
			}
			topTuple, err := algebra.writeOrdinaryFactor(formalTupleTestLive(t, algebra, 1), group, top)
			if err != nil {
				t.Fatal(err)
			}
			bottomRoots, err := algebra.groupRoots(bottomTuple, descriptor)
			if err != nil {
				t.Fatal(err)
			}
			topRoots, err := algebra.groupRoots(topTuple, descriptor)
			if err != nil {
				t.Fatal(err)
			}
			conditional := make([]decisionRef, len(bottomRoots))
			for index := range conditional {
				conditional[index] = algebra.decisions.branch(sourceRank, bottomRoots[index], topRoots[index])
			}
			root, err := algebra.applyGroupRoots(span, directory, bottomTuple.root, authority, descriptor, conditional)
			if err != nil {
				t.Fatal(err)
			}
			input := formalRelationTuple{variable: 1, root: root}
			got, err := algebra.composeGuardBoundary(input, boundary)
			if err != nil {
				t.Fatal(err)
			}
			if !algebra.equal(got, topTuple) || algebra.err() != nil {
				t.Fatalf("closed product branch did not use registered Join: equal=%t err=%v", algebra.equal(got, topTuple), algebra.err())
			}
			formalTupleGuardTestNoRank(t, algebra, got, boundary.close)
			return
		}
	}
	t.Fatal("formal product has no nontrivial ordinary lane")
}

func TestFormalTupleGuardCompositionClosesCareAndRejectsForeignRuns(t *testing.T) {
	program, boundary, sourceRank := formalTupleGuardTestProgram(t, "care-run-owner")
	algebra := formalTupleTestAlgebra(t, program)
	input := formalTupleTestLive(t, algebra, 1)
	conditionalCare := algebra.decisions.branch(sourceRank, decisionFalse, decisionTrue)
	input, err := algebra.writeCare(input, conditionalCare)
	if err != nil {
		t.Fatal(err)
	}
	got, err := algebra.composeGuardBoundary(input, boundary)
	if err != nil {
		t.Fatal(err)
	}
	care, err := algebra.care(got)
	if err != nil || care != decisionTrue {
		t.Fatalf("closed Care = %d, want true: %v", care, err)
	}
	formalTupleGuardTestNoRank(t, algebra, got, boundary.close)

	otherRun := formalTupleTestAlgebra(t, program)
	if _, err := otherRun.composeGuardBoundary(got, boundary); err == nil {
		t.Fatal("tuple guard composition accepted a foreign run tuple")
	}
	foreignProgram, foreignBoundary, _ := formalTupleGuardTestProgram(t, "foreign-boundary")
	_ = foreignProgram
	if _, err := algebra.composeGuardBoundary(got, foreignBoundary); err == nil {
		t.Fatal("tuple guard composition accepted a foreign vocabulary boundary")
	}
}

func TestFormalTupleGuardCompositionGuardFreeIsZeroAllocationIdentity(t *testing.T) {
	program, _, _ := formalTupleGuardTestProgram(t, "guard-free-identity")
	algebra := formalTupleTestAlgebra(t, program)
	tuple := formalTupleTestLive(t, algebra, 1)
	vocabulary := program.formalGuards
	boundary := formalGuardBoundary{
		owner: vocabulary, rename: formalGuardRankMap{owner: vocabulary},
		domain: formalGuardRankSet{owner: vocabulary}, close: formalGuardRankSet{owner: vocabulary},
	}
	if !boundary.validateClosure() {
		t.Fatal("guard-free boundary fixture is malformed")
	}
	var got formalRelationTuple
	var runErr error
	allocations := testing.AllocsPerRun(100, func() {
		got, runErr = algebra.composeGuardBoundary(tuple, boundary)
	})
	if runErr != nil || got.variable != tuple.variable || got.root != tuple.root {
		t.Fatalf("guard-free composition = %#v, %v; want physical identity %#v", got, runErr, tuple)
	}
	if allocations != 0 {
		t.Fatalf("guard-free composition allocations = %g, want zero", allocations)
	}
}

func formalTupleGuardTestProgram(t *testing.T, label string) (*RelationProgram, formalGuardBoundary, uint32) {
	t.Helper()
	program := formalTupleTestProgram(t, label)
	arena := program.bodies[0].relation.arena
	keys := []formalGuardRankKey{
		{variable: 1, arena: arena, term: ValueTerm(1)},
		{variable: 1, arena: arena, term: ValueTerm(2)},
		{variable: 1, arena: arena, term: ValueTerm(3)},
	}
	vocabulary := &formalGuardVocabulary{
		ranks: map[formalGuardRankKey]uint32{
			keys[0]: 0,
			keys[1]: 1,
			keys[2]: 2,
		},
		apply:       make(map[formalRelationCell]formalGuardBoundary),
		definitions: make(map[formalRelationDefinitionRef]formalGuardBoundary),
		loops:       make(map[formalGuardLoopLifetime]formalGuardRankSet),
		size:        3,
		sealed:      true,
	}
	boundary := formalGuardBoundary{
		owner:  vocabulary,
		rename: formalGuardRankMap{owner: vocabulary, pairs: []formalGuardRankPair{{source: 2, target: 1}}},
		domain: formalGuardRankSet{owner: vocabulary, ranks: []uint32{2}},
		close:  formalGuardRankSet{owner: vocabulary, ranks: []uint32{1}},
	}
	if !vocabulary.valid() || !boundary.valid() || !boundary.validateClosure() {
		t.Fatal("formal guard boundary fixture is malformed")
	}
	program.formalGuards = vocabulary
	return program, boundary, 2
}

func formalTupleGuardTestNoRank(t *testing.T, algebra *formalTupleAlgebra, tuple formalRelationTuple, forbidden formalGuardRankSet) {
	t.Helper()
	if err := algebra.validateTuple(tuple); err != nil {
		t.Fatal(err)
	}
	span, directory, _, ok := algebra.span(tuple.variable)
	if !ok || tuple.root.owner != directory {
		t.Fatal("closed tuple has foreign ownership")
	}
	for ordinal := 0; ordinal < span.count; ordinal++ {
		root, err := directory.valueAt(tuple.root, formalFiberOrdinal(ordinal))
		if err != nil {
			t.Fatal(err)
		}
		if err := forbidden.owner.validateDecisionRoot(&algebra.decisions, decisionRef(root), forbidden); err != nil {
			t.Fatal(err)
		}
	}
}
