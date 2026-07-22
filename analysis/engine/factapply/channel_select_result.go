package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
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

func channelSelectProjectedPayloadType(
	ctx transfer.NodeContext,
	typeValues *typevalue.Cache,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	casePath pathdom.Path,
) (typ.Type, bool) {
	if projected, ok := projectPathDynamicIndexValue(ctx.Registry, resolver, ctx.Point, out, casePath); ok {
		if payload, ok := channelPayloadTypeFromValue(ctx.Registry, projected); ok {
			return payload, true
		}
	}
	if projected, ok := projectPathHeapStaticMemberValue(ctx.Registry, resolver, ctx.Point, out, casePath); ok {
		if payload, ok := channelPayloadTypeFromValue(ctx.Registry, projected); ok {
			return payload, true
		}
	}
	if projected, ok := projectPathStructuralValueCached(typeValues, ctx.Registry, out, casePath, projectPath); ok {
		if payload, ok := channelPayloadTypeFromValue(ctx.Registry, projected); ok {
			return payload, true
		}
	}
	if projected, ok := projectPathOriginValue(typeValues, ctx.Registry, out, casePath, projectPath); ok {
		if payload, ok := channelPayloadTypeFromValue(ctx.Registry, projected); ok {
			return payload, true
		}
	}
	return nil, false
}

func valueWitnessType(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	witness := product.Get(reg, value, typewitness.Key)
	if witness.IsTop() || witness.IsBottom() {
		return nil, false
	}
	return witness.Type()
}
