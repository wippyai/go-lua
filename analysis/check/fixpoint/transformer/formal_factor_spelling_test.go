package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/state"
)

func TestFormalFactorSpellingLookupIsExactAndProducerOwned(t *testing.T) {
	newFixture := func(t *testing.T) (*formalTupleAlgebra, formalTupleLeafEvaluator, formalFiberGroupDescriptor, []decisionLeaf, state.LaneFactor) {
		t.Helper()
		algebra := formalTupleTestAlgebra(t, formalTupleTestProgram(t, t.Name()))
		regions, err := algebra.tupleLeafRegions(formalTupleTestLive(t, algebra, 1))
		if err != nil || len(regions) != 1 {
			t.Fatalf("bottom regions = %d, %v", len(regions), err)
		}
		evaluator := regions[0].evaluator
		var group formalFiberGroupDescriptor
		for _, candidate := range evaluator.layout.nonValues {
			if candidate.kind == formalFiberGroupCoordinateLane {
				group = candidate
				break
			}
		}
		if !group.valid() {
			t.Fatal("coordinate factor group is absent")
		}
		leaves, err := evaluator.leaves.group(group)
		if err != nil {
			t.Fatal(err)
		}
		bottom, err := evaluator.authority.product.LaneBottom(group.lane)
		if err != nil {
			t.Fatal(err)
		}
		return algebra, evaluator, group, leaves, bottom
	}

	t.Run("identity", func(t *testing.T) {
		algebra, evaluator, group, leaves, bottom := newFixture(t)
		key := formalFactorReachabilityKey{body: evaluator.authority.body, lane: group.lane.Ordinal(), hash: formalFactorLeafHash(leaves)}
		before := len(algebra.factorReachability[key])
		got, err := evaluator.laneFactor(group)
		if err != nil {
			t.Fatal(err)
		}
		same, err := evaluator.authority.product.LaneCanonicalRepresentationEqual(got, bottom)
		if err != nil || !same || len(algebra.factorReachability[key]) != before {
			t.Fatalf("retained factor identity = same %t err %v entries %d/%d", same, err, len(algebra.factorReachability[key]), before)
		}
		if allocations := testing.AllocsPerRun(1000, func() {
			if _, lookupErr := evaluator.laneFactor(group); lookupErr != nil {
				panic(lookupErr)
			}
		}); allocations != 0 {
			t.Fatalf("exact factor lookup allocations/run = %.2f, want 0", allocations)
		}
	})

	t.Run("hash collision checks full leaves", func(t *testing.T) {
		algebra, evaluator, group, leaves, bottom := newFixture(t)
		key := formalFactorReachabilityKey{body: evaluator.authority.body, lane: group.lane.Ordinal(), hash: formalFactorLeafHash(leaves)}
		decoyLeaves := append([]decisionLeaf(nil), leaves...)
		decoyLeaves[0]++
		decoyFactor, err := evaluator.authority.product.LaneTop(group.lane)
		if err != nil {
			t.Fatal(err)
		}
		algebra.factorReachability[key] = append([]formalFactorReachabilityEntry{{leaves: decoyLeaves, factor: decoyFactor}}, algebra.factorReachability[key]...)
		got, err := evaluator.laneFactor(group)
		if err != nil {
			t.Fatal(err)
		}
		same, err := evaluator.authority.product.LaneCanonicalRepresentationEqual(got, bottom)
		if err != nil || !same {
			t.Fatalf("collision selected a nonmatching factor: same=%t err=%v", same, err)
		}
	})

	t.Run("miss is a producer defect", func(t *testing.T) {
		algebra, evaluator, group, leaves, _ := newFixture(t)
		key := formalFactorReachabilityKey{body: evaluator.authority.body, lane: group.lane.Ordinal(), hash: formalFactorLeafHash(leaves)}
		delete(algebra.factorReachability, key)
		if _, err := evaluator.laneFactor(group); err == nil {
			t.Fatal("factor lookup reconstructed an unregistered producer spelling")
		}
	})

	t.Run("conflicting representation is rejected", func(t *testing.T) {
		algebra, evaluator, group, leaves, _ := newFixture(t)
		top, err := evaluator.authority.product.LaneTop(group.lane)
		if err != nil {
			t.Fatal(err)
		}
		if err := algebra.cacheFormalFactorReachability(evaluator.authority, group, leaves, top); err == nil {
			t.Fatal("one exact leaf spelling admitted conflicting lane representations")
		}
	})
}
