package factapply

import (
	"context"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func channelSelectResultValue(
	ctx transfer.NodeContext,
	typeValues *typevalue.Cache,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	selectID factflow.ChannelSelectID,
	cases []factflow.ChannelSelect,
	hasDefault bool,
) (product.Value, bool) {
	if ctx.Registry == nil || len(cases) == 0 && !hasDefault {
		return product.Value{}, false
	}
	if typeValues == nil {
		typeValues = typevalue.NewCache()
	}
	selectEvent := factflow.NewChannelSelect(factflow.ChannelSelectConfig{
		SelectID: selectID, Kind: factflow.ChannelSelectSelect, HasDefault: hasDefault,
	})
	transaction := ChannelSelectTransaction{point: ctx.Point, steps: make([]ChannelSelectStep, 0, len(cases)+1)}
	transaction.steps = append(transaction.steps, ChannelSelectStep{event: selectEvent})
	for _, event := range cases {
		transaction.steps = append(transaction.steps, ChannelSelectStep{event: event})
	}
	prepared, err := PrepareChannelSelectTransaction(ctx.Registry, transaction,
		func(path pathdom.Path) (pathaddr.StateKey, bool) {
			return visibility.AddressAt(resolver, ctx.Point, path).VisibleStateKey()
		},
		func(cfg.Point, int) (uint8, bool) { return 1, true },
	)
	if err != nil {
		return product.Value{}, false
	}
	read := func(path PreparedChannelSelectPath) (product.Value, bool) {
		payload, ok := channelSelectCasePathPayloadType(ctx, typeValues, resolver, projectPath, out, path.SourcePath())
		if !ok {
			return product.Value{}, false
		}
		return typeValues.FromTypeWithWitness(ctx.Registry, payload), true
	}
	evaluated, err := EvaluatePreparedChannelSelect(context.Background(), ctx.Registry, typeValues, prepared, read)
	if err != nil || len(evaluated.writes) != 1 {
		return product.Value{}, false
	}
	return evaluated.writes[0].Value, true
}

func channelSelectCasePathPayloadType(
	ctx transfer.NodeContext,
	typeValues *typevalue.Cache,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	casePath pathdom.Path,
) (typ.Type, bool) {
	resolved, ok := resolvePathValueAtCached(typeValues, ctx.Registry, resolver, ctx.Point, out, casePath, projectPath)
	if ok {
		if payload, ok := channelPayloadTypeFromValue(ctx.Registry, resolved.value); ok {
			return payload, true
		}
	}
	return channelSelectProjectedPayloadType(ctx, typeValues, resolver, projectPath, out, casePath)
}

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
