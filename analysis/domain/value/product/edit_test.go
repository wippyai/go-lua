package product

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

// TestEditMatchesChainedSet asserts the batch editor produces a value
// byte-identical to the equivalent chain of Set calls across both axes.
func TestEditMatchesChainedSet(t *testing.T) {
	reg := mustRegistry(t, syntheticSpec().Erase(), secondSyntheticSpec().Erase())

	chained := Set(reg, Set(reg, Top(), syntheticKey, syntheticLow), secondSyntheticKey, syntheticHigh)

	ed := Edit(reg, Top())
	EditSet(&ed, syntheticKey, syntheticLow)
	EditSet(&ed, secondSyntheticKey, syntheticHigh)
	batched := ed.Done()

	if !Equal(reg, chained, batched) {
		t.Fatalf("Edit batch value %v != chained Set value %v", formatValue(batched), formatValue(chained))
	}
	if Hash(reg, chained) != Hash(reg, batched) {
		t.Fatalf("Edit batch hash %d != chained Set hash %d", Hash(reg, batched), Hash(reg, chained))
	}
	if chained.n != batched.n {
		t.Fatalf("Edit batch and chained Set must intern to the same node")
	}
}

// TestEditTopDropAndPresenceMatchSet covers the top-omission and presence-lane
// paths so the batch editor canonicalizes them identically to Set/WithPresence.
func TestEditTopDropAndPresenceMatchSet(t *testing.T) {
	reg := mustRegistry(t, syntheticSpec().Erase(), secondSyntheticSpec().Erase())

	base := Set(reg, Set(reg, Top(), syntheticKey, syntheticHigh), secondSyntheticKey, syntheticHigh)
	// Drop one axis back to top, keep the other, and shift presence.
	chained := WithPresence(reg, Set(reg, base, syntheticKey, syntheticTop), presence.Present())

	ed := Edit(reg, base)
	EditSet(&ed, syntheticKey, syntheticTop)
	ed.SetPresence(presence.Present())
	batched := ed.Done()

	if !Equal(reg, chained, batched) {
		t.Fatalf("Edit batch value %v != chained value %v", formatValue(batched), formatValue(chained))
	}
	if Hash(reg, chained) != Hash(reg, batched) {
		t.Fatalf("Edit batch hash %d != chained hash %d", Hash(reg, batched), Hash(reg, chained))
	}
	if chained.n != batched.n {
		t.Fatalf("Edit batch and chained must intern to the same node")
	}
}
