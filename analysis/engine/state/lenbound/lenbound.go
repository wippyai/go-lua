// Package lenbound owns the must-fact lane recording proven lower bounds on the
// length of an array-typed path: a Floor{Lo} entry for path P asserts that
// len(P) >= Lo holds at the point. Floors are established only by an explicit
// guard on a branch's true edge; they are never raised on a back-edge.
package lenbound

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

// Floor records a proven lower bound on a path's length: len(path) >= Lo.
//
// Information order: a larger Lo is strictly more information, so it is lower in
// the lattice. Bottom is the unreachable +infinity floor; Top is the no-floor
// element Lo == 0. Join is min(Lo): a floor holds at a merge only when it holds
// on every incoming edge.
type Floor struct {
	Lo int64
}

// MapDomain builds the per-path must-map lattice for length floors. The lane
// keeps only keys present on all edges (must semantics) and merges per-key
// floors with the Floor element lattice.
func MapDomain() lattice.Lattice[lift.MustMapLane[pathdom.PathKey, Floor]] {
	return lift.MustMap[pathdom.PathKey, Floor](elemDomain())
}

// elemDomain builds the Floor element lattice.
//
// Termination: Join is min(Lo), monotone downward in information. Widen
// collapses to Top (Lo == 0) on any strict increase of the floor between
// iterations, so the per-key ascending chain has height at most two
// (no-floor-or-lower -> a single concrete floor -> collapse), which bounds the
// fixpoint and protects the per-fixture deadline.
func elemDomain() lattice.Lattice[Floor] {
	return lattice.Lattice[Floor]{
		Bottom: func() Floor { return Floor{Lo: maxFloor} },
		Top:    func() Floor { return Floor{Lo: 0} },
		Equal: func(a, b Floor) bool {
			return a.Lo == b.Lo
		},
		LessOrEq: func(a, b Floor) bool {
			// a <= b when a carries at least as much information (>= floor).
			return a.Lo >= b.Lo
		},
		Join: func(a, b Floor) Floor {
			return Floor{Lo: minInt64(a.Lo, b.Lo)}
		},
		Widen: func(prev, next Floor) Floor {
			if next.Lo > prev.Lo {
				return Floor{Lo: 0}
			}
			return Floor{Lo: minInt64(prev.Lo, next.Lo)}
		},
	}
}

const maxFloor = int64(^uint64(0) >> 1)

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
