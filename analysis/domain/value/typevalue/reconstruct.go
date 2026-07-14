package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// TypeOf projects stable type evidence out of a product value.
func TypeOf(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	return typeOf(reg, value, nil)
}

// TypeOf projects stable type evidence using this cache's variant-origin
// catalog. It is the preferred query surface inside a check run; the package
// function remains the uncached boundary helper.
func (c *Cache) TypeOf(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	return typeOf(reg, value, c)
}

func typeOf(reg *axis.Registry, value product.Value, cache *Cache) (typ.Type, bool) {
	if reg == nil || product.Equal(reg, value, product.Bottom(reg)) {
		return nil, false
	}
	p := product.PresenceOf(value)
	if presence.Equal(p, presence.Absent()) {
		return typ.Nil, true
	}
	origin := product.Get(reg, value, variantorigin.Key)
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		if t, ok := witness.Type(); ok {
			if narrowed, ok := narrowTypeByOrigin(t, origin, cache); ok {
				return typeWithPresence(narrowed, p), true
			}
			return typeWithPresence(t, p), true
		}
	}
	if !origin.IsBottom() && !origin.IsTop() {
		var (
			t  typ.Type
			ok bool
		)
		if cache != nil {
			t, ok = cache.TypeFromVariantOriginView(origin.Family(), origin.CasesView())
		} else {
			t, ok = variant.TypeFromOriginView(origin.Family(), origin.CasesView())
		}
		if ok {
			return typeWithPresence(t, p), true
		}
	}
	return runtimeKindType(reg, value)
}

func narrowTypeByOrigin(t typ.Type, origin variantorigin.Value, cache *Cache) (typ.Type, bool) {
	if t == nil || origin.IsBottom() || origin.IsTop() {
		return nil, false
	}
	var (
		narrowed typ.Type
		ok       bool
	)
	if cache != nil {
		narrowed, ok = cache.NarrowVariantByOriginView(t, origin.Family(), origin.CasesView())
	} else {
		narrowed, ok = variant.NarrowByOriginView(t, origin.Family(), origin.CasesView())
	}
	if ok {
		return narrowed, true
	}
	var originType typ.Type
	if cache != nil {
		originType, ok = cache.TypeFromVariantOriginView(origin.Family(), origin.CasesView())
	} else {
		originType, ok = variant.TypeFromOriginView(origin.Family(), origin.CasesView())
	}
	if ok && cache.IsSubtype(originType, t) {
		return originType, true
	}
	return nil, false
}
