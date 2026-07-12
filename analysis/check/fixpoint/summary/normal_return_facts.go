package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

func normalizeNormalReturnFacts(reg *axis.Registry, in callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	if in.Empty() {
		return callboundary.NormalReturnFacts{}
	}
	return normalizeNormalReturnFactsWith(reg, in, false)
}

func normalizeOwnedNormalReturnFacts(reg *axis.Registry, in callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	if in.Empty() {
		return callboundary.NormalReturnFacts{}
	}
	return normalizeNormalReturnFactsWith(reg, in, true)
}

func normalizeNormalReturnFactsWith(reg *axis.Registry, in callboundary.NormalReturnFacts, owned bool) callboundary.NormalReturnFacts {
	var out callboundary.NormalReturnFacts
	for _, lane := range normalReturnSummaryLanes {
		handler := lane.Value
		if handler.empty(&in) {
			continue
		}
		if owned {
			handler.normalizeOwned(reg, &in, &out)
		} else {
			handler.normalize(reg, &in, &out)
		}
	}
	if out.Empty() {
		return callboundary.NormalReturnFacts{}
	}
	return out
}

// CloneNormalReturnFacts returns a defensive copy of normal-return fact lanes.
func CloneNormalReturnFacts(in callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	if in.Empty() {
		return callboundary.NormalReturnFacts{}
	}
	var out callboundary.NormalReturnFacts
	for _, lane := range normalReturnSummaryLanes {
		handler := lane.Value
		if handler.empty(&in) {
			continue
		}
		handler.clone(&in, &out)
	}
	return out
}

func normalReturnFactsEqual(reg *axis.Registry, a, b callboundary.NormalReturnFacts) bool {
	a = normalizeNormalReturnFacts(reg, a)
	b = normalizeNormalReturnFacts(reg, b)
	return normalReturnFactsEqualNormalized(reg, a, b)
}

func normalReturnFactsEqualNormalized(reg *axis.Registry, a, b callboundary.NormalReturnFacts) bool {
	for _, lane := range normalReturnSummaryLanes {
		handler := lane.Value
		if handler.empty(&a) && handler.empty(&b) {
			continue
		}
		if !handler.equal(reg, &a, &b) {
			return false
		}
	}
	return true
}

func normalReturnFactsLessOrEq(reg *axis.Registry, a, b callboundary.NormalReturnFacts) bool {
	a = normalizeNormalReturnFacts(reg, a)
	b = normalizeNormalReturnFacts(reg, b)
	for _, lane := range normalReturnSummaryLanes {
		handler := lane.Value
		if handler.empty(&a) && handler.empty(&b) {
			continue
		}
		if !handler.lessOrEq(reg, &a, &b) {
			return false
		}
	}
	return true
}

func joinNormalReturnFacts(reg *axis.Registry, a, b callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	var out callboundary.NormalReturnFacts
	for _, lane := range normalReturnSummaryLanes {
		handler := lane.Value
		if handler.empty(&a) && handler.empty(&b) {
			continue
		}
		handler.join(reg, &a, &b, &out)
	}
	return normalizeNormalReturnFacts(reg, out)
}

func widenNormalReturnFacts(reg *axis.Registry, prev, next callboundary.NormalReturnFacts) callboundary.NormalReturnFacts {
	var out callboundary.NormalReturnFacts
	for _, lane := range normalReturnSummaryLanes {
		handler := lane.Value
		if handler.empty(&prev) && handler.empty(&next) {
			continue
		}
		handler.widen(reg, &prev, &next, &out)
	}
	return normalizeNormalReturnFacts(reg, out)
}
