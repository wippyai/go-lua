package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/domain/value/variant/caseset"
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
	return originCasesForType(c, family, allowedVariantCases(allowedCases), valueType)
}

// OriginCasesForTypeView selects from an immutable canonical case view.
func (c *Cache) OriginCasesForTypeView(family uint64, allowedCases caseset.View, valueType typ.Type) ([]int, bool) {
	return originCasesForType(c, family, viewedVariantCases(allowedCases), valueType)
}

// OriginCasesForType selects the cases in family that are compatible with
// valueType, restricted to allowedCases.
func OriginCasesForType(family uint64, allowedCases []int, valueType typ.Type) ([]int, bool) {
	return originCasesForType(nil, family, allowedVariantCases(allowedCases), valueType)
}

// OriginCasesForTypeView selects from an immutable canonical case view.
func OriginCasesForTypeView(family uint64, allowedCases caseset.View, valueType typ.Type) ([]int, bool) {
	return originCasesForType(nil, family, viewedVariantCases(allowedCases), valueType)
}

type variantCases struct {
	values []int
	view   caseset.View
	viewed bool
}

func allowedVariantCases(values []int) variantCases { return variantCases{values: values} }
func viewedVariantCases(view caseset.View) variantCases {
	return variantCases{view: view, viewed: true}
}

func (c variantCases) len() int {
	if c.viewed {
		return c.view.Len()
	}
	return len(c.values)
}

func (c variantCases) contains(value int) bool {
	if c.viewed {
		low, high := 0, c.view.Len()
		for low < high {
			middle := int(uint(low+high) >> 1)
			if c.view.At(middle) < value {
				low = middle + 1
			} else {
				high = middle
			}
		}
		return low < c.view.Len() && c.view.At(low) == value
	}
	for _, candidate := range c.values {
		if candidate == value {
			return true
		}
	}
	return false
}

func originCasesForType(cache *Cache, family uint64, allowedCases variantCases, valueType typ.Type) ([]int, bool) {
	if family == 0 || allowedCases.len() == 0 || valueType == nil || typ.IsAny(valueType) || typ.IsUnknown(valueType) || typ.IsNever(valueType) {
		return nil, false
	}
	cases, ok := variant.OriginCases(family)
	if !ok {
		return nil, false
	}
	selected := make([]int, 0, len(cases))
	for _, c := range cases {
		if !allowedCases.contains(c.Index) {
			continue
		}
		if cache.IsSubtype(valueType, c.Type) || cache.IsSubtype(c.Type, valueType) {
			selected = append(selected, c.Index)
		}
	}
	return selected, len(selected) != 0
}
