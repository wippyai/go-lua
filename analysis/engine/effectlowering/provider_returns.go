package effectlowering

import (
	"reflect"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/capability"
	caplabel "github.com/wippyai/go-lua/analysis/domain/effect/capability/label"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/projection"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ReturnTypeOps carries caller-owned type operations needed by return
// transforms that are not part of engine ownership.
type ReturnTypeOps struct {
	CallableReturn         func(typ.Type) (typ.Type, bool)
	ElementOf              func(typ.Type) (typ.Type, bool)
	TypeProjection         func(typ.Type, projection.Projection) (typ.Type, bool)
	InstantiateGenericCall func(*typ.Function, []typ.Type) (*typ.Function, bool)
}

func signatureReturnValue(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	expressionRefinements map[factflow.ExprRef]factflow.ExpressionRefinement,
	sig signature.Function,
	index int,
	in state.State,
	read func(cfg.Point) state.State,
	returnTypeOps ReturnTypeOps,
) (product.Value, bool) {
	transform, ok := activeReturnTransform(sig, index)
	if !ok {
		return product.Value{}, false
	}
	switch transform := transform.(type) {
	case returns.SameAs:
		return sameAsReturnValue(ctx, facts, sources, expressionRefinements, transform.Source, in, read)
	case *returns.SameAs:
		if transform == nil {
			return product.Value{}, false
		}
		return sameAsReturnValue(ctx, facts, sources, expressionRefinements, transform.Source, in, read)
	case returns.ElementOf:
		return elementOfReturnValue(ctx, facts, sig, transform.Source, returnTypeOps)
	case *returns.ElementOf:
		if transform == nil {
			return product.Value{}, false
		}
		return elementOfReturnValue(ctx, facts, sig, transform.Source, returnTypeOps)
	case returns.OptionalElementOf:
		return optionalElementOfReturnValue(ctx, facts, sig, transform.Source, returnTypeOps)
	case *returns.OptionalElementOf:
		if transform == nil {
			return product.Value{}, false
		}
		return optionalElementOfReturnValue(ctx, facts, sig, transform.Source, returnTypeOps)
	case returns.CallbackReturn:
		return callbackReturnValue(ctx, facts, sig, transform.CallbackParam, false, returnTypeOps)
	case *returns.CallbackReturn:
		if transform == nil {
			return product.Value{}, false
		}
		return callbackReturnValue(ctx, facts, sig, transform.CallbackParam, false, returnTypeOps)
	case returns.ArrayOfCallbackReturn:
		return callbackReturnValue(ctx, facts, sig, transform.CallbackParam, true, returnTypeOps)
	case *returns.ArrayOfCallbackReturn:
		if transform == nil {
			return product.Value{}, false
		}
		return callbackReturnValue(ctx, facts, sig, transform.CallbackParam, true, returnTypeOps)
	case returns.TypeProjection:
		return typeProjectionReturnValue(ctx, facts, sig, transform, returnTypeOps)
	case *returns.TypeProjection:
		if transform == nil {
			return product.Value{}, false
		}
		return typeProjectionReturnValue(ctx, facts, sig, *transform, returnTypeOps)
	default:
		return product.Value{}, false
	}
}

func sameAsReturnValue(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	expressionRefinements map[factflow.ExprRef]factflow.ExpressionRefinement,
	ref effect.ParamRef,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	if sources == nil {
		return product.Value{}, false
	}
	site, ok := facts.CallSite(ctx.Point)
	if !ok {
		return product.Value{}, false
	}
	args := site.ArgumentSources()
	argIndex, ok := effect.ResolveParamIndex(ref, len(args))
	if !ok {
		return product.Value{}, false
	}
	return sourcevalue.WithExpressionRefinements(ctx.Registry, sources, expressionRefinements).ValueOfSource(ctx.Point, args[argIndex], in, read)
}

func elementOfReturnValue(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sig signature.Function,
	ref effect.ParamRef,
	returnTypeOps ReturnTypeOps,
) (product.Value, bool) {
	if returnTypeOps.ElementOf == nil {
		return product.Value{}, false
	}
	site, ok := facts.CallSite(ctx.Point)
	if !ok {
		return product.Value{}, false
	}
	args := site.ArgumentSources()
	argIndex, ok := effect.ResolveParamIndex(ref, len(args))
	if !ok || sig.Type == nil || argIndex < 0 || argIndex >= len(sig.Type.Params) {
		return product.Value{}, false
	}
	elem, ok := returnTypeOps.ElementOf(sig.Type.Params[argIndex].Type)
	if !ok {
		return product.Value{}, false
	}
	return returnValueFromType(ctx.Registry, elem), true
}

func optionalElementOfReturnValue(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sig signature.Function,
	ref effect.ParamRef,
	returnTypeOps ReturnTypeOps,
) (product.Value, bool) {
	value, ok := elementOfReturnValue(ctx, facts, sig, ref, returnTypeOps)
	if !ok {
		return product.Value{}, false
	}
	return product.WithPresence(ctx.Registry, value, presence.Maybe()), true
}

func callbackReturnValue(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sig signature.Function,
	ref effect.ParamRef,
	array bool,
	returnTypeOps ReturnTypeOps,
) (product.Value, bool) {
	if returnTypeOps.CallableReturn == nil {
		return product.Value{}, false
	}
	site, ok := facts.CallSite(ctx.Point)
	if !ok {
		return product.Value{}, false
	}
	args := site.ArgumentSources()
	argIndex, ok := effect.ResolveParamIndex(ref, len(args))
	if !ok || sig.Type == nil || argIndex < 0 || argIndex >= len(sig.Type.Params) {
		return product.Value{}, false
	}
	ret, ok := returnTypeOps.CallableReturn(sig.Type.Params[argIndex].Type)
	if !ok {
		return product.Value{}, false
	}
	if array {
		ret = typ.NewArray(ret)
	}
	return returnValueFromType(ctx.Registry, ret), true
}

func typeProjectionReturnValue(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sig signature.Function,
	transform returns.TypeProjection,
	returnTypeOps ReturnTypeOps,
) (product.Value, bool) {
	if returnTypeOps.TypeProjection == nil {
		return product.Value{}, false
	}
	site, ok := facts.CallSite(ctx.Point)
	if !ok {
		return product.Value{}, false
	}
	args := site.ArgumentSources()
	argIndex, ok := effect.ResolveParamIndex(transform.Source, len(args))
	if !ok || sig.Type == nil || argIndex < 0 || argIndex >= len(sig.Type.Params) {
		return product.Value{}, false
	}
	projected, ok := returnTypeOps.TypeProjection(sig.Type.Params[argIndex].Type, transform.Projection)
	if !ok {
		return product.Value{}, false
	}
	return returnValueFromType(ctx.Registry, projected), true
}

func returnValueFromType(reg *axis.Registry, t typ.Type) product.Value {
	return typevalue.WithWitness(reg, typevalue.FromType(reg, t), t)
}

func instantiateSignatureForCall(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	expressionRefinements map[factflow.ExprRef]factflow.ExpressionRefinement,
	argumentType SignatureArgumentTypeFunc,
	sig signature.Function,
	site factflow.CallSite,
	in state.State,
	read func(cfg.Point) state.State,
	returnTypeOps ReturnTypeOps,
) signature.Function {
	if sig.Type == nil || len(sig.Type.TypeParams) == 0 || returnTypeOps.InstantiateGenericCall == nil ||
		(sources == nil && argumentType == nil) {
		return sig
	}
	args, ok := signatureCallArgumentTypes(ctx, facts, sources, expressionRefinements, argumentType, site, in, read)
	if !ok {
		return sig
	}
	instantiated, ok := returnTypeOps.InstantiateGenericCall(sig.Type, args)
	if !ok || instantiated == nil {
		return sig
	}
	sig.Type = instantiated
	return sig
}

func signatureCallArgumentTypes(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	expressionRefinements map[factflow.ExprRef]factflow.ExpressionRefinement,
	argumentType SignatureArgumentTypeFunc,
	site factflow.CallSite,
	in state.State,
	read func(cfg.Point) state.State,
) ([]typ.Type, bool) {
	if factSite, ok := facts.CallSite(ctx.Point); ok {
		site = factSite
	}
	argSources := site.ArgumentSources()
	if len(argSources) == 0 {
		return nil, false
	}
	resolver := sourcevalue.WithExpressionRefinements(ctx.Registry, sources, expressionRefinements)
	args := make([]typ.Type, len(argSources))
	seen := false
	for i, source := range argSources {
		t, ok := signatureCallArgumentType(ctx, source, in, read, argumentType, resolver)
		if !ok {
			continue
		}
		args[i] = t
		seen = true
	}
	return args, seen
}

func signatureCallArgumentType(
	ctx transfer.NodeContext,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
	argumentType SignatureArgumentTypeFunc,
	resolver sourcevalue.SourceValues,
) (typ.Type, bool) {
	if argumentType != nil {
		if t, ok := argumentType(ctx, source, in, read); ok {
			return t, true
		}
	}
	if resolver == nil {
		return nil, false
	}
	value, ok := resolver.ValueOfSource(ctx.Point, source, in, read)
	if !ok {
		return nil, false
	}
	return typevalue.TypeOf(ctx.Registry, value)
}

func activeReturnTransform(sig signature.Function, index int) (returns.ReturnType, bool) {
	for _, label := range sig.Effect.Labels {
		ret, ok := effect.NormalizeLabel(label).(returns.Return)
		if !ok || ret.ReturnIndex != index {
			continue
		}
		if !operationalReturnTransform(ret.Transform) {
			continue
		}
		if returnTransformNilPointer(ret.Transform) {
			continue
		}
		return ret.Transform, true
	}
	return nil, false
}

func operationalReturnTransform(transform returns.ReturnType) bool {
	desc, ok := caplabel.DescriptorForReturnTransform(transform)
	return ok && desc.Status == capability.StatusOperational
}

func returnTransformNilPointer(transform returns.ReturnType) bool {
	if transform == nil {
		return true
	}
	v := reflect.ValueOf(transform)
	return v.Kind() == reflect.Pointer && v.IsNil()
}
