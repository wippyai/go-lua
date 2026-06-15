// Package numbound owns must-facts about numeric lower bounds on path values.
package numbound

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

// Floor records a proven lower bound on a numeric path: value(path) >= Lo.
//
// Information order: a larger Lo is more precise. Join keeps the weaker common
// lower bound, and widening collapses increasing chains to Top to preserve
// finite fixpoint convergence.
type Floor struct {
	Lo int64
}

func MapDomain() lattice.Lattice[lift.MustMapLane[pathdom.PathKey, Floor]] {
	return lift.MustMap[pathdom.PathKey, Floor](elemDomain())
}

func elemDomain() lattice.Lattice[Floor] {
	return lattice.Lattice[Floor]{
		Bottom: func() Floor { return Floor{Lo: maxFloor} },
		Top:    func() Floor { return Floor{Lo: minFloor} },
		Equal: func(a, b Floor) bool {
			return a.Lo == b.Lo
		},
		LessOrEq: func(a, b Floor) bool {
			return a.Lo >= b.Lo
		},
		Join: func(a, b Floor) Floor {
			return Floor{Lo: minInt64(a.Lo, b.Lo)}
		},
		Widen: func(prev, next Floor) Floor {
			if next.Lo > prev.Lo {
				return Floor{Lo: minFloor}
			}
			return Floor{Lo: minInt64(prev.Lo, next.Lo)}
		},
	}
}

const (
	maxFloor = int64(^uint64(0) >> 1)
	minFloor = -maxFloor - 1
)

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
