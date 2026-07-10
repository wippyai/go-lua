// Package lenbound owns the must-fact lane recording proven lower bounds on the
// length of an array-typed path: a Floor{Lo} entry for path P asserts that
// len(P) >= Lo holds at the point. Floors are established only by an explicit
// guard on a branch's true edge; they are never raised on a back-edge.
package lenbound

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state/internal/floor"
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
func MapDomain() lattice.Lattice[lift.MustMapLane[keyspace.Key, Floor]] {
	return lift.MustMap[keyspace.Key, Floor](elemDomain())
}

// elemDomain builds the Floor element lattice.
//
// Termination: Join is min(Lo), monotone downward in information. Widen
// collapses to Top (Lo == 0) on any strict increase of the floor between
// iterations, so the per-key ascending chain has height at most two
// (no-floor-or-lower -> a single concrete floor -> collapse), which bounds the
// fixpoint and protects convergence in the engine.
func elemDomain() lattice.Lattice[Floor] {
	return floor.ElementDomain(floor.ElementOps[Floor]{
		BottomLo: maxFloor,
		TopLo:    0,
		New:      func(lo int64) Floor { return Floor{Lo: lo} },
		Lo:       func(f Floor) int64 { return f.Lo },
	})
}

const maxFloor = int64(^uint64(0) >> 1)
