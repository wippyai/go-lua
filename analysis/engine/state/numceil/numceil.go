// Package numceil owns must-facts about numeric upper bounds on path values.
package numceil

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

// Ceiling records a proven upper bound on a numeric path: value(path) <= Hi.
//
// Information order: a smaller Hi is more precise. Join keeps the weaker common
// upper bound, while widening snaps upward to the next syntactic threshold
// before falling back to Top.
type Ceiling struct {
	Hi int64
}

func MapDomain(thresholds []int64) lattice.Lattice[lift.MustMapLane[keyspace.Key, Ceiling]] {
	return lift.MustMap[keyspace.Key, Ceiling](elemDomain(thresholds))
}

func elemDomain(thresholds []int64) lattice.Lattice[Ceiling] {
	thresholds = normalizeThresholds(thresholds)
	return lattice.Lattice[Ceiling]{
		Bottom: func() Ceiling { return Ceiling{Hi: minCeil} },
		Top:    func() Ceiling { return Ceiling{Hi: maxCeil} },
		Equal: func(a, b Ceiling) bool {
			return a.Hi == b.Hi
		},
		LessOrEq: func(a, b Ceiling) bool {
			return a.Hi <= b.Hi
		},
		Join: func(a, b Ceiling) Ceiling {
			return Ceiling{Hi: maxInt64(a.Hi, b.Hi)}
		},
		Widen: func(prev, next Ceiling) Ceiling {
			if next.Hi <= prev.Hi {
				return prev
			}
			if hi, ok := nextThresholdAtLeast(thresholds, next.Hi); ok {
				return Ceiling{Hi: hi}
			}
			return Ceiling{Hi: maxCeil}
		},
		Narrow: func(prev, next Ceiling) Ceiling {
			if prev.Hi == maxCeil && next.Hi != maxCeil {
				return next
			}
			return prev
		},
	}
}

func normalizeThresholds(in []int64) []int64 {
	if len(in) == 0 {
		return nil
	}
	out := append([]int64(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	n := 0
	for _, v := range out {
		if n == 0 || out[n-1] != v {
			out[n] = v
			n++
		}
	}
	return out[:n]
}

func nextThresholdAtLeast(thresholds []int64, value int64) (int64, bool) {
	i := sort.Search(len(thresholds), func(i int) bool {
		return thresholds[i] >= value
	})
	if i >= len(thresholds) {
		return 0, false
	}
	return thresholds[i], true
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

const (
	maxCeil = int64(^uint64(0) >> 1)
	minCeil = -maxCeil - 1
)
