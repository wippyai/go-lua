package transfer

import (
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
)

// PlaceMutationEffect is the canonical reductor for side facts whose truth is
// tied to a mutable Place. Product writes and table mutators both lower through
// this payload before installing their new value facts. Transfer selects which
// fact families are invalidated; canonical access/flow layers own the write
// footprint and address-fact laws.
type PlaceMutationEffect struct {
	Place                  Place
	StaticMembers          bool
	Conditions             bool
	KeyFacts               bool
	PresentElementKeyFacts bool
	PresentElementValue    product.AbstractValue
}

func presentDynamicElementWritePreservesKeyPresence(place Place, value product.AbstractValue) bool {
	if len(place.Steps) == 0 {
		return false
	}
	for i, step := range place.Steps {
		if step.Kind != PlaceStepDynamicIndex {
			continue
		}
		if i == len(place.Steps)-1 {
			return value.DefinitelyPresent()
		}
		return true
	}
	return false
}

func (t *Transfer) applyPlaceMutationEffect(out *flow.PointState, effect PlaceMutationEffect) bool {
	if out == nil || effect.Place.Root == 0 {
		return false
	}
	footprint, ok := effect.Place.WriteFootprint(effect.PresentElementKeyFacts, effect.PresentElementValue)
	if !ok {
		return false
	}
	return flow.ApplyAccessMutation(out, flow.AccessMutation{
		Footprint:     footprint,
		StaticMembers: effect.StaticMembers,
		Conditions:    effect.Conditions,
		AddressFacts:  effect.KeyFacts,
	})
}
