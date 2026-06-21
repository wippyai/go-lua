// Package numbound owns must-facts about numeric lower bounds on path values.
package numbound

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state/internal/floor"
)

// Floor records a proven lower bound on a numeric path: value(path) >= Lo.
//
// Information order: a larger Lo is more precise. Join keeps the weaker common
// lower bound, and widening collapses increasing chains to Top to preserve
// finite fixpoint convergence.
type Floor struct {
	Lo int64
}

func MapDomain() lattice.Lattice[lift.MustMapLane[keyspace.Key, Floor]] {
	return lift.MustMap[keyspace.Key, Floor](elemDomain())
}

func elemDomain() lattice.Lattice[Floor] {
	return floor.ElementDomain(floor.ElementOps[Floor]{
		BottomLo: maxFloor,
		TopLo:    minFloor,
		New:      func(lo int64) Floor { return Floor{Lo: lo} },
		Lo:       func(f Floor) int64 { return f.Lo },
	})
}

const (
	maxFloor = int64(^uint64(0) >> 1)
	minFloor = -maxFloor - 1
)
