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
	if reg == nil || product.Equal(reg, value, product.Bottom(reg)) {
		return nil, false
	}
	p := product.PresenceOf(value)
	if presence.Equal(p, presence.Absent()) {
		return typ.Nil, true
	}
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		if t, ok := witness.Type(); ok {
			return typeWithPresence(t, p), true
		}
	}
	origin := product.Get(reg, value, variantorigin.Key)
	if !origin.IsBottom() && !origin.IsTop() {
		if t, ok := variant.TypeFromOrigin(origin.Family(), origin.Cases()); ok {
			return typeWithPresence(t, p), true
		}
	}
	return runtimeKindType(reg, value)
}
