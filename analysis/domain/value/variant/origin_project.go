package variant

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ProjectOrigin projects origin evidence through a static record path.
func ProjectOrigin(familyID uint64, cases []int, suffix []segment.Segment) (uint64, []int, bool) {
	if familyID == 0 || len(cases) == 0 || len(suffix) == 0 {
		return 0, nil, false
	}
	family, ok := loadOriginFamily(familyID)
	if !ok {
		return 0, nil, false
	}
	selected := intSet(cases)
	var outFamily uint64
	var outCases []int
	for _, c := range family.cases {
		if !selected[c.index] {
			continue
		}
		delete(selected, c.index)
		field, ok := fieldAtPath(c.typ, suffix, 0)
		if !ok {
			return 0, nil, false
		}
		childFamily, childCases, ok := OriginOfType(field)
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
	if len(selected) != 0 {
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
			keep = armAdmitsTruthiness(c.typ, suffix, !negate, 0)
		} else if negate {
			keep = !pathForcesLiteral(c.typ, suffix, lit, 0)
		} else if pathAdmitsLiteral(c.typ, suffix, lit, 0) {
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

// NarrowOriginByPath keeps parent cases whose path projection is compatible
// with constraint. When equal is false it keeps the cases proven incompatible.
func NarrowOriginByPath(parentFamily uint64, parentCases []int, suffix []segment.Segment, constraintFamily uint64, constraintCases []int, equal bool) ([]int, bool) {
	if parentFamily == 0 || len(parentCases) == 0 || len(suffix) == 0 || constraintFamily == 0 || len(constraintCases) == 0 {
		return nil, false
	}
	family, ok := loadOriginFamily(parentFamily)
	if !ok {
		return nil, false
	}
	selected := intSet(parentCases)
	constraint := intSet(constraintCases)
	out := make([]int, 0, len(parentCases))
	for _, c := range family.cases {
		if !selected[c.index] {
			continue
		}
		delete(selected, c.index)
		field, ok := fieldAtPath(c.typ, suffix, 0)
		if !ok {
			if !equal {
				out = append(out, c.index)
			}
			continue
		}
		childFamily, childCases, ok := OriginOfType(field)
		if !ok || childFamily != constraintFamily {
			out = append(out, c.index)
			continue
		}
		intersects := casesIntersect(childCases, constraint)
		if intersects == equal {
			out = append(out, c.index)
		}
	}
	if len(selected) != 0 {
		return nil, false
	}
	out = compactInts(out)
	if sameIntSet(parentCases, out) {
		return nil, false
	}
	return out, true
}

// NarrowOriginByPathType keeps parent cases whose path projection is compatible
// with a concrete constraint type. It covers equality tests where one side is a
// projected union field and the other side is a concrete local value rather than
// another variant-origin path.
func NarrowOriginByPathType(parentFamily uint64, parentCases []int, suffix []segment.Segment, constraint typ.Type, equal bool) ([]int, bool) {
	if parentFamily == 0 || len(parentCases) == 0 || len(suffix) == 0 || constraint == nil {
		return nil, false
	}
	family, ok := loadOriginFamily(parentFamily)
	if !ok {
		return nil, false
	}
	selected := intSet(parentCases)
	out := make([]int, 0, len(parentCases))
	for _, c := range family.cases {
		if !selected[c.index] {
			continue
		}
		delete(selected, c.index)
		field, ok := fieldAtPath(c.typ, suffix, 0)
		if !ok {
			if !equal {
				out = append(out, c.index)
			}
			continue
		}
		compatible := typesOverlap(field, constraint)
		if compatible == equal {
			out = append(out, c.index)
		}
	}
	if len(selected) != 0 {
		return nil, false
	}
	out = compactInts(out)
	if sameIntSet(parentCases, out) {
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
