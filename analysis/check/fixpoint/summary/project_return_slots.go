package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

func projectReturnSlots(
	reg *axis.Registry,
	_ ResultReader,
	exit state.State,
	arity int,
	declared []product.Value,
) []product.Value {
	returns := make([]product.Value, arity)
	for i := range returns {
		returns[i] = exit.ReadValue(reg, key.ReturnSlot(i))
		if i < len(declared) {
			returns[i] = joinDeclaredReturnValue(reg, returns[i], declared[i])
		}
	}
	return returns
}

func joinDeclaredReturnValue(reg *axis.Registry, value product.Value, declared product.Value) product.Value {
	joined := product.Join(reg, value, declared)
	declaredWitness := product.Get(reg, declared, typewitness.Key)
	if !declaredWitness.IsBottom() && !declaredWitness.IsTop() {
		joinedWitness := product.Get(reg, joined, typewitness.Key)
		if joinedWitness.IsTop() {
			joined = product.Set(reg, joined, typewitness.Key, declaredWitness)
		}
	}
	declaredOrigin := product.Get(reg, declared, variantorigin.Key)
	if declaredOrigin.IsBottom() || declaredOrigin.IsTop() {
		return joined
	}
	joinedOrigin := product.Get(reg, joined, variantorigin.Key)
	if !joinedOrigin.IsTop() {
		return joined
	}
	return product.Set(reg, joined, variantorigin.Key, declaredOrigin)
}
