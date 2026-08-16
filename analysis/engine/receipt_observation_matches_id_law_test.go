package engine

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestReceiptObservationMatchesIDFencesPrivateSeal(t *testing.T) {
	first := identity.ContentID(sha256.Sum256([]byte("receipt-observation-first")))
	second := identity.ContentID(sha256.Sum256([]byte("receipt-observation-second")))
	observation := ReceiptObservation[uint64]{owner: &receiptObservationOwner{}, id: first, ordinal: 0}
	if !observation.MatchesID(first) || observation.MatchesID(second) || observation.MatchesID(identity.ContentID{}) {
		t.Fatal("receipt observation ID fence")
	}
	spliced := observation
	spliced.id = second
	if !spliced.MatchesID(second) || spliced.MatchesID(first) {
		t.Fatal("receipt observation stored ID recomputation fence")
	}
}
