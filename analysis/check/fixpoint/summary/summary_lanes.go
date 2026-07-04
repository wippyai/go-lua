package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type summaryLane struct {
	fieldName      string
	slot           bool
	empty          func(Summary) bool
	assignClone    func(src Summary, dst *Summary)
	normalizeOwned func(reg *axis.Registry, s *Summary)
	equal          func(reg *axis.Registry, a, b Summary, normalized bool) bool
	lessOrEq       func(reg *axis.Registry, a, b Summary) bool
	assignJoin     func(reg *axis.Registry, a, b Summary, out *Summary)
	assignWiden    func(reg *axis.Registry, prev, next Summary, out *Summary)
}

var summaryLanes = derivedSummaryLanes()

func cloneSlice[T any](in []T) []T {
	if len(in) == 0 {
		return nil
	}
	out := make([]T, len(in))
	copy(out, in)
	return out
}

func trimTrailingProducts(reg *axis.Registry, in []product.Value, trailing product.Value) []product.Value {
	for len(in) > 0 && product.Equal(reg, in[len(in)-1], trailing) {
		in = in[:len(in)-1]
	}
	return in
}

func summaryLanesEmpty(s Summary) bool {
	for _, lane := range summaryLanes {
		if !lane.empty(s) {
			return false
		}
	}
	return true
}

func summaryNonSlotLanesEmpty(s Summary) bool {
	for _, lane := range summaryLanes {
		if lane.slot {
			continue
		}
		if !lane.empty(s) {
			return false
		}
	}
	return true
}

func summaryNonSlotLanesEqual(reg *axis.Registry, a, b Summary, normalized bool) bool {
	for _, lane := range summaryLanes {
		if lane.slot {
			continue
		}
		if !lane.equal(reg, a, b, normalized) {
			return false
		}
	}
	return true
}

func summaryNonSlotLanesLessOrEq(reg *axis.Registry, a, b Summary) bool {
	for _, lane := range summaryLanes {
		if lane.slot {
			continue
		}
		if !lane.lessOrEq(reg, a, b) {
			return false
		}
	}
	return true
}

func assignSummaryNonSlotLanesJoin(reg *axis.Registry, a, b Summary, out *Summary) {
	for _, lane := range summaryLanes {
		if lane.slot {
			continue
		}
		lane.assignJoin(reg, a, b, out)
	}
}

func assignSummaryNonSlotLanesWiden(reg *axis.Registry, prev, next Summary, out *Summary) {
	for _, lane := range summaryLanes {
		if lane.slot {
			continue
		}
		lane.assignWiden(reg, prev, next, out)
	}
}
