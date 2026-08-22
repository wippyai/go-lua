package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestEmptySummaryProjectionObservesOneConstantRow is the totality law of a
// Factor's sealed empty projection. A summary declared over zero coordinates
// is the limit of the proper coordinate subsets a Factor already admits, so it
// reads a constant: exactly one observation row over the observed region,
// carrying zero typed entries.
//
// Emitting no row is not the same answer. The read boundary refines its
// product against one output per source row, so a summary that emits nothing
// deletes its source rows instead of contributing a constant, and the whole
// rule execution refuses at preflight.
func TestEmptySummaryProjectionObservesOneConstantRow(t *testing.T) {
	for _, keyEnd := range emptySummaryProjectionKeyEnds {
		binding, state, work := newEmptySummaryProjectionBinding(t, keyEnd)
		root, ok := state.HandleAt(0)
		if !ok {
			t.Fatalf("keyEnd %d: root handle", keyEnd)
		}
		slotWork, ok := work.SlotWork(0)
		if !ok {
			t.Fatalf("keyEnd %d: slot work", keyEnd)
		}
		if !slotWork.BeginObservation() {
			t.Fatalf("keyEnd %d: begin observation", keyEnd)
		}
		rows, entries := 0, -1
		completed := slotWork.ObserveUnder(root, emptySummaryProjectionUnit, state.Support(), func(row carrier.ObservationRow) bool {
			observation, resolved := binding.ResolveObservation(slotWork, row)
			if !resolved {
				return false
			}
			rows++
			entries = observation.Count()
			return true
		})
		if !completed || !slotWork.EndObservation() {
			t.Fatalf("keyEnd %d: observation", keyEnd)
		}
		if rows != 1 {
			t.Fatalf("keyEnd %d: empty projection emitted %d rows, want 1", keyEnd, rows)
		}
		if entries != 0 {
			t.Fatalf("keyEnd %d: empty projection row carried %d entries, want 0", keyEnd, entries)
		}
	}
}

// emptySummaryProjectionKeyEnds are the coordinate universes the law ranges
// over. The empty projection is a property of the declared summary vector, not
// of the Factor's width, so a Factor still carrying coordinates answers it the
// same way a zero-width Factor does.
var emptySummaryProjectionKeyEnds = []uint64{0, 3}

// emptySummaryProjectionUnit is the Unit the declaration below seals. It is
// package state for the duration of one law case, because the two-phase Bind
// API hands the Unit to the declaration callback only.
var emptySummaryProjectionUnit carrier.Unit

func newEmptySummaryProjectionBinding(t testing.TB, keyEnd uint64) (*Binding[uint64, uint64], carrier.State, *carrier.Work) {
	t.Helper()
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("whole support")
	}
	emptySummaryProjectionUnit = carrier.Unit{}
	binding, ok := bindTest(testAlgebraInput[uint64, uint64]{
		KeyEnd:      keyEnd,
		Default:     0,
		AdmitAt:     func(_ uint64, _ uint64) bool { return true },
		Equal:       func(left, right uint64) bool { return left == right },
		Fingerprint: func(value uint64) uint64 { return value },
		Join:        maxUint64,
		Widen:       maxUint64,
		LessOrEq:    func(left, right uint64) bool { return left <= right },
		declare: func(binding *Binding[uint64, uint64]) bool {
			unit, declared := binding.DeclareSummary(nil)
			emptySummaryProjectionUnit = unit
			return declared
		},
	}, manager)
	if !ok {
		t.Fatalf("keyEnd %d: declare the empty summary projection", keyEnd)
	}
	composition, ok := attachTestComposition(t, []carrier.FactorOperation{binding})
	if !ok {
		t.Fatalf("keyEnd %d: composition", keyEnd)
	}
	state, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatalf("keyEnd %d: state", keyEnd)
	}
	return binding, state, newWork(t, composition)
}

func maxUint64(left, right uint64) uint64 {
	if left > right {
		return left
	}
	return right
}
