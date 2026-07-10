package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// VariantOriginOfValue returns finite variant-origin evidence carried by value,
// translated through an exact type witness when that witness gives a better
// common family for path-sensitive narrowing.
func VariantOriginOfValue(reg *axis.Registry, cache *Cache, value product.Value) (variantorigin.Value, bool) {
	origin := product.Get(reg, value, variantorigin.Key)
	if witnessOrigin, ok := witnessVariantOrigin(reg, cache, value); ok {
		if !origin.IsBottom() && !origin.IsTop() {
			if origin.Family() != witnessOrigin.Family() {
				return witnessOrigin, true
			}
			witnessOrigin = variantorigin.Meet(origin, witnessOrigin)
			if witnessOrigin.IsBottom() {
				return variantorigin.Value{}, false
			}
		}
		return witnessOrigin, true
	}
	t, ok := cache.TypeOf(reg, value)
	if !ok {
		if !origin.IsBottom() && !origin.IsTop() {
			return origin, true
		}
		return variantorigin.Value{}, false
	}
	var family uint64
	var cases []int
	if cache != nil {
		family, cases, ok = cache.OriginOfType(t)
	} else {
		family, cases, ok = variant.OriginOfType(t)
	}
	if !ok {
		if !origin.IsBottom() && !origin.IsTop() {
			return origin, true
		}
		return variantorigin.Value{}, false
	}
	derived := variantorigin.Of(family, cases)
	if !origin.IsBottom() && !origin.IsTop() {
		if origin.Family() != derived.Family() {
			return origin, true
		}
		derived = variantorigin.Meet(origin, derived)
		if derived.IsBottom() {
			return variantorigin.Value{}, false
		}
	}
	return derived, true
}

func witnessVariantOrigin(reg *axis.Registry, cache *Cache, value product.Value) (variantorigin.Value, bool) {
	witness := product.Get(reg, value, typewitness.Key)
	witnessType, ok := witness.Type()
	if !ok || witnessType == nil || typ.IsAny(witnessType) || typ.IsUnknown(witnessType) || typ.IsNever(witnessType) {
		return variantorigin.Value{}, false
	}
	var family uint64
	var cases []int
	if cache != nil {
		family, cases, ok = cache.OriginOfType(witnessType)
	} else {
		family, cases, ok = variant.OriginOfType(witnessType)
	}
	if !ok {
		return variantorigin.Value{}, false
	}
	if valueType, ok := cache.TypeOf(reg, value); ok {
		if selected, ok := cache.OriginCasesForType(family, cases, valueType); ok {
			return variantorigin.Of(family, selected), true
		}
	}
	return variantorigin.Of(family, cases), true
}

func (c *Cache) OriginCasesForType(family uint64, allowedCases []int, valueType typ.Type) ([]int, bool) {
	return originCasesForType(c, family, allowedCases, valueType)
}

// OriginCasesForType selects the cases in family that are compatible with
// valueType, restricted to allowedCases.
func OriginCasesForType(family uint64, allowedCases []int, valueType typ.Type) ([]int, bool) {
	return originCasesForType(nil, family, allowedCases, valueType)
}

func originCasesForType(cache *Cache, family uint64, allowedCases []int, valueType typ.Type) ([]int, bool) {
	if family == 0 || len(allowedCases) == 0 || valueType == nil || typ.IsAny(valueType) || typ.IsUnknown(valueType) || typ.IsNever(valueType) {
		return nil, false
	}
	cases, ok := variant.OriginCases(family)
	if !ok {
		return nil, false
	}
	allowed := make(map[int]bool, len(allowedCases))
	for _, c := range allowedCases {
		allowed[c] = true
	}
	selected := make([]int, 0, len(cases))
	for _, c := range cases {
		if !allowed[c.Index] {
			continue
		}
		if cache.IsSubtype(valueType, c.Type) || cache.IsSubtype(c.Type, valueType) {
			selected = append(selected, c.Index)
		}
	}
	return selected, len(selected) != 0
}
