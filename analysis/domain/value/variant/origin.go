package variant

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/variant/caseset"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// OriginCase describes one finite case in a variant-origin family.
type OriginCase struct {
	Index int
	Type  typ.Type
}

// OriginOfType returns finite variant-origin evidence for structural record
// unions and literal-tagged records.
func OriginOfType(t typ.Type) (uint64, []int, bool) {
	family, ok := originFamilyOf(t)
	if !ok {
		return 0, nil, false
	}
	return family.id, originFamilyCases(family), true
}

func originFamilyCases(family originFamily) []int {
	cases := make([]int, 0, len(family.cases))
	for _, c := range family.cases {
		cases = append(cases, c.index)
	}
	return compactInts(cases)
}

// OriginCasesOfType returns the full finite case set for t's variant-origin
// family. It is read-only diagnostic/query metadata; narrowing still flows
// through OriginOfType and NarrowByOrigin.
func OriginCasesOfType(t typ.Type) (uint64, []OriginCase, bool) {
	family, ok := originFamilyOf(t)
	if !ok {
		return 0, nil, false
	}
	return family.id, publicOriginCases(family.cases), true
}

// OriginCases returns the full finite case set for a registered family.
func OriginCases(familyID uint64) ([]OriginCase, bool) {
	if familyID == 0 {
		return nil, false
	}
	family, ok := loadOriginFamily(familyID)
	if !ok || len(family.cases) == 0 {
		return nil, false
	}
	return publicOriginCases(family.cases), true
}

// OriginCasesForType selects allowed family cases compatible with valueType.
func OriginCasesForType(familyID uint64, allowed caseset.View, valueType typ.Type) ([]int, bool) {
	if familyID == 0 || allowed.Len() == 0 || valueType == nil || typ.IsAny(valueType) || typ.IsUnknown(valueType) || typ.IsNever(valueType) {
		return nil, false
	}
	cases, ok := OriginCases(familyID)
	if !ok {
		return nil, false
	}
	selected := make([]int, 0, len(cases))
	for _, c := range cases {
		if !containsCase(allowed, c.Index) {
			continue
		}
		if subtype.IsSubtype(valueType, c.Type) || subtype.IsSubtype(c.Type, valueType) {
			selected = append(selected, c.Index)
		}
	}
	return selected, len(selected) != 0
}

// SingleCaseWithField returns the index of the only origin case whose type has
// field as a static field/member. It reports false when no case has the field or
// more than one case has it.
func SingleCaseWithField(cases []OriginCase, field string) (int, bool) {
	if field == "" {
		return 0, false
	}
	return SingleCaseWithPath(cases, []segment.Segment{{Kind: segment.SegmentField, Name: field}})
}

// SingleCaseWithPath returns the index of the only origin case whose type admits
// suffix as a static path. It reports false when no case admits the suffix or
// more than one case admits it.
func SingleCaseWithPath(cases []OriginCase, suffix []segment.Segment) (int, bool) {
	if len(cases) == 0 || len(suffix) == 0 {
		return 0, false
	}
	required := -1
	for _, c := range cases {
		if _, ok := FieldAtPath(c.Type, suffix); !ok {
			continue
		}
		if required >= 0 {
			return 0, false
		}
		required = c.Index
	}
	return required, required >= 0
}

func publicOriginCases(cases []originCase) []OriginCase {
	if len(cases) == 0 {
		return nil
	}
	out := make([]OriginCase, 0, len(cases))
	for _, c := range cases {
		out = append(out, OriginCase{Index: c.index, Type: c.typ})
	}
	return out
}

// NarrowByOrigin narrows t using an immutable canonical case view.
func NarrowByOrigin(t typ.Type, familyID uint64, cases caseset.View) (typ.Type, bool) {
	if familyID == 0 || cases.Len() == 0 {
		return t, false
	}
	family, ok := originFamilyOf(t)
	if !ok || family.id != familyID {
		return t, false
	}
	return narrowByOriginFamily(t, family, cases)
}

func narrowByOriginFamily(t typ.Type, family originFamily, cases caseset.View) (typ.Type, bool) {
	var out []typ.Type
	changed := false
	for _, c := range family.cases {
		if containsCase(cases, c.index) {
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

// TypeFromOrigin reconstructs a type from an immutable canonical case view.
func TypeFromOrigin(familyID uint64, cases caseset.View) (typ.Type, bool) {
	if familyID == 0 || cases.Len() == 0 {
		return nil, false
	}
	family, ok := loadOriginFamily(familyID)
	if !ok {
		return nil, false
	}
	return typeFromOriginFamily(family, cases)
}

func typeFromOriginFamily(family originFamily, cases caseset.View) (typ.Type, bool) {
	out := make([]typ.Type, 0, cases.Len())
	for _, c := range family.cases {
		if containsCase(cases, c.index) {
			out = append(out, c.typ)
		}
	}
	if !allCasesKnown(cases, family.cases) || len(out) == 0 {
		return nil, false
	}
	return normalize.UnionForEvidence(out...), true
}
