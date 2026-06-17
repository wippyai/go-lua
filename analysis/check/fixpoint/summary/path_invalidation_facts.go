package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/factset"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

type pathInvalidationFactKey pathdom.PathKey

// pathInvalidationLane is the canonical keyed-fact-set lattice for path
// invalidations: one fact per path, with an ancestor path subsuming its
// descendants.
var pathInvalidationLane = factset.Set[pathInvalidationFactKey, callboundary.PathInvalidationFact]{
	Key: func(f callboundary.PathInvalidationFact) pathInvalidationFactKey {
		return pathInvalidationFactKey(f.Path.Key())
	},
	EqualFact: func(a, b callboundary.PathInvalidationFact) bool { return a.Path.Key() == b.Path.Key() },
	Less:      func(a, b callboundary.PathInvalidationFact) bool { return a.Path.Less(b.Path) },
	Valid:     func(f callboundary.PathInvalidationFact) bool { return f.Path.IsPlaceholder() },
	CloneFact: func(f callboundary.PathInvalidationFact) callboundary.PathInvalidationFact {
		f.Path = f.Path.Clone()
		return f
	},
	Prefer:    func(kept, incoming callboundary.PathInvalidationFact) bool { return true },
	Dominates: func(super, sub callboundary.PathInvalidationFact) bool { return pathHasPrefix(sub.Path, super.Path) },
}

func normalizePathInvalidationFacts(in []callboundary.PathInvalidationFact) []callboundary.PathInvalidationFact {
	return pathInvalidationLane.Normalize(in)
}

func clonePathInvalidationFacts(in []callboundary.PathInvalidationFact) []callboundary.PathInvalidationFact {
	return pathInvalidationLane.Clone(in)
}

func pathInvalidationFactsEqual(a, b []callboundary.PathInvalidationFact) bool {
	return pathInvalidationLane.Equal(a, b)
}

func pathInvalidationFactsLessOrEq(a, b []callboundary.PathInvalidationFact) bool {
	return pathInvalidationLane.LessOrEq(a, b)
}

func joinPathInvalidationFacts(a, b []callboundary.PathInvalidationFact) []callboundary.PathInvalidationFact {
	return pathInvalidationLane.Join(a, b)
}

func widenPathInvalidationFacts(prev, next []callboundary.PathInvalidationFact) []callboundary.PathInvalidationFact {
	return pathInvalidationLane.Widen(prev, next)
}
