package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// FromType materializes sound point-local value evidence from a type.
func FromType(reg *axis.Registry, t typ.Type) product.Value {
	return fromType(reg, t, nil)
}

// fromType materializes sound point-local value evidence from a type.
func fromType(reg *axis.Registry, t typ.Type, cache *Cache) product.Value {
	ed := product.Edit(reg, product.Top())
	if typ.IsAny(t) || typ.IsUnknown(t) {
		product.EditSet(&ed, evidence.Key, evidence.ExplicitTop())
	}
	if p, ok := presenceFromType(t); ok {
		ed.SetPresence(p)
	}
	if kindValue, ok := RuntimeKindFromType(t); ok {
		product.EditSet(&ed, runtimekind.Key, kindValue)
	}
	if family, cases, ok := cache.originOfType(t); ok {
		product.EditSet(&ed, variantorigin.Key, variantorigin.Of(family, cases))
	}
	return ed.Done()
}

// Nil materializes the canonical nil value: presence-absent carrying the
// typ.Nil witness, so a nil source (uninitialized local, over-arity fill) joins
// identically to an explicit `= nil` instead of being absorbed as join identity.
func Nil(reg *axis.Registry) product.Value {
	return WithWitness(reg, FromType(reg, typ.Nil), typ.Nil)
}

// WithWitness records exact runtime type-witness evidence on value.
func WithWitness(reg *axis.Registry, value product.Value, t typ.Type) product.Value {
	if witness := typewitness.Of(t); !witness.IsTop() {
		return product.Set(reg, value, typewitness.Key, witness)
	}
	return value
}
