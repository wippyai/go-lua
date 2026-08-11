package variant

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/variant/caseset"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ProjectOrigin projects origin evidence through this cache's owner-local
// family payload.
func (cache *Cache) ProjectOrigin(familyID uint64, cases caseset.View, suffix []segment.Segment) (uint64, []int, bool) {
	if cache == nil || familyID == 0 || cases.Len() == 0 || len(suffix) == 0 {
		return 0, nil, false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	family, ok := cache.loadOriginFamilyLocked(familyID)
	if !ok {
		return 0, nil, false
	}
	var outFamily uint64
	var outCases []int
	for _, item := range family.cases {
		if !containsCase(cases, item.index) {
			continue
		}
		field, ok := fieldAtPath(item.typ, suffix)
		if !ok {
			return 0, nil, false
		}
		childFamily, childCases, ok := cache.originOfTypeLocked(field)
		if !ok {
			return 0, nil, false
		}
		if outFamily == 0 {
			outFamily = childFamily
		}
		if outFamily != childFamily {
			return 0, nil, false
		}
		outCases = append(outCases, childCases...)
	}
	if !allCasesKnown(cases, family.cases) {
		return 0, nil, false
	}
	outCases = compactInts(outCases)
	if outFamily == 0 || len(outCases) == 0 {
		return 0, nil, false
	}
	return outFamily, outCases, true
}

func originByPathLiteral(t typ.Type, suffix []segment.Segment, lit typ.Type, negate bool) (uint64, []int, bool) {
	return originByPathLiteralWithCache(nil, t, suffix, lit, negate)
}

func originByPathLiteralWithCache(cache *Cache, t typ.Type, suffix []segment.Segment, lit typ.Type, negate bool) (uint64, []int, bool) {
	if t == nil || len(suffix) == 0 || lit == nil {
		return 0, nil, false
	}
	var family originFamily
	var ok bool
	if cache != nil {
		family, ok = cache.originFamilyOf(t)
	} else {
		family, ok = originFamilyOf(t)
	}
	if !ok {
		return 0, nil, false
	}
	truthyGuard := isTruthySentinel(lit)
	out := make([]int, 0, len(family.cases))
	changed := false
	for _, c := range family.cases {
		var keep bool
		if truthyGuard {
			// A truthy guard keeps the arms whose member can hold the requested
			// truthiness directly; the presence and truthiness axes are not
			// symmetric the way literal admission is, so each edge consults its
			// own predicate rather than negating the other.
			keep = armAdmitsTruthiness(c.typ, suffix, !negate)
		} else if negate {
			keep = !pathForcesLiteral(c.typ, suffix, lit)
		} else if pathAdmitsLiteral(c.typ, suffix, lit) {
			keep = true
		}
		if keep {
			out = append(out, c.index)
			continue
		}
		changed = true
	}
	out = compactInts(out)
	if !changed || len(out) == 0 {
		return 0, nil, false
	}
	return family.id, out, true
}

// NarrowOriginByPath narrows a parent family using this cache's owner-local
// payload. When equal is false it keeps the cases proven incompatible.
func (cache *Cache) NarrowOriginByPath(parentFamily uint64, parentCases caseset.View, suffix []segment.Segment, constraintFamily uint64, constraintCases caseset.View, equal bool) ([]int, bool) {
	if cache == nil || parentFamily == 0 || parentCases.Len() == 0 || len(suffix) == 0 || constraintFamily == 0 || constraintCases.Len() == 0 {
		return nil, false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	family, ok := cache.loadOriginFamilyLocked(parentFamily)
	if !ok {
		return nil, false
	}
	out := make([]int, 0, parentCases.Len())
	for _, item := range family.cases {
		if !containsCase(parentCases, item.index) {
			continue
		}
		field, ok := fieldAtPath(item.typ, suffix)
		if !ok {
			if !equal {
				out = append(out, item.index)
			}
			continue
		}
		childFamily, childCases, ok := cache.originOfTypeLocked(field)
		if !ok || childFamily != constraintFamily {
			out = append(out, item.index)
			continue
		}
		intersects := casesIntersect(constraintCases, childCases)
		if intersects == equal {
			out = append(out, item.index)
		}
	}
	if !allCasesKnown(parentCases, family.cases) {
		return nil, false
	}
	out = compactInts(out)
	if sameCases(parentCases, out) {
		return nil, false
	}
	return out, true
}

// NarrowOriginByPathType narrows a parent family against a type using this
// cache's owner-local payload. It covers equality tests where one side is a
// projected union field and the other side is a concrete local value rather than
// another variant-origin path.
func (cache *Cache) NarrowOriginByPathType(parentFamily uint64, parentCases caseset.View, suffix []segment.Segment, constraint typ.Type, equal bool) ([]int, bool) {
	if cache == nil || parentFamily == 0 || parentCases.Len() == 0 || len(suffix) == 0 || constraint == nil {
		return nil, false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	family, ok := cache.loadOriginFamilyLocked(parentFamily)
	if !ok {
		return nil, false
	}
	out := make([]int, 0, parentCases.Len())
	for _, item := range family.cases {
		if !containsCase(parentCases, item.index) {
			continue
		}
		field, ok := fieldAtPath(item.typ, suffix)
		if !ok {
			if !equal {
				out = append(out, item.index)
			}
			continue
		}
		compatible := typesOverlap(field, constraint)
		if compatible == equal {
			out = append(out, item.index)
		}
	}
	if !allCasesKnown(parentCases, family.cases) {
		return nil, false
	}
	out = compactInts(out)
	if sameCases(parentCases, out) {
		return nil, false
	}
	return out, true
}

func typesOverlap(left, right typ.Type) bool {
	if left == nil || right == nil {
		return false
	}
	return subtype.IsSubtype(left, right) || subtype.IsSubtype(right, left)
}
