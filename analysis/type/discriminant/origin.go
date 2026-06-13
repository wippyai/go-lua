package discriminant

import (
	"sort"
	"sync"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

type originCase struct {
	index int
	typ   typ.Type
}

type originFamily struct {
	id    uint64
	cases []originCase
}

var (
	originCatalogMu sync.Mutex
	originCatalog   = make(map[uint64]originFamily)
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
		field, ok := fieldAtPath(c.typ, suffix, 0)
		if !ok {
			continue
		}
		childFamily, childCases, ok := OriginOfType(field)
		if !ok {
			continue
		}
		if outFamily == 0 {
			outFamily = childFamily
		}
		if outFamily != childFamily {
			return 0, nil, false
		}
		outCases = append(outCases, childCases...)
	}
	outCases = compactInts(outCases)
	if outFamily == 0 || len(outCases) == 0 {
		return 0, nil, false
	}
	return outFamily, outCases, true
}

func originByPathLiteral(t typ.Type, suffix []segment.Segment, lit typ.Type, negate bool) (uint64, []int, bool) {
	if t == nil || len(suffix) == 0 || lit == nil {
		return 0, nil, false
	}
	family, ok := originFamilyOf(t)
	if !ok {
		return 0, nil, false
	}
	out := make([]int, 0, len(family.cases))
	changed := false
	for _, c := range family.cases {
		matches := pathAdmitsLiteral(c.typ, suffix, lit, 0)
		if matches != negate {
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
		field, ok := fieldAtPath(c.typ, suffix, 0)
		if !ok {
			out = append(out, c.index)
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
	out = compactInts(out)
	if sameIntSet(parentCases, out) {
		return nil, false
	}
	return out, true
}

func originFamilyOf(t typ.Type) (originFamily, bool) {
	t = unwrap.Annotated(unwrap.NormalizeNil(t))
	switch v := t.(type) {
	case *typ.Alias:
		return originFamilyOf(v.UnaliasedTarget())
	case *typ.Optional:
		return originFamilyOf(v.Inner)
	case *typ.Instantiated:
		expanded, ok := expandInstantiated(v)
		if !ok {
			return originFamily{}, false
		}
		return originFamilyOf(expanded)
	case *typ.Union:
		return closedRecordUnionFamily(v)
	case *typ.Record:
		return taggedRecordFamily(v)
	default:
		return originFamily{}, false
	}
}

func closedRecordUnionFamily(u *typ.Union) (originFamily, bool) {
	if u == nil || len(u.Members) < 2 {
		return originFamily{}, false
	}
	records := make([]*typ.Record, 0, len(u.Members))
	for _, member := range u.Members {
		rec, ok := recordOf(member)
		if !ok || rec.Open || rec.HasMapComponent() {
			return originFamily{}, false
		}
		records = append(records, rec)
	}
	if !NewDetector().ClosedRecordSetConflicts(records) {
		return originFamily{}, false
	}
	id := internal.FnvString("discriminant.union.family")
	for _, member := range u.Members {
		id = internal.MixHash(id, typ.EqualityHash(member))
	}
	id = nonZeroHash(id)
	cases := make([]originCase, 0, len(u.Members))
	for i, member := range u.Members {
		cases = append(cases, originCase{index: i, typ: member})
	}
	family := originFamily{id: id, cases: cases}
	storeOriginFamily(family)
	return family, true
}

func taggedRecordFamily(r *typ.Record) (originFamily, bool) {
	if r == nil {
		return originFamily{}, false
	}
	tags := NewDetector().RequiredTags(r)
	if len(tags) == 0 {
		return originFamily{}, false
	}
	paths := make([]string, 0, len(tags))
	for path := range tags {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	familyID := internal.FnvString("discriminant.tag.family")
	caseHash := internal.FnvString("discriminant.tag.case")
	for _, path := range paths {
		familyID = internal.MixHash(familyID, internal.FnvString(path))
		caseHash = internal.MixHash(caseHash, internal.FnvString(path))
		caseHash = internal.MixHash(caseHash, tags[path])
	}
	familyID = nonZeroHash(familyID)
	caseIndex := hashCaseIndex(caseHash)
	family := originFamily{
		id: familyID,
		cases: []originCase{{
			index: caseIndex,
			typ:   r,
		}},
	}
	storeOriginFamily(family)
	return family, true
}

func recordOf(t typ.Type) (*typ.Record, bool) {
	switch v := unwrap.Annotated(unwrap.NormalizeNil(t)).(type) {
	case *typ.Alias:
		return recordOf(v.UnaliasedTarget())
	case *typ.Instantiated:
		expanded, ok := expandInstantiated(v)
		if !ok {
			return nil, false
		}
		return recordOf(expanded)
	case *typ.Record:
		return v, true
	default:
		return nil, false
	}
}

func storeOriginFamily(f originFamily) {
	if f.id == 0 || len(f.cases) == 0 {
		return
	}
	originCatalogMu.Lock()
	defer originCatalogMu.Unlock()
	if existing, ok := originCatalog[f.id]; ok {
		f.cases = mergeOriginCases(existing.cases, f.cases)
	}
	originCatalog[f.id] = f
}

func loadOriginFamily(id uint64) (originFamily, bool) {
	originCatalogMu.Lock()
	defer originCatalogMu.Unlock()
	family, ok := originCatalog[id]
	return family, ok
}

func mergeOriginCases(a, b []originCase) []originCase {
	byIndex := make(map[int]originCase, len(a)+len(b))
	for _, c := range a {
		byIndex[c.index] = c
	}
	for _, c := range b {
		byIndex[c.index] = c
	}
	indices := make([]int, 0, len(byIndex))
	for index := range byIndex {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	out := make([]originCase, 0, len(indices))
	for _, index := range indices {
		out = append(out, byIndex[index])
	}
	return out
}

func intSet(values []int) map[int]bool {
	out := make(map[int]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

func compactInts(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	sort.Ints(values)
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func casesIntersect(cases []int, constraint map[int]bool) bool {
	for _, c := range cases {
		if constraint[c] {
			return true
		}
	}
	return false
}

func sameIntSet(a, b []int) bool {
	a = compactInts(append([]int(nil), a...))
	b = compactInts(append([]int(nil), b...))
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hashCaseIndex(h uint64) int {
	maxInt := uint64(^uint(0) >> 1)
	out := int(h & maxInt)
	if out == 0 {
		return 1
	}
	return out
}

func nonZeroHash(h uint64) uint64 {
	if h == 0 {
		return 1
	}
	return h
}
