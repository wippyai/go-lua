package variant

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// OriginOfType returns finite variant-origin evidence for structural record
// unions and literal-tagged records.
func OriginOfType(t typ.Type) (uint64, []int, bool) {
	family, ok := originFamilyOf(t)
	if !ok {
		return 0, nil, false
	}
	cases := make([]int, 0, len(family.cases))
	for _, c := range family.cases {
		cases = append(cases, c.index)
	}
	sort.Ints(cases)
	return family.id, cases, true
}

// NarrowByOrigin narrows t to the cases represented by origin evidence.
func NarrowByOrigin(t typ.Type, familyID uint64, cases []int) (typ.Type, bool) {
	if familyID == 0 || len(cases) == 0 {
		return t, false
	}
	family, ok := originFamilyOf(t)
	if !ok || family.id != familyID {
		return t, false
	}
	allowed := intSet(cases)
	var out []typ.Type
	changed := false
	for _, c := range family.cases {
		if allowed[c.index] {
			out = append(out, c.typ)
			continue
		}
		changed = true
	}
	if !changed {
		return t, false
	}
	if len(out) == 0 {
		return typ.Never, true
	}
	return normalize.UnionForEvidence(out...), true
}

// OriginByPathLiteral returns origin evidence for the cases whose path admits
// lit. The returned bool reports whether the origin was strictly narrowed.
func OriginByPathLiteral(t typ.Type, suffix []segment.Segment, lit typ.Type) (uint64, []int, bool) {
	return originByPathLiteral(t, suffix, lit, false)
}

// OriginByPathLiteralNot returns origin evidence for the cases whose path does
// not admit lit. The returned bool reports whether the origin was strictly
// narrowed.
func OriginByPathLiteralNot(t typ.Type, suffix []segment.Segment, lit typ.Type) (uint64, []int, bool) {
	return originByPathLiteral(t, suffix, lit, true)
}

// FullFamilyType reconstructs the complete structural union of every case
// registered for a variant-origin family, independent of any narrowing recorded
// on a specific value. It yields the broad declared shape a discriminated value
// originated from.
func FullFamilyType(familyID uint64) (typ.Type, bool) {
	if familyID == 0 {
		return nil, false
	}
	family, ok := loadOriginFamily(familyID)
	if !ok || len(family.cases) == 0 {
		return nil, false
	}
	out := make([]typ.Type, 0, len(family.cases))
	for _, c := range family.cases {
		out = append(out, c.typ)
	}
	return normalize.UnionForEvidence(out...), true
}

// TypeFromOrigin reconstructs the structural union represented by origin
// evidence previously registered from a source type.
func TypeFromOrigin(familyID uint64, cases []int) (typ.Type, bool) {
	if familyID == 0 || len(cases) == 0 {
		return nil, false
	}
	family, ok := loadOriginFamily(familyID)
	if !ok {
		return nil, false
	}
	allowed := intSet(cases)
	out := make([]typ.Type, 0, len(cases))
	for _, c := range family.cases {
		if allowed[c.index] {
			out = append(out, c.typ)
		}
	}
	if len(out) == 0 {
		return typ.Never, true
	}
	return normalize.UnionForEvidence(out...), true
}
