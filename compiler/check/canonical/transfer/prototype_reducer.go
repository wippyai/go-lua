package transfer

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
)

type prototypeReducer struct{}

var prototypes = prototypeReducer{}

// Prototype facts model Lua split-pattern OOP: local instance bindings remember
// which prototype table backs a storage symbol, while PrototypeSelf records the
// receiver value visible to methods of that prototype.
func (prototypeReducer) recordSelf(out *flow.PointState, proto cfg.SymbolID, value product.AbstractValue) bool {
	if out == nil || proto == 0 || value.IsZero() {
		return false
	}
	before := out.PrototypeSelf
	out.PrototypeSelf = out.PrototypeSelf.JoinValue(proto, value)
	return !flow.PrototypeSelfDomain.Equal(before, out.PrototypeSelf)
}

func (prototypeReducer) bindInstance(out *flow.PointState, sym cfg.SymbolID, proto cfg.SymbolID) bool {
	if out == nil || sym == 0 || proto == 0 {
		return false
	}
	before := out.PrototypeInstances
	out.PrototypeInstances = out.PrototypeInstances.WithPrototype(sym, proto)
	return !flow.PrototypeInstancesDomain.Equal(before, out.PrototypeInstances)
}

func (prototypeReducer) clearInstance(out *flow.PointState, sym cfg.SymbolID) bool {
	if out == nil || sym == 0 {
		return false
	}
	before := out.PrototypeInstances
	out.PrototypeInstances = out.PrototypeInstances.With(sym, nil)
	return !flow.PrototypeInstancesDomain.Equal(before, out.PrototypeInstances)
}
