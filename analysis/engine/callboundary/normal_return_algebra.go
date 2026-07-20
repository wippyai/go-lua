package callboundary

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

func NormalizeNormalReturnFacts(reg *axis.Registry, in NormalReturnFacts) NormalReturnFacts {
	if in.Empty() {
		return NormalReturnFacts{}
	}
	return normalizeNormalReturnFactsWith(reg, in, false)
}

func NormalizeOwnedNormalReturnFacts(reg *axis.Registry, in NormalReturnFacts) NormalReturnFacts {
	if in.Empty() {
		return NormalReturnFacts{}
	}
	return normalizeNormalReturnFactsWith(reg, in, true)
}

func normalizeNormalReturnFactsWith(reg *axis.Registry, in NormalReturnFacts, owned bool) NormalReturnFacts {
	var out NormalReturnFacts
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
		return NormalReturnFacts{}
	}
	return out
}

// CloneNormalReturnFacts returns a defensive copy of normal-return fact lanes.
func CloneNormalReturnFacts(in NormalReturnFacts) NormalReturnFacts {
	if in.Empty() {
		return NormalReturnFacts{}
	}
	var out NormalReturnFacts
	for _, lane := range normalReturnSummaryLanes {
		handler := lane.Value
		if handler.empty(&in) {
			continue
		}
		handler.clone(&in, &out)
	}
	return out
}

func NormalReturnFactsEqual(reg *axis.Registry, a, b NormalReturnFacts) bool {
	a = NormalizeNormalReturnFacts(reg, a)
	b = NormalizeNormalReturnFacts(reg, b)
	return NormalReturnFactsEqualNormalized(reg, a, b)
}

func NormalReturnFactsEqualNormalized(reg *axis.Registry, a, b NormalReturnFacts) bool {
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

func NormalReturnFactsLessOrEq(reg *axis.Registry, a, b NormalReturnFacts) bool {
	a = NormalizeNormalReturnFacts(reg, a)
	b = NormalizeNormalReturnFacts(reg, b)
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

func JoinNormalReturnFacts(reg *axis.Registry, a, b NormalReturnFacts) NormalReturnFacts {
	var out NormalReturnFacts
	for _, lane := range normalReturnSummaryLanes {
		handler := lane.Value
		if handler.empty(&a) && handler.empty(&b) {
			continue
		}
		handler.join(reg, &a, &b, &out)
	}
	return NormalizeNormalReturnFacts(reg, out)
}

func WidenNormalReturnFacts(reg *axis.Registry, prev, next NormalReturnFacts) NormalReturnFacts {
	var out NormalReturnFacts
	for _, lane := range normalReturnSummaryLanes {
		handler := lane.Value
		if handler.empty(&prev) && handler.empty(&next) {
			continue
		}
		handler.widen(reg, &prev, &next, &out)
	}
	return NormalizeNormalReturnFacts(reg, out)
}
