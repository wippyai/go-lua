package variant

import (
	"sort"
	"sync"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

type originFamilyKind uint8

const (
	originFamilyKindClosedRecordUnion originFamilyKind = iota + 1
	originFamilyKindTaggedRecord
)

type originCase struct {
	index int
	typ   typ.Type
}

type originFamily struct {
	id        uint64
	kind      originFamilyKind
	signature string
	cases     []originCase
}

var (
	originCatalogMu       sync.Mutex
	originCatalog         = make(map[uint64]originFamily)
	originCatalogRevision = make(map[uint64]uint64)
	originCatalogPoisoned = make(map[uint64]struct{})
)

func storeOriginFamily(f originFamily) bool {
	if f.id == 0 || f.kind == 0 || len(f.cases) == 0 {
		return false
	}
	originCatalogMu.Lock()
	defer originCatalogMu.Unlock()
	if _, poisoned := originCatalogPoisoned[f.id]; poisoned {
		return false
	}
	if existing, ok := originCatalog[f.id]; ok {
		if !originFamiliesCompatible(existing, f) {
			delete(originCatalog, f.id)
			originCatalogPoisoned[f.id] = struct{}{}
			originCatalogRevision[f.id]++
			return false
		}
		f.cases = mergeOriginCases(existing.cases, f.cases)
	}
	if existing, ok := originCatalog[f.id]; ok && originFamiliesEqual(existing, f) {
		return true
	}
	originCatalogRevision[f.id]++
	originCatalog[f.id] = f
	return true
}

func loadOriginFamily(id uint64) (originFamily, bool) {
	originCatalogMu.Lock()
	defer originCatalogMu.Unlock()
	if _, poisoned := originCatalogPoisoned[id]; poisoned {
		return originFamily{}, false
	}
	family, ok := originCatalog[id]
	return family, ok
}

func originFamilyRevision(id uint64) (uint64, bool) {
	originCatalogMu.Lock()
	defer originCatalogMu.Unlock()
	if _, poisoned := originCatalogPoisoned[id]; poisoned {
		return 0, false
	}
	if _, ok := originCatalog[id]; !ok {
		return 0, false
	}
	return originCatalogRevision[id], true
}

func originFamiliesCompatible(existing, next originFamily) bool {
	if existing.id != next.id || existing.kind != next.kind || existing.signature != next.signature {
		return false
	}
	switch existing.kind {
	case originFamilyKindClosedRecordUnion:
		return originCasesEqual(existing.cases, next.cases)
	case originFamilyKindTaggedRecord:
		return true
	default:
		return false
	}
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

func originFamiliesEqual(a, b originFamily) bool {
	return a.id == b.id &&
		a.kind == b.kind &&
		a.signature == b.signature &&
		originCasesEqual(a.cases, b.cases)
}

func originCasesEqual(a, b []originCase) bool {
	if len(a) != len(b) {
		return false
	}
	byIndex := make(map[int]typ.Type, len(a))
	for _, c := range a {
		byIndex[c.index] = c.typ
	}
	for _, c := range b {
		existing, ok := byIndex[c.index]
		if !ok || !typ.TypeEquals(existing, c.typ) {
			return false
		}
	}
	return true
}
