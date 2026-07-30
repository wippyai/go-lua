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
		if originFamilyCovers(existing, f) {
			return true
		}
		if !originFamiliesCompatible(existing, f) {
			delete(originCatalog, f.id)
			originCatalogPoisoned[f.id] = struct{}{}
			originCatalogRevision[f.id]++
			return false
		}
		f.cases = mergeOriginCases(existing.cases, f.cases)
	} else {
		f = cloneOriginFamily(f)
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
	if ok {
		family = cloneOriginFamily(family)
	}
	return family, ok
}

func cloneOriginFamily(f originFamily) originFamily {
	f.cases = cloneOriginCases(f.cases)
	return f
}

func cloneOriginCases(cases []originCase) []originCase {
	if len(cases) == 0 {
		return nil
	}
	out := make([]originCase, len(cases))
	copy(out, cases)
	return out
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
		return originCasesOverlapCompatible(existing.cases, next.cases)
	default:
		return false
	}
}

func originFamilyCovers(existing, next originFamily) bool {
	return existing.id == next.id &&
		existing.kind == next.kind &&
		existing.signature == next.signature &&
		originCasesCover(existing.cases, next.cases)
}

func originCasesOverlapCompatible(a, b []originCase) bool {
	for i, left := range a {
		for j := i + 1; j < len(a); j++ {
			right := a[j]
			if left.index == right.index && !typ.TypeEquals(left.typ, right.typ) {
				return false
			}
		}
		for _, right := range b {
			if left.index == right.index && !typ.TypeEquals(left.typ, right.typ) {
				return false
			}
		}
	}
	for i, left := range b {
		for j := i + 1; j < len(b); j++ {
			right := b[j]
			if left.index == right.index && !typ.TypeEquals(left.typ, right.typ) {
				return false
			}
		}
	}
	return true
}

func originCasesCover(haystack, needles []originCase) bool {
	if len(needles) == 0 {
		return true
	}
	if len(haystack) < len(needles) {
		return false
	}
	for _, needle := range needles {
		found := false
		for _, existing := range haystack {
			if existing.index == needle.index && typ.TypeEquals(existing.typ, needle.typ) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func mergeOriginCases(a, b []originCase) []originCase {
	out := make([]originCase, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].index < out[j].index
	})
	n := 0
	for _, c := range out {
		if n > 0 && out[n-1].index == c.index {
			out[n-1] = c
			continue
		}
		out[n] = c
		n++
	}
	return out[:n]
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
	return originCasesCover(a, b) && originCasesCover(b, a)
}
