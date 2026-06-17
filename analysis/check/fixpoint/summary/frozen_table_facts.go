package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/factset"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

type frozenTableFactKey pathdom.PathKey

// frozenTableLane is the canonical must (intersection) lattice for frozen-table
// facts: one fact per target, kept only when guaranteed on every joined path.
var frozenTableLane = factset.Set[frozenTableFactKey, callboundary.FrozenTableFact]{
	Key:       func(f callboundary.FrozenTableFact) frozenTableFactKey { return frozenTableFactKey(f.Target.Key()) },
	EqualFact: func(a, b callboundary.FrozenTableFact) bool { return a.Target.Key() == b.Target.Key() },
	Less:      func(a, b callboundary.FrozenTableFact) bool { return a.Target.Less(b.Target) },
	Valid:     func(f callboundary.FrozenTableFact) bool { return f.Target.IsPlaceholder() },
	CloneFact: func(f callboundary.FrozenTableFact) callboundary.FrozenTableFact {
		f.Target = f.Target.Clone()
		return f
	},
	Prefer:    func(kept, incoming callboundary.FrozenTableFact) bool { return true },
	Intersect: true,
}
