package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func channelPayloadTypeFromValue(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	channelType, ok := valueWitnessType(reg, value)
	if !ok {
		return nil, false
	}
	return ambient.ChannelPayloadType(channelType)
}

func valueWitnessType(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	witness := product.Get(reg, value, typewitness.Key)
	if witness.IsTop() || witness.IsBottom() {
		return nil, false
	}
	return witness.Type()
}
