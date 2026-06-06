package transfer

import (
	canonicalplace "github.com/wippyai/go-lua/compiler/check/canonical/place"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
)

// PlaceMutationEffect is the canonical reductor for side facts whose truth is
// tied to a mutable Place. Product writes and table mutators both lower through
// this payload before installing their new value facts, so mutable-path
// invalidation is owned by transfer rather than by liveness, the driver, or
// ad-hoc per-assignment helpers.
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
	path, ok := place.StaticPrefixPath()
	if !ok || path.Symbol == 0 {
		return false
	}
	effect := flow.AddressWritePathInvalidation{
		WritePath:           path,
		PresentElementWrite: presentElementWrite && len(place.Steps) > 0,
		Written:             presentElementValue,
	}
	if presentElementWrite && len(place.Steps) > 0 {
		if arrayPath, member, ok := presentElementMemberWriteFootprint(place); ok {
			effect.PresentElementArrayPath = arrayPath
			effect.HasPresentElementArrayPath = true
			effect.PresentElementMember = member
		}
	}
	return flow.ApplyAddressWritePathInvalidation(out, effect)
}

func presentElementMemberWriteFootprint(place Place) (constraint.Path, []constraint.Segment, bool) {
	if place.Root == 0 {
		return constraint.Path{}, nil, false
	}
	array := constraint.NewPath(place.Root, place.RootName)
	for i, step := range place.Steps {
		if step.Kind == PlaceStepDynamicIndex {
			if i == len(place.Steps)-1 {
				return constraint.Path{}, nil, false
			}
			member, ok := staticMemberSuffix(place.Steps[i+1:])
			return array, member, ok
		}
		seg, ok := canonicalplace.SegmentFromStep(step)
		if !ok {
			return constraint.Path{}, nil, false
		}
		array.Segments = append(array.Segments, seg)
	}
	return constraint.Path{}, nil, false
}

func staticMemberSuffix(steps []PlaceStep) ([]constraint.Segment, bool) {
	if len(steps) == 0 {
		return nil, false
	}
	out := make([]constraint.Segment, 0, len(steps))
	for _, step := range steps {
		seg, ok := canonicalplace.SegmentFromStep(step)
		if !ok {
			return nil, false
		}
		out = append(out, seg)
	}
	return out, true
}
