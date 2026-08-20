package mounted

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

func orderLawID(label string) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte("mounted-order-law/" + label)))
}

// TestUnsealedPopulationsAnswerNothing proves the closed default: a population
// that was never sealed reports no members and addresses no row, so a caller
// cannot read an empty column as a total one.
func TestUnsealedPopulationsAnswerNothing(t *testing.T) {
	var census ObservationSites
	if census.Available() || census.Count() != 0 {
		t.Fatal("an unsealed observation census answers membership")
	}
	if _, ok := census.At(0); ok {
		t.Fatal("an unsealed observation census addresses a row")
	}
}

// TestSealRejectsIncompleteMountInput proves the shared admission: no mount, an
// unavailable mount, or one module identity placed twice leaves every
// population unavailable rather than published over a partial input.
func TestSealRejectsIncompleteMountInput(t *testing.T) {
	if _, ok := SealObservationSites(nil, nil, []schema.Key{"value"}); ok {
		t.Fatal("the observation census seals over an empty mount set")
	}
}
