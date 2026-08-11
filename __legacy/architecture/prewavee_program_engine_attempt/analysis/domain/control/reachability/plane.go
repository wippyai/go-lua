package reachability

import "github.com/wippyai/go-lua/analysis/engine"

// PlaneConfig is reachability's complete typed-fact declaration.  It is a
// pure domain contract: the engine chooses the guarded fact representation
// and binds these operations at its one composition boundary.  In particular,
// this declaration owns no Program traversal, rule scheduling, or state.
//
// The one-key schema is intentional.  Reachability attaches one Boolean fact
// to an exact Program occurrence; occurrence identity remains in the Program
// coordinate and must not be mirrored as a domain key.
func PlaneConfig() engine.FactorConfig[uint64, Value] {
	return engine.FactorConfig[uint64, Value]{
		Keys:     engine.KeySpace{End: 1},
		Semantic: semantic("factor"),
		Lattice:  Lattice(),
		Default:  Unreachable,
		Fingerprint: func(value Value) uint64 {
			return uint64(value)
		},
		// Widen moves upward through the two-point chain.  The rank is the
		// remaining distance to Top, so every strict transition descends.
		WidenRank: engine.Measure[uint64, Value]{
			Width: 1,
			At: func(_ uint64, value Value, _ int) uint64 {
				return uint64(Reachable - value)
			},
		},
	}
}
