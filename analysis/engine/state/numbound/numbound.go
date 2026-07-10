// Package numbound owns shared mechanics for per-path numeric bounds.
package numbound

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

// Direction selects the dual order for one numeric bound axis.
type Direction uint8

const (
	// Lower records facts of the form value(path) >= bound. Larger bounds are
	// more precise, join keeps the minimum, and widening collapses strict
	// increases to Top.
	Lower Direction = iota
	// Upper records facts of the form value(path) <= bound. Smaller bounds are
	// more precise, join keeps the maximum, and widening snaps strict increases
	// upward to configured thresholds before Top.
	Upper
)

// Spec configures an int64 bound lattice.
type Spec struct {
	Direction  Direction
	Bottom     int64
	Top        int64
	Thresholds []int64
}

// IntDomain builds a direction-aware int64 bound element lattice.
func IntDomain(in Spec) lattice.Lattice[int64] {
	in.Thresholds = normalizeThresholds(in.Thresholds)
	domain := lattice.Lattice[int64]{
		Bottom: func() int64 { return in.Bottom },
		Top:    func() int64 { return in.Top },
		Equal: func(a, b int64) bool {
			return a == b
		},
		LessOrEq: func(a, b int64) bool {
			return elemLessOrEq(in, a, b)
		},
		Join: func(a, b int64) int64 {
			return elemJoin(in, a, b)
		},
		Widen: func(prev, next int64) int64 {
			return elemWiden(in, prev, next)
		},
	}
	if in.Direction == Upper {
		domain.Narrow = func(prev, next int64) int64 {
			return elemNarrow(in, prev, next)
		}
	}
	return domain
}

func elemLessOrEq(spec Spec, a, b int64) bool {
	if spec.Direction == Upper {
		return a <= b
	}
	return a >= b
}

func elemJoin(spec Spec, a, b int64) int64 {
	if spec.Direction == Upper {
		return max(a, b)
	}
	return min(a, b)
}

func elemWiden(spec Spec, prev, next int64) int64 {
	if spec.Direction == Upper {
		if next <= prev {
			return prev
		}
		if threshold, ok := nextThresholdAtLeast(spec.Thresholds, next); ok {
			return threshold
		}
		return spec.Top
	}
	if next > prev {
		return spec.Top
	}
	return min(prev, next)
}

func elemNarrow(spec Spec, prev, next int64) int64 {
	if spec.Direction == Upper && prev == spec.Top && next != spec.Top {
		return next
	}
	return prev
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
	ints := IntDomain(Spec{
		Direction: Lower,
		Bottom:    maxFloor,
		Top:       minFloor,
	})
	return lattice.Lattice[Floor]{
		Bottom:   func() Floor { return Floor{Lo: ints.Bottom()} },
		Top:      func() Floor { return Floor{Lo: ints.Top()} },
		Equal:    func(a, b Floor) bool { return ints.Equal(a.Lo, b.Lo) },
		LessOrEq: func(a, b Floor) bool { return ints.LessOrEq(a.Lo, b.Lo) },
		Join:     func(a, b Floor) Floor { return Floor{Lo: ints.Join(a.Lo, b.Lo)} },
		Widen:    func(prev, next Floor) Floor { return Floor{Lo: ints.Widen(prev.Lo, next.Lo)} },
	}
}

const (
	maxFloor = int64(^uint64(0) >> 1)
	minFloor = -maxFloor - 1
)
