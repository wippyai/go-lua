package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestHistorySensitiveWidenThenNarrowTerminatesByRank uses one synthetic
// recurrence whose post-widen desired state is deliberately retained apart
// from the widened current state. Widen jumps from the initial history to a
// finite postfix; Narrow then descends one rank at a time while remaining
// above that independently recomputed desired state. There is no iteration
// budget: each strict transition is accepted only with a smaller rank.
func TestHistorySensitiveWidenThenNarrowTerminatesByRank(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	var widenCalls, narrowCalls int
	config := testAlgebraInput[uint64, uint64]{
		KeyEnd:      1,
		Default:     0,
		AdmitAt:     func(_ uint64, _ uint64) bool { return true },
		Equal:       func(left, right uint64) bool { return left == right },
		Fingerprint: func(value uint64) uint64 { return value },
		Join: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		// The result depends on the prior state: only an ascending first
		// visit jumps to the finite widened postfix.
		Widen: func(previous, desired uint64) uint64 {
			widenCalls++
			if previous < desired {
				return 4
			}
			return previous
		},
		// The desired value remains a lower bound throughout the decreasing
		// phase. One finite rank component permits exactly the descent below.
		Narrow: func(previous, desired uint64) uint64 {
			narrowCalls++
			if previous <= desired {
				return previous
			}
			return previous - 1
		},
		LessOrEq:   func(left, right uint64) bool { return left <= right },
		WidenRank:  Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }},
		NarrowRank: Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }},
	}
	binding, initial, slot, composition, fixture := bindingState(t, manager, config, whole)
	widenCalls, narrowCalls = 0, 0 // Construction proves only Default stability.
	widenScope := scopedWidenScope(t, composition, fixture, 0)
	narrowScope, ok := composition.SealNarrowing([]carrier.Target{fixture.target(t, 0, carrier.StrongTarget)})
	if !ok {
		t.Fatal("narrow scope")
	}
	valueAt := func(state carrier.State) uint64 {
		t.Helper()
		root, ok := state.HandleAt(slot)
		if !ok {
			t.Fatal("factor root")
		}
		value, present, valid := observedExactValue(binding, newWork(t, composition), root, fixture.unit(t, 0), whole, func(guard.Atom) bool { return false })
		if !valid || !present {
			t.Fatalf("exact value invalid/present = %t/%t", valid, present)
		}
		return value
	}

	// The desired state is a separate immutable result, never an alias of the
	// current widened state supplied to the recurrence operator.
	desired := writeState(t, newWork(t, composition), binding, fixture, initial, slot, whole, 1)
	work := newWork(t, composition)
	widened, _, ok := work.Merge3Under(carrier.Widen, initial, desired, widenScope)
	if !ok || valueAt(widened) != 4 {
		t.Fatalf("first widening did not reach synthetic postfix: ok=%t value=%d", ok, valueAt(widened))
	}
	postfix, _, ok := work.Merge3Under(carrier.Widen, widened, desired, widenScope)
	if !ok || !work.EqualUnder(postfix, widened) || widenCalls != 2 {
		t.Fatalf("widen calls=%d postfix=%t, want two calls and equality", widenCalls, work.EqualUnder(postfix, widened))
	}

	// Recompute the desired result after the widening phase from the immutable
	// predecessor. It must remain distinct from current; Narrow(current,
	// current) would erase every strict transition asserted below.
	recomputedDesired := writeState(t, newWork(t, composition), binding, fixture, initial, slot, whole, 1)
	if work.EqualUnder(widened, recomputedDesired) {
		t.Fatal("post-widen desired state collapsed into current")
	}
	current := widened
	strictDescents := 0
	for {
		before := valueAt(current)
		next, _, ok := work.Merge3Under(carrier.Narrow, current, recomputedDesired, narrowScope)
		if !ok {
			t.Fatal("narrowing rejected a ranked transition")
		}
		if work.EqualUnder(next, current) {
			if before != 1 || !work.EqualUnder(next, recomputedDesired) {
				t.Fatalf("narrowing stopped at %d instead of the valid post-fixpoint", before)
			}
			break
		}
		after := valueAt(next)
		if after >= before || after != before-1 || after < valueAt(recomputedDesired) {
			t.Fatalf("invalid narrowing transition %d -> %d", before, after)
		}
		strictDescents++
		current = next
	}
	if strictDescents != 3 || narrowCalls != strictDescents+1 {
		t.Fatalf("strict descents=%d narrow calls=%d, want 3 descents followed by equality", strictDescents, narrowCalls)
	}
}
