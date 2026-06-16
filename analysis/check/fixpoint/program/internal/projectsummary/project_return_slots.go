package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
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
	if product.Equal(reg, value, product.Bottom(reg)) {
		return declared
	}
	joined := value
	joined = product.WithPresence(reg, joined, presence.Join(product.PresenceOf(joined), product.PresenceOf(declared)))
	declaredKind := product.Get(reg, declared, runtimekind.Key)
	if !declaredKind.IsTop() {
		joined = product.Set(reg, joined, runtimekind.Key, runtimekind.Join(product.Get(reg, joined, runtimekind.Key), declaredKind))
	}
	declaredEvidence := product.Get(reg, declared, evidence.Key)
	if !evidence.Equal(declaredEvidence, evidence.Top()) {
		joined = product.Set(reg, joined, evidence.Key, evidence.Join(product.Get(reg, joined, evidence.Key), declaredEvidence))
	}
	declaredWitness := product.Get(reg, declared, typewitness.Key)
	if !declaredWitness.IsBottom() && !declaredWitness.IsTop() {
		joined = product.Set(reg, joined, typewitness.Key, declaredWitness)
	}
	declaredOrigin := product.Get(reg, declared, variantorigin.Key)
	if declaredOrigin.IsBottom() || declaredOrigin.IsTop() {
		return joined
	}
	return product.Set(reg, joined, variantorigin.Key, declaredOrigin)
}
