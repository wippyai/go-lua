package callboundary

import (
	"github.com/wippyai/go-lua/__legacy/analysis/domain/lattice/factset"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

type pathInvalidationFactKey pathdom.PathKey

// pathInvalidationLane is the canonical keyed-fact-set lattice for path
// invalidations: one fact per path, with an ancestor path subsuming its
// descendants.
var pathInvalidationLane = factset.Set[pathInvalidationFactKey, PathInvalidationFact]{
	Key: pathInvalidationKeyOf,
	EqualFact: func(a, b PathInvalidationFact) bool {
		return pathInvalidationKeyOf(a) == pathInvalidationKeyOf(b) &&
			a.PreserveStructuralWitness == b.PreserveStructuralWitness
	},
	Less:  func(a, b PathInvalidationFact) bool { return a.Path.Less(b.Path) },
	Valid: func(f PathInvalidationFact) bool { return !f.Path.IsEmpty() },
	CloneFact: func(f PathInvalidationFact) PathInvalidationFact {
		f.Path = f.Path.Clone()
		return f
	},
	Prefer: func(kept, incoming PathInvalidationFact) bool {
		return kept.PreserveStructuralWitness && !incoming.PreserveStructuralWitness
	},
	Dominates: func(super, sub PathInvalidationFact) bool {
		return sub.Path.HasPrefix(super.Path) &&
			(!super.PreserveStructuralWitness || sub.PreserveStructuralWitness)
	},
}

func pathInvalidationKeyOf(f PathInvalidationFact) pathInvalidationFactKey {
	return pathInvalidationFactKey(f.Path.Key())
}
