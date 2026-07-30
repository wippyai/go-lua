package sourcevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
)

// StaticIndexValue performs a pure index projection over type/value evidence.
// Identity-backed values require heap state and are deliberately rejected;
// callers must use the stateful dynamic-read kernel for those values.
func StaticIndexValue(reg *axis.Registry, typeValues *typevalue.Cache, owner, key product.Value) (product.Value, bool) {
	if reg == nil || !presence.Equal(product.PresenceOf(owner), presence.Present()) {
		return product.Value{}, false
	}
	if _, identityBacked := product.Get(reg, owner, identity.Key).ID(); identityBacked {
		return product.Value{}, false
	}
	if _, exact := typevalue.ExactScalarKeySegment(reg, typeValues, key); !exact {
		return product.Value{}, false
	}
	var value product.Value
	var ok bool
	if typeValues == nil {
		value, ok = typevalue.RuntimeIndex(reg, owner, key)
	} else {
		value, ok = typeValues.RuntimeIndex(reg, owner, key)
	}
	if !ok {
		return product.Value{}, false
	}
	return InheritTopOriginEvidence(reg, value, owner), true
}
