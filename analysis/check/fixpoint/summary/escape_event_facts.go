package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/factset"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

type escapeEventFactKey struct {
	target    pathdom.PathKey
	recursive bool
}

// escapeEventLane is the canonical keyed-fact-set lattice for escape events: one
// fact per (target, recursive) keeping the strongest kind, with recursive
// targets subsuming descendants under the same root.
var escapeEventLane = factset.Set[escapeEventFactKey, callboundary.EscapeEventFact]{
	Key:       escapeEventKeyOf,
	EqualFact: func(a, b callboundary.EscapeEventFact) bool { return escapeEventKeyOf(a) == escapeEventKeyOf(b) && a.Kind == b.Kind },
	Less:      escapeEventFactLess,
	Valid:     func(f callboundary.EscapeEventFact) bool { return f.Target.IsPlaceholder() && f.Kind != callboundary.EscapeEventNone },
	CloneFact: func(f callboundary.EscapeEventFact) callboundary.EscapeEventFact { f.Target = f.Target.Clone(); return f },
	Prefer:    func(kept, incoming callboundary.EscapeEventFact) bool { return incoming.Kind > kept.Kind },
	Dominates: escapeEventDominates,
}

func normalizeEscapeEventFacts(in []callboundary.EscapeEventFact) []callboundary.EscapeEventFact {
	return escapeEventLane.Normalize(in)
}

func cloneEscapeEventFacts(in []callboundary.EscapeEventFact) []callboundary.EscapeEventFact {
	return escapeEventLane.Clone(in)
}

func escapeEventFactsEqual(a, b []callboundary.EscapeEventFact) bool {
	return escapeEventLane.Equal(a, b)
}

func escapeEventFactsLessOrEq(a, b []callboundary.EscapeEventFact) bool {
	return escapeEventLane.LessOrEq(a, b)
}

func joinEscapeEventFacts(a, b []callboundary.EscapeEventFact) []callboundary.EscapeEventFact {
	return escapeEventLane.Join(a, b)
}

func widenEscapeEventFacts(prev, next []callboundary.EscapeEventFact) []callboundary.EscapeEventFact {
	return escapeEventLane.Widen(prev, next)
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

func escapeEventFactLess(a, b callboundary.EscapeEventFact) bool {
	if !a.Target.Equal(b.Target) {
		return a.Target.Less(b.Target)
	}
	if a.Recursive != b.Recursive {
		return !a.Recursive
	}
	return a.Kind < b.Kind
}
