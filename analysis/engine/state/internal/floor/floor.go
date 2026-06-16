// Package floor provides shared mechanics for lower-bound floor lattices owned
// by state leaf packages.
package floor

import "github.com/wippyai/go-lua/analysis/domain/lattice"

// ElementOps describes how a semantic floor package maps its owned value type
// to and from the shared lower-bound mechanics.
type ElementOps[T any] struct {
	BottomLo int64
	TopLo    int64
	New      func(lo int64) T
	Lo       func(T) int64
}

// ElementDomain builds a floor element lattice.
//
// Information order: a larger Lo is more information. Bottom is the strongest
// unreachable floor, Top is the domain-specific no-floor value. Join keeps the
// weaker common lower bound, and Widen collapses strict floor increases to Top
// to bound ascending chains.
func ElementDomain[T any](ops ElementOps[T]) lattice.Lattice[T] {
	return lattice.Lattice[T]{
		Bottom: func() T { return ops.New(ops.BottomLo) },
		Top:    func() T { return ops.New(ops.TopLo) },
		Equal: func(a, b T) bool {
			return ops.Lo(a) == ops.Lo(b)
		},
		LessOrEq: func(a, b T) bool {
			return ops.Lo(a) >= ops.Lo(b)
		},
		Join: func(a, b T) T {
			return ops.New(minInt64(ops.Lo(a), ops.Lo(b)))
		},
		Widen: func(prev, next T) T {
			if ops.Lo(next) > ops.Lo(prev) {
				return ops.New(ops.TopLo)
			}
			return ops.New(minInt64(ops.Lo(prev), ops.Lo(next)))
		},
	}
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
