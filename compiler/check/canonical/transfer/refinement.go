package transfer

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

type RefinementKind uint8

const (
	RefinementSetValue RefinementKind = iota + 1
	RefinementPathCheck
	RefinementTypeCast
	RefinementLengthLowerBound
)

// RefinementEffect is the canonical product-state effect of learning a stronger
// fact about a Place. Unlike WriteEffect, it is meet-like: it refines Env/Cells
// and must not kill key-presence, static-member, reference, or relation facts
// that assignments invalidate.
type RefinementEffect struct {
	Place     Place
	Kind      RefinementKind
	Value     product.AbstractValue
	Check     cfg.CondCheckKind
	TypeName  string
	Target    typ.Type
	LengthMin int64
	PreferEnv bool
}

// StaticMemberRefinementEffect is the product-axis companion to a path
// RefinementEffect. It updates the read-only static-member cache when a guard
// proves a member path present or more precise. It is refinement, not assignment:
// failed/impossible refinements may kill the cached fact, but unrelated write
// invalidation stays owned by WriteEffect.
type StaticMemberRefinementEffect struct {
	Place    Place
	Base     product.AbstractValue
	HasBase  bool
	Check    cfg.CondCheckKind
	TypeName string
}

func (t *Transfer) applyRefinementEffect(out *flow.PointState, effect RefinementEffect) bool {
	if out == nil || effect.Place.Root == 0 {
		return false
	}
	switch effect.Kind {
	case RefinementSetValue:
		if effect.Value.IsZero() {
			return false
		}
		if product.Domain.Equal(effect.Value, product.Domain.Bottom()) {
			*out = flow.PointStateDomain.Bottom()
			return true
		}
		t.writeRefinedRoot(out, effect.Place.Root, effect.Value)
		return true
	case RefinementPathCheck:
		base, has := t.narrowBaseFor(*out, effect.Place.Root, effect.PreferEnv)
		if !has {
			return false
		}
		path, ok := effect.Place.StaticPath()
		if !ok {
			return false
		}
		narrowed, ok := narrowAtPath(base, path.Segments, effect.Check, effect.TypeName)
		if !ok {
			return false
		}
		if product.Domain.Equal(narrowed, product.Domain.Bottom()) {
			*out = flow.PointStateDomain.Bottom()
			return true
		}
		t.writeRefinedRoot(out, effect.Place.Root, narrowed)
		return true
	case RefinementTypeCast:
		if effect.Target == nil || typ.IsAbsentOrUnknown(effect.Target) {
			return false
		}
		path, ok := effect.Place.StaticPath()
		if !ok {
			return false
		}
		if len(path.Segments) == 0 {
			refined := product.FromType(effect.Target)
			if product.Domain.Equal(refined, product.Domain.Bottom()) {
				*out = flow.PointStateDomain.Bottom()
				return true
			}
			t.writeRefinedRoot(out, effect.Place.Root, refined)
			return true
		}
		base, has := t.narrowBaseFor(*out, effect.Place.Root, effect.PreferEnv)
		if !has {
			return false
		}
		refined := refineAtPath(base.ProjectValue(), path.Segments, func(typ.Type) typ.Type {
			return effect.Target
		})
		if refined == nil {
			return false
		}
		refinedValue := product.FromType(refined)
		if product.Domain.Equal(refinedValue, product.Domain.Bottom()) {
			*out = flow.PointStateDomain.Bottom()
			return true
		}
		t.writeRefinedRoot(out, effect.Place.Root, refinedValue)
		if pathKey, ok := symbolPathKey(effect.Place); ok {
			out.StaticMembers = out.StaticMembers.With(pathKey, product.FromType(effect.Target))
		}
		return true
	case RefinementLengthLowerBound:
		return t.applyLengthLowerBoundRefinement(out, effect.Place, effect.LengthMin, effect.PreferEnv)
	default:
		return false
	}
}

func (t *Transfer) writeRefinedRoot(out *flow.PointState, sym cfg.SymbolID, val product.AbstractValue) {
	t.writeSymbolValue(out, sym, val, false, false)
}

func (t *Transfer) applyLengthLowerBoundRefinement(out *flow.PointState, place Place, lower int64, preferEnv bool) bool {
	if out == nil || place.Root == 0 || lower <= 0 {
		return false
	}
	path, ok := place.StaticPath()
	if !ok || path.Symbol == 0 {
		return false
	}
	base, has := t.narrowBaseFor(*out, path.Symbol, preferEnv)
	if !has {
		return false
	}

	var refined product.AbstractValue
	if len(path.Segments) == 0 {
		refined = product.NarrowLengthLowerBound(base, lower)
	} else {
		refinedType := refineAtPath(base.ProjectValue(), path.Segments, func(slot typ.Type) typ.Type {
			return narrow.RefineByLengthLowerBound(slot, lower)
		})
		if refinedType == nil {
			return false
		}
		refined = product.FromType(refinedType)
	}
	if refined.IsZero() {
		return false
	}
	t.writeRefinedRoot(out, path.Symbol, refined)
	changed := true
	if len(path.Segments) > 0 {
		changed = t.refineStaticMemberFactForLengthLower(out, path, base, lower) || changed
	}
	return changed
}

func (t *Transfer) refineStaticMemberFactForLengthLower(out *flow.PointState, path constraint.Path, base product.AbstractValue, lower int64) bool {
	if out == nil || path.Symbol == 0 || len(path.Segments) == 0 || lower <= 0 {
		return false
	}
	pathKey := flow.SymbolPathKey(path.Symbol, path.Segments)
	source, has := out.StaticMembers.Value(pathKey)
	if !has && !base.IsZero() {
		source, has = productMemberPathValue(base, path.Segments)
	}
	if !has || source.IsZero() {
		return false
	}
	refined := product.NarrowLengthLowerBound(source, lower)
	if valueIsBottom(refined) {
		out.StaticMembers = out.StaticMembers.KillSubtree(pathKey)
		return true
	}
	if !refined.DefinitelyPresent() {
		return false
	}
	out.StaticMembers = out.StaticMembers.With(pathKey, refined)
	return true
}

func (t *Transfer) applyStaticMemberRefinementEffect(out *flow.PointState, effect StaticMemberRefinementEffect) bool {
	if out == nil {
		return false
	}
	path, ok := effect.Place.StaticPath()
	if !ok || path.Symbol == 0 || len(path.Segments) == 0 {
		return false
	}
	pathKey := flow.SymbolPathKey(path.Symbol, path.Segments)
	if existing, ok := out.StaticMembers.Value(pathKey); ok {
		refined, ok := refinedStaticMemberValue(existing, true, effect.Base, effect.HasBase, path.Segments, effect.Check, effect.TypeName)
		if !ok || refined.IsZero() || !refined.DefinitelyPresent() {
			out.StaticMembers = out.StaticMembers.KillSubtree(pathKey)
			return true
		}
		out.StaticMembers = out.StaticMembers.With(pathKey, refined)
		return true
	}
	if !staticMemberGuardImpliesPresence(effect.Check, effect.TypeName) {
		return false
	}
	leaf := product.PresentDynamic()
	if effect.HasBase {
		if read, ok := productMemberPathValue(effect.Base, path.Segments); ok && !read.IsZero() {
			leaf = read
		}
	}
	refined, ok := narrowValue(leaf, effect.Check, effect.TypeName)
	if !ok || refined.IsZero() || !refined.DefinitelyPresent() {
		return false
	}
	out.StaticMembers = out.StaticMembers.With(pathKey, refined)
	return true
}

func refinedStaticMemberValue(
	existing product.AbstractValue,
	hasExisting bool,
	base product.AbstractValue,
	hasBase bool,
	segments []constraint.Segment,
	check cfg.CondCheckKind,
	typeName string,
) (product.AbstractValue, bool) {
	var existingRefined product.AbstractValue
	existingOK := false
	if hasExisting && !existing.IsZero() {
		existingRefined, existingOK = narrowValue(existing, check, typeName)
		if existingOK && (existingRefined.IsZero() || !existingRefined.DefinitelyPresent()) {
			existingOK = false
		}
	}

	var baseRefined product.AbstractValue
	baseOK := false
	if hasBase {
		if read, ok := productMemberPathValue(base, segments); ok && !read.IsZero() {
			baseRefined, baseOK = narrowValue(read, check, typeName)
			if baseOK && (baseRefined.IsZero() || !baseRefined.DefinitelyPresent()) {
				baseOK = false
			}
		}
	}

	switch {
	case existingOK && baseOK:
		if baseRefined.Covers(existingRefined) {
			return existingRefined, true
		}
		if existingRefined.Covers(baseRefined) {
			return baseRefined, true
		}
		return baseRefined, true
	case baseOK:
		return baseRefined, true
	case existingOK:
		return existingRefined, true
	default:
		return product.AbstractValue{}, false
	}
}

func productMemberPathValue(base product.AbstractValue, segments []constraint.Segment) (product.AbstractValue, bool) {
	cur := base
	for _, seg := range segments {
		member, ok := value.MemberFromSegment(seg)
		if !ok {
			return product.AbstractValue{}, false
		}
		next, ok := product.MemberOf(cur, member)
		if !ok || next.IsZero() {
			return product.AbstractValue{}, false
		}
		cur = next
	}
	return cur, true
}
