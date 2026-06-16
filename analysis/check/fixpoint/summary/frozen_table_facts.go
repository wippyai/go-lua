package summary

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

type frozenTableFactKey pathdom.PathKey

func normalizeFrozenTableFacts(in []callboundary.FrozenTableFact) []callboundary.FrozenTableFact {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[frozenTableFactKey]callboundary.FrozenTableFact, len(in))
	for _, fact := range in {
		if !fact.Target.IsPlaceholder() {
			continue
		}
		fact.Target = cloneSummaryPath(fact.Target)
		seen[frozenTableFactKey(fact.Target.Key())] = fact
	}
	return sortedFrozenTableFacts(seen)
}

func cloneFrozenTableFacts(in []callboundary.FrozenTableFact) []callboundary.FrozenTableFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.FrozenTableFact, len(in))
	for i, fact := range in {
		fact.Target = cloneSummaryPath(fact.Target)
		out[i] = fact
	}
	return out
}

func frozenTableFactsEqual(a, b []callboundary.FrozenTableFact) bool {
	a = normalizeFrozenTableFacts(a)
	b = normalizeFrozenTableFacts(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Target.Key() != b[i].Target.Key() {
			return false
		}
	}
	return true
}

func frozenTableFactsLessOrEq(a, b []callboundary.FrozenTableFact) bool {
	aSet := frozenTableFactsSet(a)
	for _, fact := range normalizeFrozenTableFacts(b) {
		if _, ok := aSet[frozenTableFactKey(fact.Target.Key())]; !ok {
			return false
		}
	}
	return true
}

func joinFrozenTableFacts(a, b []callboundary.FrozenTableFact) []callboundary.FrozenTableFact {
	aSet := frozenTableFactsSet(a)
	bSet := frozenTableFactsSet(b)
	if len(aSet) == 0 || len(bSet) == 0 {
		return nil
	}
	out := make(map[frozenTableFactKey]callboundary.FrozenTableFact)
	for key, fact := range aSet {
		if _, ok := bSet[key]; ok {
			out[key] = fact
		}
	}
	return sortedFrozenTableFacts(out)
}

func frozenTableFactsSet(in []callboundary.FrozenTableFact) map[frozenTableFactKey]callboundary.FrozenTableFact {
	out := normalizeFrozenTableFacts(in)
	if len(out) == 0 {
		return nil
	}
	m := make(map[frozenTableFactKey]callboundary.FrozenTableFact, len(out))
	for _, fact := range out {
		m[frozenTableFactKey(fact.Target.Key())] = fact
	}
	return m
}

func sortedFrozenTableFacts(in map[frozenTableFactKey]callboundary.FrozenTableFact) []callboundary.FrozenTableFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.FrozenTableFact, 0, len(in))
	for _, fact := range in {
		out = append(out, fact)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Target.Less(out[j].Target) })
	return out
}
