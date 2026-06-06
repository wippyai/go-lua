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
	changed := false
	if effect.StaticMembers {
		changed = t.invalidateStaticMembersForPlace(out, effect.Place) || changed
	}
	if effect.Conditions {
		changed = t.invalidateConditionsForPlace(out, effect.Place) || changed
	}
	if effect.KeyFacts {
		changed = t.invalidateKeyFactsForPlaceWithValue(out, effect.Place, effect.PresentElementKeyFacts, effect.PresentElementValue) || changed
	}
	return changed
}

func (t *Transfer) invalidateConditionsForPlace(out *flow.PointState, place Place) bool {
	if out == nil || out.Cond.IsFalse() || out.Cond.IsTrue() || place.Root == 0 {
		return false
	}
	path, ok := place.StaticPrefixPath()
	if !ok || path.Symbol == 0 {
		return false
	}
	return flow.ForgetConditionAffectedByWrite(out, path)
}

func (t *Transfer) invalidateStaticMembersForPlace(out *flow.PointState, place Place) bool {
	if out == nil {
		return false
	}
	if path, ok := place.StaticPath(); ok && path.Symbol != 0 {
		return flow.InvalidateStaticMemberWritePath(out, path)
	}
	path, ok := place.StaticPrefixPath()
	if !ok || path.Symbol == 0 {
		return false
	}
	return flow.KillStaticMemberSubtreePath(out, path)
}

func (t *Transfer) invalidateKeyFactsForPlace(out *flow.PointState, place Place, presentElementWrite bool) bool {
	return t.invalidateKeyFactsForPlaceWithValue(out, place, presentElementWrite, product.AbstractValue{})
}

func (t *Transfer) invalidateKeyFactsForPlaceWithValue(
	out *flow.PointState,
	place Place,
	presentElementWrite bool,
	presentElementValue product.AbstractValue,
) bool {
	if out == nil {
		return false
	}
	footprint, ok := place.WriteFootprint(presentElementWrite, presentElementValue)
	if !ok {
		return false
	}
	return flow.ApplyAccessWriteFootprint(out, footprint)
}
