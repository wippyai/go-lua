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
	next := out.Cond.Forget(func(c constraint.Constraint) bool {
		return conditionConstraintAffectedByWrite(c, path)
	})
	if constraint.Domain.Equal(out.Cond, next) {
		return false
	}
	out.Cond = next
	return true
}

func conditionConstraintAffectedByWrite(c constraint.Constraint, writePath constraint.Path) bool {
	for _, p := range constraint.SemanticAffectedPaths(c) {
		if conditionPathAffectedByWrite(p, writePath) {
			return true
		}
	}
	return false
}

func conditionPathAffectedByWrite(path, writePath constraint.Path) bool {
	if path.Symbol == 0 || writePath.Symbol == 0 || path.Symbol != writePath.Symbol {
		return false
	}
	if len(writePath.Segments) > len(path.Segments) {
		return false
	}
	for i := range writePath.Segments {
		if !conditionSegmentsEqual(writePath.Segments[i], path.Segments[i]) {
			return false
		}
	}
	return true
}

func conditionSegmentsEqual(a, b constraint.Segment) bool {
	return a.Kind == b.Kind && a.Name == b.Name && a.Index == b.Index
}

func (t *Transfer) invalidateStaticMembersForPlace(out *flow.PointState, place Place) bool {
	if out == nil {
		return false
	}
	before := out.StaticMembers
	if path, ok := place.StaticPath(); ok && path.Symbol != 0 {
		for i := 1; i < len(path.Segments); i++ {
			prefix := constraint.Path{
				Root:     path.Root,
				Symbol:   path.Symbol,
				Version:  path.Version,
				Segments: append([]constraint.Segment(nil), path.Segments[:i]...),
			}
			if prefixAddr, ok := flow.StableAddressOfPath(prefix); ok {
				out.StaticMembers = out.StaticMembers.WithAddress(prefixAddr, product.Domain.Bottom())
			}
		}
		if addr, ok := flow.StableAddressOfPath(path); ok {
			out.StaticMembers = out.StaticMembers.KillSubtreeAddress(addr)
		}
		return !flow.StaticMemberFactsDomain.Equal(before, out.StaticMembers)
	}
	path, ok := place.StaticPrefixPath()
	if !ok || path.Symbol == 0 {
		return false
	}
	if addr, ok := flow.StableAddressOfPath(path); ok {
		out.StaticMembers = out.StaticMembers.KillSubtreeAddress(addr)
	}
	return !flow.StaticMemberFactsDomain.Equal(before, out.StaticMembers)
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
	addr, ok := flow.StableAddressOfPath(path)
	if !ok {
		return false
	}
	effect := flow.AddressWriteInvalidation{
		Write:               addr,
		PresentElementWrite: presentElementWrite && len(place.Steps) > 0,
		Written:             presentElementValue,
	}
	if presentElementWrite && len(place.Steps) > 0 {
		if arrayPath, member, ok := presentElementMemberWriteFootprint(place); ok {
			if arrayAddr, ok := flow.StableAddressOfPath(arrayPath); ok {
				effect.PresentElementArray = arrayAddr
				effect.HasPresentElementArray = true
				effect.PresentElementMember = member
			} else {
				effect.PresentElementMember = nil
			}
		}
	}
	return flow.ApplyAddressWriteInvalidation(out, effect)
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
