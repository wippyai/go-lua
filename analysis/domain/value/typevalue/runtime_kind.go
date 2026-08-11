package typevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekindof"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// RuntimeKindFromType returns concrete Lua runtime-kind evidence for t. The
// canonical mapping lives in axis/runtimekindof so the typewitness reducer and
// this projection share one implementation.
func RuntimeKindFromType(t typ.Type) (runtimekind.Value, bool) {
	return runtimekindof.RuntimeKindFromType(t)
}

// RefineWitnessByRuntimeKind narrows value's reconstructed type witness by a
// runtime-kind proof and returns a witnessed product value for the narrowed
// type. This keeps callers outside typevalue from importing type internals just
// to reject never after runtime-kind restriction.
func (c *Cache) RefineWitnessByRuntimeKind(reg *axis.Registry, value product.Value, allowed runtimekind.Value) (product.Value, bool) {
	if c == nil {
		t, ok := TypeOf(reg, value)
		if !ok {
			return product.Value{}, false
		}
		narrowed, changed := runtimekindof.RestrictTypeToRuntimeKind(t, allowed)
		if !changed || typ.IsNever(narrowed) {
			return product.Value{}, false
		}
		return WithWitness(reg, FromType(reg, narrowed), narrowed), true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.typeOfLocked(reg, value)
	if !ok {
		return product.Value{}, false
	}
	narrowed, changed := runtimekindof.RestrictTypeToRuntimeKind(t, allowed)
	if !changed || typ.IsNever(narrowed) {
		return product.Value{}, false
	}
	return c.fromTypeWithWitnessLocked(reg, narrowed), true
}

// HasLuaTypeEvidenceAxes reports whether reg contains the standard sparse axes
// needed to recover Lua type evidence from product values.
func HasLuaTypeEvidenceAxes(reg *axis.Registry) bool {
	return registryHasAxis(reg, variantorigin.Key.ID()) &&
		registryHasAxis(reg, typewitness.Key.ID()) &&
		registryHasAxis(reg, runtimekind.Key.ID())
}

// RecoverRuntimeKindWitnessMeet refines value with a runtime-kind-only
// constraint while preserving compatible type-witness evidence. It owns the
// runtime-kind/type-witness interaction so higher-level refinement code does not
// need to know the runtime-kind axis internals.
func RecoverRuntimeKindWitnessMeet(reg *axis.Registry, value, constraint product.Value) (product.Value, bool) {
	if !registryHasAxis(reg, runtimekind.Key.ID()) || !registryHasAxis(reg, typewitness.Key.ID()) {
		return product.Value{}, false
	}
	allowed := product.Get(reg, constraint, runtimekind.Key)
	if allowed.IsTop() || allowed.IsBottom() {
		return product.Value{}, false
	}
	if witness := product.Get(reg, constraint, typewitness.Key); !witness.IsTop() {
		return product.Value{}, false
	}
	valueType, ok := WitnessOf(reg, value)
	if !ok || valueType == nil || typ.IsAny(valueType) || typ.IsUnknown(valueType) {
		return product.Value{}, false
	}
	narrowed := valueType
	if valueKinds, ok := runtimekindof.RuntimeKindFromType(valueType); !ok || !allowed.Covers(valueKinds) {
		var changed bool
		narrowed, changed = runtimekindof.RestrictTypeToRuntimeKind(valueType, allowed)
		if !changed || typ.IsNever(narrowed) {
			return product.Value{}, false
		}
	}
	valueWithoutWitness := product.Set(reg, value, typewitness.Key, typewitness.Top())
	constraintWithoutWitness := product.Set(reg, constraint, typewitness.Key, typewitness.Top())
	refined := product.Meet(reg, valueWithoutWitness, constraintWithoutWitness)
	if product.Equal(reg, refined, product.Bottom(reg)) {
		return product.Value{}, false
	}
	return WithWitness(reg, refined, narrowed), true
}

func registryHasAxis(reg *axis.Registry, id string) bool {
	if reg == nil {
		return false
	}
	_, ok := reg.LookupErased(id)
	return ok
}
