package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

func normalizeNormalReturnFacts(reg *axis.Registry, in callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	return normalizeNormalReturnFactsWith(reg, in, false)
}

func normalizeOwnedNormalReturnFacts(reg *axis.Registry, in callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	return normalizeNormalReturnFactsWith(reg, in, true)
}

func normalizeNormalReturnFactsWith(reg *axis.Registry, in callboundary.NormalReturnFacts, owned bool) callboundary.NormalReturnFacts {
	if in.Empty() {
		return callboundary.NormalReturnFacts{}
	}
	var out callboundary.NormalReturnFacts
	for _, lane := range normalReturnSummaryLanes {
		if lane.empty(&in) {
			continue
		}
		if owned {
			lane.normalizeOwned(reg, &in, &out)
		} else {
			lane.normalize(reg, &in, &out)
		}
	}
	if out.Empty() {
		return callboundary.NormalReturnFacts{}
	}
	return out
}

func cloneNormalReturnFacts(in callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	if in.Empty() {
		return callboundary.NormalReturnFacts{}
	}
	var out callboundary.NormalReturnFacts
	for _, lane := range normalReturnSummaryLanes {
		if lane.empty(&in) {
			continue
		}
		lane.clone(&in, &out)
	}
	return out
}

// CloneNormalReturnFacts returns a defensive copy of normal-return fact lanes.
func CloneNormalReturnFacts(in callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	return cloneNormalReturnFacts(in)
}

func normalReturnFactsEqual(reg *axis.Registry, a, b callboundary.NormalReturnFacts) bool {
	a = normalizeNormalReturnFacts(reg, a)
	b = normalizeNormalReturnFacts(reg, b)
	return normalReturnFactsEqualNormalized(reg, a, b)
}

func normalReturnFactsEqualNormalized(reg *axis.Registry, a, b callboundary.NormalReturnFacts) bool {
	for _, lane := range normalReturnSummaryLanes {
		if lane.empty(&a) && lane.empty(&b) {
			continue
		}
		if !lane.equal(reg, &a, &b) {
			return false
		}
	}
	return true
}

func normalReturnFactsLessOrEq(reg *axis.Registry, a, b callboundary.NormalReturnFacts) bool {
	a = normalizeNormalReturnFacts(reg, a)
	b = normalizeNormalReturnFacts(reg, b)
	for _, lane := range normalReturnSummaryLanes {
		if lane.empty(&a) && lane.empty(&b) {
			continue
		}
		if !lane.lessOrEq(reg, &a, &b) {
			return false
		}
	}
	return true
}

func joinNormalReturnFacts(reg *axis.Registry, a, b callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	var out callboundary.NormalReturnFacts
	for _, lane := range normalReturnSummaryLanes {
		if lane.empty(&a) && lane.empty(&b) {
			continue
		}
		lane.join(reg, &a, &b, &out)
	}
	return normalizeNormalReturnFacts(reg, out)
}

func widenNormalReturnFacts(reg *axis.Registry, prev, next callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	var out callboundary.NormalReturnFacts
	for _, lane := range normalReturnSummaryLanes {
		if lane.empty(&prev) && lane.empty(&next) {
			continue
		}
		lane.widen(reg, &prev, &next, &out)
	}
	return normalizeNormalReturnFacts(reg, out)
}
