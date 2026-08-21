package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

// DirectActivationTransport is the semantic edge payload emitted from one
// sealed activation-row directory entry.  It is not an owner object and has
// no lifecycle or ownership API; the directory owns the row identity that
// selected-edge lowering checks.
type DirectActivationTransport struct {
	Source PointRef
	Target PointRef
	Factor composition.Key
}

func canonicalDirectActivationRefs(values []PointRef) ([]PointRef, bool) {
	result := append([]PointRef(nil), values...)
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	for index, ref := range result {
		if ref == 0 || index != 0 && ref == result[index-1] {
			return nil, false
		}
	}
	return result, true
}

func canonicalDirectActivationFactors(source *composition.Composition, values []composition.Key) ([]composition.Key, bool) {
	if source == nil {
		return nil, false
	}
	result := append([]composition.Key(nil), values...)
	sort.Slice(result, func(left, right int) bool { return lessKey(result[left], result[right]) })
	for index, factor := range result {
		if !factor.Available() || index != 0 && factor == result[index-1] {
			return nil, false
		}
		if _, known := source.FactorIndex(factor); !known {
			return nil, false
		}
	}
	return result, true
}
