package summary

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

func normalizeNumFloorFacts(in []callboundary.NumFloorFact) []callboundary.NumFloorFact {
	if len(in) == 0 {
		return nil
	}
	byPath := make(map[pathdom.PathKey]callboundary.NumFloorFact, len(in))
	for _, fact := range in {
		if !fact.Path.IsPlaceholder() {
			continue
		}
		fact.Path = fact.Path.Clone()
		key := fact.Path.Key()
		if kept, ok := byPath[key]; ok && kept.Floor >= fact.Floor {
			continue
		}
		byPath[key] = fact
	}
	if len(byPath) == 0 {
		return nil
	}
	out := make([]callboundary.NumFloorFact, 0, len(byPath))
	for _, fact := range byPath {
		out = append(out, fact)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Path.Equal(out[j].Path) {
			return out[i].Path.Less(out[j].Path)
		}
		return out[i].Floor < out[j].Floor
	})
	return out
}

func cloneNumFloorFacts(in []callboundary.NumFloorFact) []callboundary.NumFloorFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.NumFloorFact, len(in))
	for i, fact := range in {
		out[i] = callboundary.NumFloorFact{
			Path:  fact.Path.Clone(),
			Floor: fact.Floor,
		}
	}
	return out
}

func numFloorFactsEqual(a, b []callboundary.NumFloorFact) bool {
	a = normalizeNumFloorFacts(a)
	b = normalizeNumFloorFacts(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Path.Equal(b[i].Path) || a[i].Floor != b[i].Floor {
			return false
		}
	}
	return true
}

func numFloorFactsLessOrEq(a, b []callboundary.NumFloorFact) bool {
	a = normalizeNumFloorFacts(a)
	b = normalizeNumFloorFacts(b)
	if len(b) == 0 {
		return true
	}
	floors := make(map[pathdom.PathKey]int64, len(a))
	for _, fact := range a {
		floors[fact.Path.Key()] = fact.Floor
	}
	for _, right := range b {
		left, ok := floors[right.Path.Key()]
		if !ok || left < right.Floor {
			return false
		}
	}
	return true
}

func joinNumFloorFacts(a, b []callboundary.NumFloorFact) []callboundary.NumFloorFact {
	a = normalizeNumFloorFacts(a)
	b = normalizeNumFloorFacts(b)
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	right := make(map[pathdom.PathKey]int64, len(b))
	for _, fact := range b {
		right[fact.Path.Key()] = fact.Floor
	}
	out := make([]callboundary.NumFloorFact, 0, len(a))
	for _, left := range a {
		floor, ok := right[left.Path.Key()]
		if !ok {
			continue
		}
		if left.Floor < floor {
			floor = left.Floor
		}
		out = append(out, callboundary.NumFloorFact{
			Path:  left.Path.Clone(),
			Floor: floor,
		})
	}
	return normalizeNumFloorFacts(out)
}
