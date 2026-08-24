package engine

import (
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// TestFactorSlotLayoutIsTheOrderOfSemanticIdentity names a mechanism three
// separate lanes have now rediscovered from its symptom.
//
// A Program's Factor slots are assigned by sorting the bound Factors on
// identity.CompareSemanticKey, which compares the bytes of a CONTENT DIGEST.
// A Factor's semantic identity digests its whole declaration surface - for an
// axis, that includes its member catalog, down to every declared member's key
// string - so an edit anywhere in an axis's declaration can move that axis's
// digest past another axis's, exchange their slots, and reorder every
// slot-ordered traversal downstream of them.
//
// That is why a commit that only ADDS a declaration no Program references
// moves solve counters: bench/fibonacci measures 13, 15, 16 or 17 WTO passes
// depending on nothing but the key string of one unreferenced heap reducer
// row, with identical evaluate and body counts each time. The solve does the
// same work; it visits it in a different order.
//
// Two consequences follow, and both are the reason this is stated rather than
// left to be found again. A ladder comparison is only a measurement when both
// sides declare the same surface: a counter delta across a declaration-only
// commit is a re-layout, not a regression. And the physical slot layout is
// currently BORROWED from the authored-identity ordering, which
// CompareSemanticKey documents as an ordering over authored identities with
// physical layout to be derived later. Deriving that layout - from the
// dependency structure the schedule actually follows - is the change that
// would make a slot order mean something; until it lands, this is the law.
func TestFactorSlotLayoutIsTheOrderOfSemanticIdentity(t *testing.T) {
	fixture := newReadLaneFixture(t)
	plane := fixture.plane
	if plane == nil || len(plane.ordered) < 2 || len(plane.factors) != len(plane.ordered) {
		t.Fatalf("plane holds %d ordered factors, want at least two to order", len(plane.ordered))
	}
	for index := 1; index < len(plane.ordered); index++ {
		previous := plane.ordered[index-1].semantic()
		current := plane.ordered[index].semantic()
		if identity.CompareSemanticKey(previous, current) >= 0 {
			t.Fatalf("slot %d holds an identity that does not follow slot %d's", index, index-1)
		}
	}

	// The layout is a function of the identities alone: it is the sort of the
	// bound Factors on their digests, independent of the order they were
	// enumerated in. That independence is the property the digest ordering was
	// adopted for, and it is real - registration order cannot reach the layout.
	// What reaches it instead is every byte the digest covers.
	want := append([]runtimeFactor(nil), plane.factors...)
	sort.Slice(want, func(left, right int) bool {
		return identity.CompareSemanticKey(want[left].semantic(), want[right].semantic()) < 0
	})
	for index := range want {
		if want[index].semantic() != plane.ordered[index].semantic() {
			t.Fatalf("slot %d does not hold the identity the digest order puts there", index)
		}
	}
}
