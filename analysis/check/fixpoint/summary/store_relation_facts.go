package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/factset"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

type storeRelationFactKey struct {
	source pathdom.PathKey
	into   pathdom.PathKey
}

// storeRelationLane is the canonical must (intersection) keyed-fact-set lattice
// for store relations: a relation is preserved only when guaranteed on every
// joined path.
var storeRelationLane = factset.Set[storeRelationFactKey, callboundary.StoreRelationFact]{
	Key:       storeRelationKeyOf,
	EqualFact: func(a, b callboundary.StoreRelationFact) bool { return storeRelationKeyOf(a) == storeRelationKeyOf(b) },
	Less: func(a, b callboundary.StoreRelationFact) bool {
		if !a.Source.Equal(b.Source) {
			return a.Source.Less(b.Source)
		}
		return a.Into.Less(b.Into)
	},
	Valid: func(f callboundary.StoreRelationFact) bool { return f.Source.IsPlaceholder() && f.Into.IsPlaceholder() },
	CloneFact: func(f callboundary.StoreRelationFact) callboundary.StoreRelationFact {
		f.Source = f.Source.Clone()
		f.Into = f.Into.Clone()
		return f
	},
	Prefer:    func(kept, incoming callboundary.StoreRelationFact) bool { return true },
	Intersect: true,
}

func storeRelationKeyOf(fact callboundary.StoreRelationFact) storeRelationFactKey {
	return storeRelationFactKey{source: fact.Source.Key(), into: fact.Into.Key()}
}
