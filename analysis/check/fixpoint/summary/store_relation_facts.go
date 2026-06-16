package summary

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

type storeRelationFactKey struct {
	source pathdom.PathKey
	into   pathdom.PathKey
}

func normalizeStoreRelationFacts(in []callboundary.StoreRelationFact) []callboundary.StoreRelationFact {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[storeRelationFactKey]callboundary.StoreRelationFact, len(in))
	for _, fact := range in {
		if !fact.Source.IsPlaceholder() || !fact.Into.IsPlaceholder() {
			continue
		}
		fact.Source = cloneSummaryPath(fact.Source)
		fact.Into = cloneSummaryPath(fact.Into)
		seen[storeRelationKeyOf(fact)] = fact
	}
	if len(seen) == 0 {
		return nil
	}
	return sortedStoreRelationFacts(seen)
}

func cloneStoreRelationFacts(in []callboundary.StoreRelationFact) []callboundary.StoreRelationFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.StoreRelationFact, len(in))
	for i, fact := range in {
		fact.Source = cloneSummaryPath(fact.Source)
		fact.Into = cloneSummaryPath(fact.Into)
		out[i] = fact
	}
	return out
}

func storeRelationFactsEqual(a, b []callboundary.StoreRelationFact) bool {
	a = normalizeStoreRelationFacts(a)
	b = normalizeStoreRelationFacts(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if storeRelationKeyOf(a[i]) != storeRelationKeyOf(b[i]) {
			return false
		}
	}
	return true
}

func storeRelationFactsLessOrEq(a, b []callboundary.StoreRelationFact) bool {
	left := storeRelationFactsSet(a)
	for key := range storeRelationFactsSet(b) {
		if _, ok := left[key]; !ok {
			return false
		}
	}
	return true
}

func joinStoreRelationFacts(a, b []callboundary.StoreRelationFact) []callboundary.StoreRelationFact {
	left := storeRelationFactsSet(a)
	right := storeRelationFactsSet(b)
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	out := make(map[storeRelationFactKey]callboundary.StoreRelationFact)
	for key, fact := range left {
		if _, ok := right[key]; ok {
			out[key] = fact
		}
	}
	return sortedStoreRelationFacts(out)
}

func widenStoreRelationFacts(prev, next []callboundary.StoreRelationFact) []callboundary.StoreRelationFact {
	return joinStoreRelationFacts(prev, next)
}

func storeRelationFactsSet(in []callboundary.StoreRelationFact) map[storeRelationFactKey]callboundary.StoreRelationFact {
	out := normalizeStoreRelationFacts(in)
	if len(out) == 0 {
		return nil
	}
	m := make(map[storeRelationFactKey]callboundary.StoreRelationFact, len(out))
	for _, fact := range out {
		m[storeRelationKeyOf(fact)] = fact
	}
	return m
}

func storeRelationKeyOf(fact callboundary.StoreRelationFact) storeRelationFactKey {
	return storeRelationFactKey{source: fact.Source.Key(), into: fact.Into.Key()}
}

func sortedStoreRelationFacts(in map[storeRelationFactKey]callboundary.StoreRelationFact) []callboundary.StoreRelationFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.StoreRelationFact, 0, len(in))
	for _, fact := range in {
		out = append(out, fact)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Source.Equal(out[j].Source) {
			return out[i].Source.Less(out[j].Source)
		}
		return out[i].Into.Less(out[j].Into)
	})
	return out
}
