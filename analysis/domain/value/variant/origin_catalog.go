package variant

import (
	"sort"
	"sync"

	"github.com/wippyai/go-lua/analysis/type/typ"
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
