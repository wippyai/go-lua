package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
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

// HasConcreteType reports whether value carries type evidence precise enough to
// prefer over a broader cached declaration. Unknown, any, and never are not
// concrete refinements.
func HasConcreteType(reg *axis.Registry, value product.Value) bool {
	return hasConcreteType(reg, value, nil)
}

// HasConcreteType reports whether value carries concrete type evidence using
// this cache's query surface.
func (c *Cache) HasConcreteType(reg *axis.Registry, value product.Value) bool {
	return hasConcreteType(reg, value, c)
}

func hasConcreteType(reg *axis.Registry, value product.Value, cache *Cache) bool {
	if reg == nil || product.Equal(reg, value, product.Bottom(reg)) || product.Equal(reg, value, product.Top()) {
		return false
	}
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		if t, ok := witness.Type(); ok && typ.IsNever(t) {
			return false
		}
	}
	t, ok := cache.TypeOf(reg, value)
	return ok && t != nil && !typ.IsAny(t) && !typ.IsUnknown(t) && !typ.IsNever(t)
}

// WithWitness records exact runtime type-witness evidence on value.
func WithWitness(reg *axis.Registry, value product.Value, t typ.Type) product.Value {
	if witness := typewitness.Of(t); !witness.IsTop() {
		if typewitness.Equal(product.Get(reg, value, typewitness.Key), witness) {
			return value
		}
		return product.Set(reg, value, typewitness.Key, witness)
	}
	return value
}

// MergeDeclaredTypeFacts carries type-derived presence, runtime-kind, and
// top-origin facts from a declared contract into a computed value. It keeps the
// axis leaves owned by typevalue so callers that refine contracts do not need to
// know the carrier vocabulary.
func MergeDeclaredTypeFacts(reg *axis.Registry, value, declared product.Value) product.Value {
	ed := product.Edit(reg, value)
	ed.SetPresence(presence.Join(product.PresenceOf(value), product.PresenceOf(declared)))
	declaredKind := product.Get(reg, declared, runtimekind.Key)
	if declaredKind.IsTop() {
		if witness := product.Get(reg, declared, typewitness.Key); !witness.IsTop() && !witness.IsBottom() {
			if t, ok := witness.Type(); ok {
				if kindValue, ok := RuntimeKindFromType(t); ok {
					declaredKind = kindValue
				}
			}
		}
	}
	if !declaredKind.IsTop() {
		product.EditSet(&ed, runtimekind.Key, runtimekind.Join(product.Get(reg, value, runtimekind.Key), declaredKind))
	}
	declaredEvidence := product.Get(reg, declared, evidence.Key)
	if !evidence.Equal(declaredEvidence, evidence.Top()) {
		product.EditSet(&ed, evidence.Key, evidence.Join(product.Get(reg, value, evidence.Key), declaredEvidence))
	}
	return ed.Done()
}
