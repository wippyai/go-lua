package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type StructuralTypeOptions struct {
	ApplyPresence     bool
	OptionalWhenMaybe bool
}

func StructuralTypeOf(reg *axis.Registry, cache *Cache, value product.Value, opts StructuralTypeOptions) (typ.Type, bool) {
	origin := product.Get(reg, value, variantorigin.Key)
	valuePresence := product.PresenceOf(value)
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		if t, ok := witness.Type(); ok {
			t = structuralTypeWithPresence(t, valuePresence, opts)
			if !origin.IsBottom() && !origin.IsTop() {
				if narrowed, ok := cache.NarrowVariantByOrigin(t, origin.Family(), origin.CasesRef()); ok {
					return narrowed, true
				}
				if narrowed, ok := cache.TypeFromVariantOrigin(origin.Family(), origin.CasesRef()); ok {
					return structuralTypeWithPresence(narrowed, valuePresence, opts), true
				}
			}
			return t, true
		}
	}
	if !origin.IsBottom() && !origin.IsTop() {
		if t, ok := cache.TypeFromVariantOrigin(origin.Family(), origin.CasesRef()); ok {
			return structuralTypeWithPresence(t, valuePresence, opts), true
		}
	}
	return nil, false
}

func structuralTypeWithPresence(t typ.Type, p presence.Value, opts StructuralTypeOptions) typ.Type {
	if !opts.ApplyPresence {
		return t
	}
	if opts.OptionalWhenMaybe {
		return typeWithPresence(t, p)
	}
	switch {
	case presence.Equal(p, presence.Absent()):
		return typ.Nil
	case presence.Equal(p, presence.Present()):
		return typeWithPresence(t, p)
	default:
		return t
	}
}
