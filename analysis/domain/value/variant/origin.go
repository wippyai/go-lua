package variant

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/variant/caseset"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
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

// OriginCases returns the complete cases retained by this caller-owned cache.
func (c *Cache) OriginCases(familyID uint64) ([]OriginCase, bool) {
	if c == nil || familyID == 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	family, ok := c.loadOriginFamilyLocked(familyID)
	if !ok || len(family.cases) == 0 {
		return nil, false
	}
	return publicOriginCases(family.cases), true
}

// OriginCasesForType selects cases using this cache's family payload.
func (c *Cache) OriginCasesForType(familyID uint64, allowed caseset.View, valueType typ.Type) ([]int, bool) {
	if c == nil || familyID == 0 || allowed.Len() == 0 || valueType == nil || typ.IsAny(valueType) || typ.IsUnknown(valueType) || typ.IsNever(valueType) {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	family, ok := c.loadOriginFamilyLocked(familyID)
	if !ok {
		return nil, false
	}
	selected := make([]int, 0, len(family.cases))
	for _, item := range family.cases {
		if !containsCase(allowed, item.index) {
			continue
		}
		if subtype.IsSubtype(valueType, item.typ) || subtype.IsSubtype(item.typ, valueType) {
			selected = append(selected, item.index)
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
	if ok && family.id == familyID {
		return narrowByOriginFamily(t, family, cases)
	}
	// A witness may be a broad union while the origin token describes one
	// concrete tagged member (for example a declared Msg|Timer value carrying
	// Msg evidence). Search only the witness's own finite union; this derives
	// the payload from the caller's type graph and does not require an ID->type
	// process catalog.
	return narrowContainedOrigin(t, familyID, cases)
}

func narrowContainedOrigin(t typ.Type, familyID uint64, cases caseset.View) (typ.Type, bool) {
	switch value := unwrap.Annotated(t).(type) {
	case *typ.Alias:
		return narrowContainedOrigin(value.UnaliasedTarget(), familyID, cases)
	case *typ.Optional:
		return narrowContainedOrigin(value.Inner, familyID, cases)
	case *typ.Union:
		var members []typ.Type
		for _, member := range value.Members {
			family, ok := originFamilyOf(member)
			if !ok || family.id != familyID {
				continue
			}
			narrowed, _ := narrowByOriginFamily(member, family, cases)
			if narrowed != nil && !typ.IsNever(narrowed) {
				members = append(members, narrowed)
			}
		}
		if len(members) == 0 {
			return t, false
		}
		return normalize.UnionForEvidence(members...), true
	default:
		return t, false
	}
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

// FullFamilyType reconstructs a complete family from this cache's owner-local
// payload.
func (c *Cache) FullFamilyType(familyID uint64) (typ.Type, bool) {
	if c == nil || familyID == 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	family, ok := c.loadOriginFamilyLocked(familyID)
	if !ok || len(family.cases) == 0 {
		return nil, false
	}
	out := make([]typ.Type, 0, len(family.cases))
	for _, c := range family.cases {
		out = append(out, c.typ)
	}
	return normalize.UnionForEvidence(out...), true
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
