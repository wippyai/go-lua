package state

import (
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

func TestBoundaryRootSlotVocabularyIncludesCallResultsButNotExpressionScratch(t *testing.T) {
	keys := keyspace.New()
	callResult := statekey.CallResult(17, 3)
	if !validBoundaryRootSlot(callResult) {
		t.Fatal("point-owned call result is not an addressable boundary slot")
	}
	if _, err := SealBoundaryFactorSelection(keys, []BoundaryFactorRoot{{Slot: callResult}}, nil, false); err != nil {
		t.Fatalf("factor selection rejected point-owned call result: %v", err)
	}
	expression := statekey.ExpressionValue(17)
	if validBoundaryRootSlot(expression) {
		t.Fatal("expression scratch entered the structural boundary vocabulary")
	}
	if _, err := SealBoundaryFactorSelection(keys, []BoundaryFactorRoot{{Slot: expression}}, nil, false); err == nil {
		t.Fatal("factor selection accepted expression scratch as a boundary root")
	}
}
