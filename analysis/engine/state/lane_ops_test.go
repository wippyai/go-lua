package state

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestLanesLessOrEqSkipsSharedLaneRepresentation(t *testing.T) {
	var comparisons int
	lanes := []laneOps{{
		same: func(State, State) bool { return true },
		lessOrEq: func(State, State) bool {
			comparisons++
			return true
		},
	}}
	if !lanesLessOrEq(lanes, State{}, State{}) {
		t.Fatal("shared lane should compare less-or-equal")
	}
	if comparisons != 0 {
		t.Fatalf("LessOrEq invoked lane comparison %d times for a shared lane", comparisons)
	}
}

func TestValueLaneDomainRecognizesSharedPersistentMaps(t *testing.T) {
	reg := standard.Registry()
	domain := valueLaneDomain(reg)
	values := map[key.Value]product.Value{key.SymbolValue(1): product.Absent(reg)}
	shared := valueLane{symbols: values}
	if !domain.Same(shared, shared) {
		t.Fatal("value lane did not recognize shared persistent maps")
	}
}
