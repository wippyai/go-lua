package summary

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

type pathInvalidationFactKey pathdom.PathKey

func normalizePathInvalidationFacts(in []callboundary.PathInvalidationFact) []callboundary.PathInvalidationFact {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[pathInvalidationFactKey]callboundary.PathInvalidationFact, len(in))
	for _, fact := range in {
		if !fact.Path.IsPlaceholder() {
			continue
		}
		fact.Path = cloneSummaryPath(fact.Path)
		seen[pathInvalidationFactKey(fact.Path.Key())] = fact
	}
	if len(seen) == 0 {
		return nil
	}
	facts := sortedPathInvalidationFacts(seen)
	out := facts[:0]
	for _, fact := range facts {
		if pathInvalidationDominatedByAny(fact, out) {
			continue
		}
		write := 0
		for _, existing := range out {
			if pathHasPrefix(existing.Path, fact.Path) {
				continue
			}
			out[write] = existing
			write++
		}
		out = out[:write]
		out = append(out, fact)
	}
	return out
}

func clonePathInvalidationFacts(in []callboundary.PathInvalidationFact) []callboundary.PathInvalidationFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.PathInvalidationFact, len(in))
	for i, fact := range in {
		fact.Path = cloneSummaryPath(fact.Path)
		out[i] = fact
	}
	return out
}

func pathInvalidationFactsEqual(a, b []callboundary.PathInvalidationFact) bool {
	a = normalizePathInvalidationFacts(a)
	b = normalizePathInvalidationFacts(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Path.Key() != b[i].Path.Key() {
			return false
		}
	}
	return true
}

func pathInvalidationFactsLessOrEq(a, b []callboundary.PathInvalidationFact) bool {
	a = normalizePathInvalidationFacts(a)
	b = normalizePathInvalidationFacts(b)
	for _, left := range a {
		if !pathInvalidationDominatedByAny(left, b) {
			return false
		}
	}
	return true
}

func joinPathInvalidationFacts(a, b []callboundary.PathInvalidationFact) []callboundary.PathInvalidationFact {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make([]callboundary.PathInvalidationFact, 0, len(a)+len(b))
	out = append(out, clonePathInvalidationFacts(a)...)
	out = append(out, clonePathInvalidationFacts(b)...)
	return normalizePathInvalidationFacts(out)
}

func widenPathInvalidationFacts(prev, next []callboundary.PathInvalidationFact) []callboundary.PathInvalidationFact {
	return joinPathInvalidationFacts(prev, next)
}

func pathInvalidationDominatedByAny(fact callboundary.PathInvalidationFact, facts []callboundary.PathInvalidationFact) bool {
	for _, existing := range facts {
		if pathHasPrefix(fact.Path, existing.Path) {
			return true
		}
	}
	return false
}

func sortedPathInvalidationFacts(in map[pathInvalidationFactKey]callboundary.PathInvalidationFact) []callboundary.PathInvalidationFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.PathInvalidationFact, 0, len(in))
	for _, fact := range in {
		out = append(out, fact)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path.Less(out[j].Path) })
	return out
}
