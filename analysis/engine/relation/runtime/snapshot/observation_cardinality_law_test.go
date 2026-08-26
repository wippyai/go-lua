package snapshot

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func TestCompleteDenominatorIsNotAnObservationCardinality(t *testing.T) {
	cardinality, ok := model.NewCardinality(model.CompleteDenominator, 0)
	if !ok {
		t.Fatal("complete cardinality")
	}
	if cardinalityHolds(cardinality, 0) || cardinalityHolds(cardinality, 1) {
		t.Fatal("observation cardinality acquired a witness-free CompleteDenominator rule")
	}
}
