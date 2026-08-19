package heap

import "testing"

// cellStateProofOwner builds the minimal *schema needed to construct owned
// Present tuples without the full seal pipeline: two slots and two payloads
// give canonicalCellState real, distinct bounds to check against.
func cellStateProofOwner() *schema {
	return &schema{slots: make([]slotRow, 2), payloads: make([]payloadRow, 2)}
}

func cellStateProofPresent(owner *schema, slotID, payloadID uint32) Present {
	return Present{
		owner:            owner,
		slotID:           slotID,
		payloadID:        payloadID,
		valueContainment: Containment{owner: owner, kind: ContainmentNone},
		keyContainment:   Containment{owner: owner, kind: ContainmentNone},
	}
}

// canonicalCellState is the single construction gate for a fresh CellState
// presents image: it is the one place ownership, individual Present
// validity, and strict ascending order are proved.
func TestCanonicalCellStateGateRefusesUnsortedOrForeignPresents(t *testing.T) {
	owner := cellStateProofOwner()
	foreign := cellStateProofOwner()

	low := cellStateProofPresent(owner, 1, 1)
	high := cellStateProofPresent(owner, 2, 1)
	foreignPresent := cellStateProofPresent(foreign, 1, 1)

	if _, ok := canonicalCellState(owner, RawPresent, []Present{high, low}); ok {
		t.Fatal("canonicalCellState admitted a descending presents image")
	}
	if _, ok := canonicalCellState(owner, RawPresent, []Present{low, low}); ok {
		t.Fatal("canonicalCellState admitted a repeated present")
	}
	if _, ok := canonicalCellState(owner, RawPresent, []Present{foreignPresent}); ok {
		t.Fatal("canonicalCellState admitted a foreign-owner present")
	}
	if _, ok := canonicalCellState(owner, RawAbsent, []Present{low}); ok {
		t.Fatal("canonicalCellState admitted RawAbsent with a non-empty presents image")
	}
	if _, ok := canonicalCellState(owner, RawPresent, nil); ok {
		t.Fatal("canonicalCellState admitted RawPresent with an empty presents image")
	}

	state, ok := canonicalCellState(owner, RawPresent, []Present{low, high})
	if !ok {
		t.Fatal("canonicalCellState refused an ascending, owner-consistent presents image")
	}
	if !state.valid() {
		t.Fatal("canonicalCellState produced a state that fails its own residue check")
	}
}

// An algebra entry costs O(1) in the size of the cell it is handed. The
// construction gate above already proved sorted order and Present validity,
// and a CellState's presents image is immutable, so no entry re-derives that
// proof. Corrupting an already constructed image is the one observable that
// separates a constant-time answer from a walk: a walking implementation
// reports the corruption, a construction-proved one cannot see it.
func TestCellStateAlgebraEntryDoesNotRereadOwnedImage(t *testing.T) {
	owner := cellStateProofOwner()
	low := cellStateProofPresent(owner, 1, 1)
	high := cellStateProofPresent(owner, 2, 1)

	state, ok := canonicalCellState(owner, RawPresent, []Present{low, high})
	if !ok {
		t.Fatal("construction gate refused a valid ascending image")
	}

	// Reverse the proven order in place. This violates every clause
	// canonicalCellState proved: sorted order is now false, yet the residue
	// below must not detect it because it is O(1) and never loops.
	state.presents[0], state.presents[1] = state.presents[1], state.presents[0]

	if !state.valid() {
		t.Fatal("valid() walked the presents image instead of trusting the construction proof")
	}
	if state.PresentCount() != 2 {
		t.Fatal("PresentCount changed because valid() re-derived canonical form")
	}
	if !equalCellState(state, state) {
		t.Fatal("equalCellState walked the image instead of answering identity")
	}
	if !cellStateLessOrEqAdmitted(state, state) {
		t.Fatal("cellStateLessOrEqAdmitted walked the image instead of answering reflexivity by identity")
	}

	merged, mergedOK := mergeCellStatesAdmitted(state, state)
	if !mergedOK || len(merged.presents) != len(state.presents) || &merged.presents[0] != &state.presents[0] {
		t.Fatal("mergeCellStatesAdmitted rebuilt an image for an identity operand pair")
	}
}

// mergeCellStatesAdmitted must answer a pointwise-dominated merge by reusing
// the dominating operand's proven image, using the existing
// cellStateLessOrEqAdmitted inclusion test rather than rebuilding a union.
func TestMergeCellStatesReusesDominatingOperand(t *testing.T) {
	owner := cellStateProofOwner()
	low := cellStateProofPresent(owner, 1, 1)
	high := cellStateProofPresent(owner, 2, 1)

	small, ok := canonicalCellState(owner, RawPresent, []Present{low})
	if !ok {
		t.Fatal("construction gate refused a valid singleton image")
	}
	big, ok := canonicalCellState(owner, RawPresent, []Present{low, high})
	if !ok {
		t.Fatal("construction gate refused a valid two-present image")
	}

	merged, mergedOK := mergeCellStatesAdmitted(small, big)
	if !mergedOK || len(merged.presents) != len(big.presents) || &merged.presents[0] != &big.presents[0] {
		t.Fatal("merge of a dominated operand rebuilt an image instead of reusing the dominating one")
	}

	merged, mergedOK = mergeCellStatesAdmitted(big, small)
	if !mergedOK || len(merged.presents) != len(big.presents) || &merged.presents[0] != &big.presents[0] {
		t.Fatal("merge of a dominating operand rebuilt an image instead of reusing itself")
	}
}

// BenchmarkMergeCellStatesAdmitted isolates the profiled hot path: the
// empty-side and dominated-operand cases must cost zero allocations, and a
// genuine disjoint merge costs exactly one.
func BenchmarkMergeCellStatesAdmitted(b *testing.B) {
	owner := cellStateProofOwner()
	empty, _ := canonicalCellState(owner, RawAbsent, nil)
	small, _ := canonicalCellState(owner, RawPresent, []Present{cellStateProofPresent(owner, 1, 1)})
	big, _ := canonicalCellState(owner, RawPresent, []Present{cellStateProofPresent(owner, 1, 1), cellStateProofPresent(owner, 2, 1)})
	disjointLeft, _ := canonicalCellState(owner, RawPresent, []Present{cellStateProofPresent(owner, 1, 1)})
	disjointRight, _ := canonicalCellState(owner, RawPresent, []Present{cellStateProofPresent(owner, 2, 1)})

	b.Run("empty_side", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			mergeCellStatesAdmitted(empty, big)
		}
	})
	b.Run("dominated_operand", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			mergeCellStatesAdmitted(small, big)
		}
	})
	b.Run("same_image", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			mergeCellStatesAdmitted(big, big)
		}
	})
	b.Run("disjoint_merge", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			mergeCellStatesAdmitted(disjointLeft, disjointRight)
		}
	})
}
