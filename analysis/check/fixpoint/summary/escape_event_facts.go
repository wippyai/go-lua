package summary

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

type escapeEventFactKey struct {
	target    pathdom.PathKey
	recursive bool
}

func normalizeEscapeEventFacts(in []callboundary.EscapeEventFact) []callboundary.EscapeEventFact {
	if len(in) == 0 {
		return nil
	}
	merged := make(map[escapeEventFactKey]callboundary.EscapeEventFact, len(in))
	for _, fact := range in {
		if !fact.Target.IsPlaceholder() || fact.Kind == callboundary.EscapeEventNone {
			continue
		}
		fact.Target = cloneSummaryPath(fact.Target)
		key := escapeEventKeyOf(fact)
		if existing, ok := merged[key]; ok && existing.Kind >= fact.Kind {
			continue
		}
		merged[key] = fact
	}
	return compressEscapeEventFacts(merged)
}

func cloneEscapeEventFacts(in []callboundary.EscapeEventFact) []callboundary.EscapeEventFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.EscapeEventFact, len(in))
	for i, fact := range in {
		fact.Target = cloneSummaryPath(fact.Target)
		out[i] = fact
	}
	return out
}

func escapeEventFactsEqual(a, b []callboundary.EscapeEventFact) bool {
	a = normalizeEscapeEventFacts(a)
	b = normalizeEscapeEventFacts(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if escapeEventKeyOf(a[i]) != escapeEventKeyOf(b[i]) || a[i].Kind != b[i].Kind {
			return false
		}
	}
	return true
}

func escapeEventFactsLessOrEq(a, b []callboundary.EscapeEventFact) bool {
	a = normalizeEscapeEventFacts(a)
	b = normalizeEscapeEventFacts(b)
	for _, left := range a {
		if !escapeEventDominatedByAny(left, b) {
			return false
		}
	}
	return true
}

func joinEscapeEventFacts(a, b []callboundary.EscapeEventFact) []callboundary.EscapeEventFact {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make([]callboundary.EscapeEventFact, 0, len(a)+len(b))
	out = append(out, cloneEscapeEventFacts(a)...)
	out = append(out, cloneEscapeEventFacts(b)...)
	return normalizeEscapeEventFacts(out)
}

func widenEscapeEventFacts(prev, next []callboundary.EscapeEventFact) []callboundary.EscapeEventFact {
	return joinEscapeEventFacts(prev, next)
}

func compressEscapeEventFacts(in map[escapeEventFactKey]callboundary.EscapeEventFact) []callboundary.EscapeEventFact {
	if len(in) == 0 {
		return nil
	}
	facts := sortedEscapeEventFacts(in)
	out := facts[:0]
	for _, fact := range facts {
		if escapeEventDominatedByAny(fact, out) {
			continue
		}
		write := 0
		for _, existing := range out {
			if escapeEventDominates(fact, existing) {
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

func escapeEventDominatedByAny(fact callboundary.EscapeEventFact, facts []callboundary.EscapeEventFact) bool {
	for _, existing := range facts {
		if escapeEventDominates(existing, fact) {
			return true
		}
	}
	return false
}

func escapeEventDominates(parent, child callboundary.EscapeEventFact) bool {
	if parent.Kind < child.Kind {
		return false
	}
	if parent.Recursive {
		return pathHasPrefix(child.Target, parent.Target)
	}
	return !child.Recursive && parent.Target.Equal(child.Target)
}

func pathHasPrefix(candidate, prefix pathdom.Path) bool {
	if candidate.Symbol != prefix.Symbol || candidate.Root != prefix.Root || candidate.Version != prefix.Version {
		return false
	}
	if len(prefix.Segments) > len(candidate.Segments) {
		return false
	}
	for i, seg := range prefix.Segments {
		other := candidate.Segments[i]
		if seg.Kind != other.Kind || seg.Name != other.Name || seg.Index != other.Index {
			return false
		}
	}
	return true
}

func escapeEventKeyOf(fact callboundary.EscapeEventFact) escapeEventFactKey {
	return escapeEventFactKey{target: fact.Target.Key(), recursive: fact.Recursive}
}

func sortedEscapeEventFacts(in map[escapeEventFactKey]callboundary.EscapeEventFact) []callboundary.EscapeEventFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.EscapeEventFact, 0, len(in))
	for _, fact := range in {
		out = append(out, fact)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Target.Equal(out[j].Target) {
			return out[i].Target.Less(out[j].Target)
		}
		if out[i].Recursive != out[j].Recursive {
			return !out[i].Recursive
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}
