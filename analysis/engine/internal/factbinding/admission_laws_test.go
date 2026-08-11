package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// A direct fact becomes eligible for carrier's allocation-free same-root Join
// path as soon as its Patch accepts.  That path cannot repair a malformed
// Join(v, v), so ingress must reject the value before a candidate can publish.
// Widen is deliberately absent from this ordinary-write proof: recurrence is
// the only authority allowed to invoke it.
func TestDirectAdmissionRequiresJoinFixedPointWithoutWidening(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	widenCalls := 0
	config := lawInput(false)
	config.Join = func(left, right uint64) uint64 {
		if left == 1 && right == 1 {
			return 2 // deliberately violates Join idempotence at the staged value
		}
		if left > right {
			return left
		}
		return right
	}
	config.Widen = func(left, right uint64) uint64 {
		widenCalls++
		if left > right {
			return left
		}
		return right
	}
	binding, state, _, composition, fixture := bindingState(t, manager, config, whole)
	widenCalls = 0 // binding construction validates only the declared Default.

	patch := binding.Begin(newWork(t, composition), state)
	if patch == nil {
		t.Fatal("stage direct fact")
	}
	if patch.Write(fixture.target(t, 0, carrier.StrongTarget), whole, 1) {
		t.Fatal("direct non-idempotent Join value crossed admission")
	}
	if widenCalls != 0 {
		t.Fatalf("ordinary direct admission invoked Widen %d times", widenCalls)
	}
	if !patch.Discard() {
		t.Fatal("discard rejected unpublished candidate")
	}
}
