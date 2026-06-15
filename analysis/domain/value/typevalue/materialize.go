package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// FromType materializes sound point-local value evidence from a type.
func FromType(reg *axis.Registry, t typ.Type) product.Value {
	value := product.Top()
	if typ.IsAny(t) || typ.IsUnknown(t) {
		value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	}
	if p, ok := presenceFromType(t); ok {
		value = product.WithPresence(reg, value, p)
	}
	if kindValue, ok := RuntimeKindFromType(t); ok {
		value = product.Set(reg, value, runtimekind.Key, kindValue)
	}
	if family, cases, ok := variant.OriginOfType(t); ok {
		value = product.Set(reg, value, variantorigin.Key, variantorigin.Of(family, cases))
	}
	return value
}

// WithWitness records exact runtime type-witness evidence on value.
func WithWitness(reg *axis.Registry, value product.Value, t typ.Type) product.Value {
	if witness := typewitness.Of(t); !witness.IsTop() {
		return product.Set(reg, value, typewitness.Key, witness)
	}
	return value
}
