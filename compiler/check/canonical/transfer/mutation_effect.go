package transfer

import (
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
}

func presentDynamicElementWritePreservesKeyPresence(place Place, value product.AbstractValue) bool {
	if !value.DefinitelyPresent() || len(place.Steps) == 0 {
		return false
	}
	return place.Steps[len(place.Steps)-1].Kind == PlaceStepDynamicIndex
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
		changed = t.invalidateKeyFactsForPlace(out, effect.Place, effect.PresentElementKeyFacts) || changed
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
			out.StaticMembers = out.StaticMembers.With(flow.SymbolPathKey(prefix.Symbol, prefix.Segments), product.Domain.Bottom())
		}
		out.StaticMembers = out.StaticMembers.KillSubtree(flow.SymbolPathKey(path.Symbol, path.Segments))
		return !flow.StaticMemberFactsDomain.Equal(before, out.StaticMembers)
	}
	path, ok := place.StaticPrefixPath()
	if !ok || path.Symbol == 0 {
		return false
	}
	out.StaticMembers = out.StaticMembers.KillSubtree(flow.SymbolPathKey(path.Symbol, path.Segments))
	return !flow.StaticMemberFactsDomain.Equal(before, out.StaticMembers)
}

func (t *Transfer) invalidateKeyFactsForPlace(out *flow.PointState, place Place, presentElementWrite bool) bool {
	if out == nil {
		return false
	}
	path, ok := place.StaticPrefixPath()
	if !ok || path.Symbol == 0 {
		return false
	}
	beforeKeyPresence := out.KeyPresence
	beforeValueOrigins := out.ValueOrigins
	beforeIndexWrites := out.IndexWrites
	pathKey := flow.KeyPresencePathKey(path)
	if presentElementWrite && len(place.Steps) > 0 {
		out.KeyPresence = out.KeyPresence.KillAffectedByPresentElementWrite(pathKey)
	} else {
		out.KeyPresence = out.KeyPresence.KillAffectedByWrite(pathKey)
	}
	out.ValueOrigins = out.ValueOrigins.KillAffectedByWrite(pathKey)
	out.IndexWrites = out.IndexWrites.KillAffectedByWrite(pathKey)
	return !flow.KeyPresenceFactsDomain.Equal(beforeKeyPresence, out.KeyPresence) ||
		!flow.ValueOriginFactsDomain.Equal(beforeValueOrigins, out.ValueOrigins) ||
		!flow.IndexWriteAdmissionFactsDomain.Equal(beforeIndexWrites, out.IndexWrites)
}
