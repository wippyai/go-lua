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

// TestDirectAdmissionRefusesAFactThatIsNotEqualToItself is the read boundary's
// guarantee, stated where it is owed: at the write.
//
// A value the Factor's own equality does not admit as equal to itself is not a
// fact. Every consumer that compares it - to the Factor default, to a
// predecessor, to another cell of the same vector - gets an answer that
// depends on nothing, so a coordinate holding one delivers evidence no reader
// can authenticate. It must therefore be impossible to store, and it must be
// impossible without any reader restating the check: a rule family reads what
// this boundary admitted, and a family that authenticated its own reads would
// be compensating for a boundary that let the value through.
//
// Same must not launder it. Same is a representation relation - it says two
// values are the same object, not that the Factor considers them equal - and a
// value whose Join fixed point holds only under Same is exactly the case a
// per-family gate used to catch.
func TestDirectAdmissionRefusesAFactThatIsNotEqualToItself(t *testing.T) {
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("support")
	}
	const unequal = 3
	config := lawInput(false)
	config.Equal = func(left, right uint64) bool { return left == right && left != unequal }
	config.Same = func(left, right uint64) bool { return left == right }
	binding, state, _, composition, fixture := bindingState(t, manager, config, whole)

	patch := binding.Begin(newWork(t, composition), state)
	if patch == nil {
		t.Fatal("stage direct fact")
	}
	if patch.Write(fixture.target(t, 0, carrier.StrongTarget), whole, unequal) {
		t.Fatal("a fact the Factor's equality refuses to call equal to itself crossed admission")
	}
	// A self-equal value of the same algebra still writes, so the refusal is
	// the value's own and not the Same relation being present.
	if !patch.Write(fixture.target(t, 1, carrier.StrongTarget), whole, 2) {
		t.Fatal("a self-equal fact was refused")
	}
	if !patch.Discard() {
		t.Fatal("discard rejected unpublished candidate")
	}
}
