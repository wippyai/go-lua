package callboundary

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/factset"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

type frozenTableFactKey pathdom.PathKey

// frozenTableLane is the canonical must (intersection) lattice for frozen-table
// facts: one fact per target, kept only when guaranteed on every joined path.
var frozenTableLane = factset.Set[frozenTableFactKey, FrozenTableFact]{
	Key:       func(f FrozenTableFact) frozenTableFactKey { return frozenTableFactKey(f.Target.Key()) },
	EqualFact: func(a, b FrozenTableFact) bool { return a.Target.Key() == b.Target.Key() },
	Less:      func(a, b FrozenTableFact) bool { return a.Target.Less(b.Target) },
	Valid:     func(f FrozenTableFact) bool { return f.Target.IsPlaceholder() },
	CloneFact: func(f FrozenTableFact) FrozenTableFact {
		f.Target = f.Target.Clone()
		return f
	},
	Prefer:    func(kept, incoming FrozenTableFact) bool { return true },
	Intersect: true,
}
