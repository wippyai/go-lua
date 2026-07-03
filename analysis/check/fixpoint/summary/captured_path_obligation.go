package summary

import (
	"sort"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

func normalizeCapturedPathObligations(reg *axis.Registry, in []CapturedPathObligation) []CapturedPathObligation {
	if len(in) == 0 || reg == nil {
		return nil
	}
	top := product.Top()
	merged := make(map[pathaddr.StableKey]product.Value, len(in))
	for _, fact := range in {
		if !capturedPathObligationValid(reg, fact) {
			continue
		}
		if existing, ok := merged[fact.Path]; ok {
			merged[fact.Path] = product.Meet(reg, existing, fact.Value)
			continue
		}
		merged[fact.Path] = fact.Value
	}
	out := make([]CapturedPathObligation, 0, len(merged))
	for path, value := range merged {
		if product.Equal(reg, value, top) || product.Equal(reg, value, product.Bottom(reg)) {
			continue
		}
		out = append(out, CapturedPathObligation{Path: path, Value: value})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func capturedPathObligationValid(reg *axis.Registry, fact CapturedPathObligation) bool {
	if reg == nil || !UsefulParamObligation(reg, fact.Value) {
		return false
	}
	_, ok := pathaddr.StableFromKey(fact.Path.PathKey())
	return ok
}

func capturedPathObligationsEqual(reg *axis.Registry, a, b []CapturedPathObligation) bool {
	a = normalizeCapturedPathObligations(reg, a)
	b = normalizeCapturedPathObligations(reg, b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Path != b[i].Path || !product.Equal(reg, a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

func capturedPathObligationsLessOrEq(reg *axis.Registry, a, b []CapturedPathObligation) bool {
	am := capturedPathObligationValueMap(reg, a)
	bm := capturedPathObligationValueMap(reg, b)
	top := product.Top()
	for path, left := range am {
		right, ok := bm[path]
		if !ok {
			right = top
		}
		if !product.LessOrEq(reg, right, left) {
			return false
		}
	}
	for path, right := range bm {
		if _, ok := am[path]; ok {
			continue
		}
		if !product.LessOrEq(reg, right, top) {
			return false
		}
	}
	return true
}

func joinCapturedPathObligations(reg *axis.Registry, a, b []CapturedPathObligation) []CapturedPathObligation {
	return combineCapturedPathObligations(reg, a, b)
}

func widenCapturedPathObligations(reg *axis.Registry, prev, next []CapturedPathObligation) []CapturedPathObligation {
	return combineCapturedPathObligations(reg, prev, next)
}

func combineCapturedPathObligations(reg *axis.Registry, a, b []CapturedPathObligation) []CapturedPathObligation {
	if len(a) == 0 {
		return normalizeCapturedPathObligations(reg, b)
	}
	if len(b) == 0 {
		return normalizeCapturedPathObligations(reg, a)
	}
	merged := capturedPathObligationValueMap(reg, a)
	for path, value := range capturedPathObligationValueMap(reg, b) {
		if existing, ok := merged[path]; ok {
			merged[path] = product.Meet(reg, existing, value)
			continue
		}
		merged[path] = value
	}
	out := make([]CapturedPathObligation, 0, len(merged))
	for path, value := range merged {
		out = append(out, CapturedPathObligation{Path: path, Value: value})
	}
	return normalizeCapturedPathObligations(reg, out)
}

func capturedPathObligationValueMap(reg *axis.Registry, in []CapturedPathObligation) map[pathaddr.StableKey]product.Value {
	normalized := normalizeCapturedPathObligations(reg, in)
	if len(normalized) == 0 {
		return nil
	}
	out := make(map[pathaddr.StableKey]product.Value, len(normalized))
	for _, fact := range normalized {
		out[fact.Path] = fact.Value
	}
	return out
}
