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
	return typeOfUncached(reg, value)
}

// TypeOf projects stable type evidence using this cache's variant-origin
// catalog. It is the preferred query surface inside a check run; the package
// function remains the uncached boundary helper.
func (c *Cache) TypeOf(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	if c == nil {
		return TypeOf(reg, value)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.typeOfLocked(reg, value)
}

func typeOfUncached(reg *axis.Registry, value product.Value) (typ.Type, bool) {
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
			if narrowed, ok := narrowTypeByOrigin(t, origin, nil); ok {
				return typeWithPresence(narrowed, p), true
			}
			return typeWithPresence(t, p), true
		}
	}
	return runtimeKindType(reg, value)
}

func (c *Cache) typeOfLocked(reg *axis.Registry, value product.Value) (typ.Type, bool) {
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
			if narrowed, ok := narrowTypeByOriginLocked(t, origin, c); ok {
				return typeWithPresence(narrowed, p), true
			}
			return typeWithPresence(t, p), true
		}
	}
	if !origin.IsBottom() && !origin.IsTop() {
		if t, ok := c.variantsLocked().TypeFromOrigin(origin.Family(), origin.CasesView()); ok {
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
		narrowed, ok = cache.Variants().NarrowByOrigin(t, origin.Family(), origin.CasesView())
	} else {
		narrowed, ok = variant.NarrowByOrigin(t, origin.Family(), origin.CasesView())
	}
	if ok {
		return narrowed, true
	}
	if cache != nil {
		if originType, ok := cache.Variants().TypeFromOrigin(origin.Family(), origin.CasesView()); ok && cache.IsSubtype(originType, t) {
			return originType, true
		}
	}
	return nil, false
}

func narrowTypeByOriginLocked(t typ.Type, origin variantorigin.Value, cache *Cache) (typ.Type, bool) {
	if t == nil || cache == nil || origin.IsBottom() || origin.IsTop() {
		return nil, false
	}
	narrowed, ok := cache.variantsLocked().NarrowByOrigin(t, origin.Family(), origin.CasesView())
	if ok {
		return narrowed, true
	}
	if originType, ok := cache.variantsLocked().TypeFromOrigin(origin.Family(), origin.CasesView()); ok && cache.IsSubtype(originType, t) {
		return originType, true
	}
	return nil, false
}
