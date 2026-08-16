package engine

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

func TestReceiptObservationMatchesIDFencesPrivateSeal(t *testing.T) {
	first := keyspace.ContentID(sha256.Sum256([]byte("receipt-observation-first")))
	second := keyspace.ContentID(sha256.Sum256([]byte("receipt-observation-second")))
	observation := ReceiptObservation[uint64]{owner: &receiptObservationOwner{}, id: first, ordinal: 0}
	if !observation.MatchesID(first) || observation.MatchesID(second) || observation.MatchesID(keyspace.ContentID{}) {
		t.Fatal("receipt observation ID fence")
	}
	spliced := observation
	spliced.id = second
	if !spliced.MatchesID(second) || spliced.MatchesID(first) {
		t.Fatal("receipt observation stored ID recomputation fence")
	}
}
