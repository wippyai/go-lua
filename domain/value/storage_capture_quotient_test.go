package value

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func storageCaptureQuotientLawID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

func TestStorageCaptureQuotientSharesMutableCaptureAcrossTransitiveEdges(t *testing.T) {
	outer := storageCaptureQuotientLawID(3)
	middle := storageCaptureQuotientLawID(2)
	inner := storageCaptureQuotientLawID(1)
	quotient := make(storageCaptureQuotient)
	for _, id := range []identity.ContentID{outer, middle, inner} {
		if !quotient.add(id) {
			t.Fatal("capture storage identity was not admitted")
		}
	}
	// A mutable upvalue may cross more than one closure boundary.  The bind at
	// the outer edge and the read at the inner edge must therefore redeem the
	// same representative, even when the middle edge is joined separately.
	if !quotient.link(middle, outer) || !quotient.link(inner, middle) {
		t.Fatal("capture edges were not joined")
	}
	bindCoordinate, bindOK := quotient.canonical(outer)
	readCoordinate, readOK := quotient.canonical(inner)
	if !bindOK || !readOK || bindCoordinate != readCoordinate {
		t.Fatalf("mutable capture endpoints diverged: bind=%v/%t read=%v/%t", bindCoordinate, bindOK, readCoordinate, readOK)
	}
	if bindCoordinate != outer {
		t.Fatalf("quotient representative is not outer owner: got %v want %v", bindCoordinate, outer)
	}
}

func TestStorageCaptureQuotientKeepsUnrelatedCellsSeparate(t *testing.T) {
	left := storageCaptureQuotientLawID(11)
	right := storageCaptureQuotientLawID(21)
	quotient := make(storageCaptureQuotient)
	if !quotient.add(left) || !quotient.add(right) {
		t.Fatal("capture storage identities were not admitted")
	}
	leftCanonical, leftOK := quotient.canonical(left)
	rightCanonical, rightOK := quotient.canonical(right)
	if !leftOK || !rightOK || leftCanonical == rightCanonical {
		t.Fatal("unrelated mutable cells were merged")
	}
}
