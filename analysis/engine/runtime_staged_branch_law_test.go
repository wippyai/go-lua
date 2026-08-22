// runtime_staged_branch_law_test.go fences the value-vector ownership of one
// staged cross-product round. The round consumes its parent partial, so the
// parent vector belongs to the first branch and only further branches of the
// same observation own a clone.

package engine

import (
	"slices"
	"testing"
	"unsafe"
)

// stagedBranchSpecification is the unconditional-copy specification of one
// staged unit round: every branch of every parent receives its own copy of the
// parent vector with the round's slot rewritten.
func stagedBranchSpecification(parents [][]uint64, slot int, fanout []uint64) [][]uint64 {
	next := make([][]uint64, 0, len(parents)*len(fanout))
	for _, parent := range parents {
		for _, value := range fanout {
			values := append([]uint64(nil), parent...)
			values[slot] = value
			next = append(next, values)
		}
	}
	return next
}

// stagedBranchRound drives the same round through the production owner.
func stagedBranchRound(t testing.TB, parents [][]uint64, slot int, fanout []uint64) [][]uint64 {
	t.Helper()
	next := make([][]uint64, 0, len(parents)*len(fanout))
	for _, parent := range parents {
		branch := stagedBranch[uint64]{parent: parent, slot: slot}
		for _, value := range fanout {
			values, ok := branch.values(value)
			if !ok {
				t.Fatalf("staged branch refused slot %d of width %d", slot, len(parent))
			}
			next = append(next, values)
		}
	}
	return next
}

func TestStagedBranchDonatesTheParentVectorOnceAndClonesEveryFurtherBranch(t *testing.T) {
	parent := []uint64{7, 0, 0}
	branch := stagedBranch[uint64]{parent: parent, slot: 1}

	first, firstOK := branch.values(11)
	second, secondOK := branch.values(22)
	third, thirdOK := branch.values(33)
	if !firstOK || !secondOK || !thirdOK {
		t.Fatal("staged branch refused a declared branch")
	}
	if unsafe.SliceData(first) != unsafe.SliceData(parent) {
		t.Fatal("staged branch copied the consumed parent vector instead of donating it")
	}
	if unsafe.SliceData(second) == unsafe.SliceData(first) || unsafe.SliceData(third) == unsafe.SliceData(first) ||
		unsafe.SliceData(third) == unsafe.SliceData(second) {
		t.Fatal("staged branch aliased two sibling branches onto one vector")
	}
	// A clone is taken after the donated branch already rewrote the round slot.
	// The slot is the only distinction a sibling round can hold, so each clone
	// must still carry its own branch value and the shared prefix.
	if !slices.Equal(first, []uint64{7, 11, 0}) || !slices.Equal(second, []uint64{7, 22, 0}) || !slices.Equal(third, []uint64{7, 33, 0}) {
		t.Fatalf("staged branch vectors = %v %v %v", first, second, third)
	}
}

func TestStagedBranchRejectsASlotOutsideTheParentWidth(t *testing.T) {
	parent := []uint64{1, 2}
	for _, slot := range []int{-1, 2, 7} {
		branch := stagedBranch[uint64]{parent: parent, slot: slot}
		if _, ok := branch.values(9); ok {
			t.Fatalf("staged branch admitted slot %d of width %d", slot, len(parent))
		}
	}
	var absent *stagedBranch[uint64]
	if _, ok := absent.values(9); ok {
		t.Fatal("staged branch admitted an absent round")
	}
}

// TestStagedBranchComposesTheUnconditionalCopyCrossProduct pins the semantic
// equivalence the donation relies on: composing rounds through the production
// owner yields exactly the vectors an unconditional per-observation copy
// yields, in the same order, for every fan-out.
func TestStagedBranchComposesTheUnconditionalCopyCrossProduct(t *testing.T) {
	const width = 3
	for _, fanout := range [][]uint64{{11}, {11, 22}, {11, 22, 33}} {
		specified := [][]uint64{make([]uint64, width)}
		composed := [][]uint64{make([]uint64, width)}
		for slot := 0; slot < width; slot++ {
			specified = stagedBranchSpecification(specified, slot, fanout)
			composed = stagedBranchRound(t, composed, slot, fanout)
		}
		if len(specified) != len(composed) {
			t.Fatalf("fan-out %v: composed %d partials, specification has %d", fanout, len(composed), len(specified))
		}
		for index := range specified {
			if !slices.Equal(specified[index], composed[index]) {
				t.Fatalf("fan-out %v partial %d = %v, specification = %v", fanout, index, composed[index], specified[index])
			}
		}
		for left := range composed {
			for right := left + 1; right < len(composed); right++ {
				if unsafe.SliceData(composed[left]) == unsafe.SliceData(composed[right]) {
					t.Fatalf("fan-out %v aliased partials %d and %d", fanout, left, right)
				}
			}
		}
	}
}

// TestStagedBranchAllocatesOnlyPerAdditionalBranch documents the exact bound:
// observing a unit that refines its parent into one region allocates no value
// vector at all, and each further region of the same observation allocates
// exactly one.
func TestStagedBranchAllocatesOnlyPerAdditionalBranch(t *testing.T) {
	branch := stagedBranch[uint64]{parent: make([]uint64, 1024), slot: 512}
	donation := testing.AllocsPerRun(200, func() {
		branch.donated = false
		if _, ok := branch.values(1); !ok {
			t.Fatal("staged branch refused the donated branch")
		}
	})
	if donation != 0 {
		t.Fatalf("donated staged branch allocated %v vectors, want 0", donation)
	}
	sibling := testing.AllocsPerRun(200, func() {
		branch.donated = true
		if _, ok := branch.values(2); !ok {
			t.Fatal("staged branch refused the sibling branch")
		}
	})
	if sibling != 1 {
		t.Fatalf("sibling staged branch allocated %v vectors, want 1", sibling)
	}
}

func BenchmarkStagedBranchSingleRegionRound1024(b *testing.B) {
	branch := stagedBranch[uint64]{parent: make([]uint64, 1024), slot: 512}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		branch.donated = false
		if _, ok := branch.values(uint64(iteration)); !ok {
			b.Fatal("staged branch")
		}
	}
}
