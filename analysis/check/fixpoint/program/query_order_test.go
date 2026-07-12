package program

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestDependencyFirstSummaryKeysOrdersAcyclicCalleesBeforeCallers(t *testing.T) {
	caller := orderTestKey(1)
	middle := orderTestKey(2)
	leaf := orderTestKey(3)
	island := orderTestKey(4)
	original := []summary.SummaryKey{caller, middle, leaf, island}
	edges := map[summary.SummaryKey][]summary.SummaryKey{
		caller: {middle},
		middle: {leaf},
	}
	want := []summary.SummaryKey{leaf, middle, caller, island}
	if got := dependencyFirstSummaryKeys(original, edges); !reflect.DeepEqual(got, want) {
		t.Fatalf("dependency-first order = %v, want %v", got, want)
	}
}

func TestDependencyFirstSummaryKeysCondensesRecursiveSCCStably(t *testing.T) {
	left := orderTestKey(1)
	right := orderTestKey(2)
	caller := orderTestKey(3)
	// The preexisting order intentionally opposes SummaryKey order. Moving the
	// component is safe; reordering its members could change widening spelling.
	original := []summary.SummaryKey{right, left, caller}
	edges := map[summary.SummaryKey][]summary.SummaryKey{
		caller: {right},
		right:  {left},
		left:   {right},
	}
	want := []summary.SummaryKey{right, left, caller}
	if got := dependencyFirstSummaryKeys(original, edges); !reflect.DeepEqual(got, want) {
		t.Fatalf("SCC order = %v, want %v", got, want)
	}
}

func TestDependencyFirstSummaryKeysIgnoresUnknownDynamicTargets(t *testing.T) {
	left := orderTestKey(1)
	right := orderTestKey(2)
	unknown := orderTestKey(99)
	original := []summary.SummaryKey{left, right}
	edges := map[summary.SummaryKey][]summary.SummaryKey{left: {unknown}}
	want := []summary.SummaryKey{left, right}
	if got := dependencyFirstSummaryKeys(original, edges); !reflect.DeepEqual(got, want) {
		t.Fatalf("order with unknown target = %v, want %v", got, want)
	}
}

func orderTestKey(id symbol.ID) summary.SummaryKey {
	return summary.DefaultSummaryKey(ref.FromSymbol(id))
}
