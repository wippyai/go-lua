package callboundary

import (
	"github.com/wippyai/go-lua/__legacy/analysis/domain/lattice/factset"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

type escapeEventFactKey struct {
	target    pathdom.PathKey
	recursive bool
}

// escapeEventLane is the canonical keyed-fact-set lattice for escape events: one
// fact per (target, recursive) keeping the strongest kind, with recursive
// targets subsuming descendants under the same root.
var escapeEventLane = factset.Set[escapeEventFactKey, EscapeEventFact]{
	Key: escapeEventKeyOf,
	EqualFact: func(a, b EscapeEventFact) bool {
		return escapeEventKeyOf(a) == escapeEventKeyOf(b) && a.Kind == b.Kind
	},
	Less: escapeEventFactLess,
	Valid: func(f EscapeEventFact) bool {
		return f.Target.IsPlaceholder() && f.Kind != EscapeEventNone
	},
	CloneFact: func(f EscapeEventFact) EscapeEventFact {
		f.Target = f.Target.Clone()
		return f
	},
	Prefer:    func(kept, incoming EscapeEventFact) bool { return incoming.Kind > kept.Kind },
	Dominates: escapeEventDominates,
}

func escapeEventDominates(parent, child EscapeEventFact) bool {
	if parent.Kind < child.Kind {
		return false
	}
	if parent.Recursive {
		return child.Target.HasPrefix(parent.Target)
	}
	return !child.Recursive && parent.Target.Equal(child.Target)
}

func escapeEventKeyOf(fact EscapeEventFact) escapeEventFactKey {
	return escapeEventFactKey{target: fact.Target.Key(), recursive: fact.Recursive}
}

func escapeEventFactLess(a, b EscapeEventFact) bool {
	if !a.Target.Equal(b.Target) {
		return a.Target.Less(b.Target)
	}
	if a.Recursive != b.Recursive {
		return !a.Recursive
	}
	return a.Kind < b.Kind
}
