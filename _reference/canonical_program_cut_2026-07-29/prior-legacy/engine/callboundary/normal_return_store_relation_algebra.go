package callboundary

import (
	"github.com/wippyai/go-lua/__legacy/analysis/domain/lattice/factset"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

type storeRelationFactKey struct {
	source pathdom.PathKey
	into   pathdom.PathKey
}

// storeRelationLane is the canonical must (intersection) keyed-fact-set lattice
// for store relations: a relation is preserved only when guaranteed on every
// joined path.
var storeRelationLane = factset.Set[storeRelationFactKey, StoreRelationFact]{
	Key:       storeRelationKeyOf,
	EqualFact: func(a, b StoreRelationFact) bool { return storeRelationKeyOf(a) == storeRelationKeyOf(b) },
	Less: func(a, b StoreRelationFact) bool {
		if !a.Source.Equal(b.Source) {
			return a.Source.Less(b.Source)
		}
		return a.Into.Less(b.Into)
	},
	Valid: func(f StoreRelationFact) bool { return f.Source.IsPlaceholder() && f.Into.IsPlaceholder() },
	CloneFact: func(f StoreRelationFact) StoreRelationFact {
		f.Source = f.Source.Clone()
		f.Into = f.Into.Clone()
		return f
	},
	Prefer:    func(kept, incoming StoreRelationFact) bool { return true },
	Intersect: true,
}

func storeRelationKeyOf(fact StoreRelationFact) storeRelationFactKey {
	return storeRelationFactKey{source: fact.Source.Key(), into: fact.Into.Key()}
}
