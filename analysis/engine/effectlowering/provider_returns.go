package effectlowering

import (
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
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ReturnTypeOps carries caller-owned type operations needed by return
// transforms that are not part of engine ownership.
type ReturnTypeOps struct {
	CallableReturn         func(typ.Type) (typ.Type, bool)
	ElementOf              func(typ.Type) (typ.Type, bool)
	TypeProjection         func(typ.Type, projection.Projection) (typ.Type, bool)
	InstantiateGenericCall func(*typ.Function, []typ.Type) (GenericCallInstantiation, bool)
}

// GenericCallInstantiation carries the function type and binding set produced
// by one generic call inference operation. The binding set is reused to
// substitute type-bearing signature sidecars, keeping function type and
// operational effects in lockstep.
type GenericCallInstantiation struct {
	Type       *typ.Function
	TypeParams []*typ.TypeParam
	TypeArgs   []typ.Type
}

func signatureReturnValue(
	ctx transfer.NodeContext,
	sources sourcevalue.SourceValues,
	expressionRefinements sourcevalue.ExpressionRefinements,
	sig signature.Function,
	index int,
	argSources signatureArgumentReader,
	in state.State,
	read func(cfg.Point) state.State,
	returnTypeOps ReturnTypeOps,
	typeValues *typevalue.Cache,
) (product.Value, bool) {
	transform, ok := activeReturnTransform(sig, index)
	if !ok {
		return product.Value{}, false
	}
	if transform, ok := returns.AsSameAs(transform); ok {
		return sameAsReturnValue(ctx, sources, expressionRefinements, transform.Source, argSources, in, read)
	}
	if transform, ok := returns.AsElementOf(transform); ok {
		return elementOfReturnValue(ctx, sig, transform.Source, argSources, returnTypeOps, typeValues)
	}
	if transform, ok := returns.AsOptionalElementOf(transform); ok {
		return optionalElementOfReturnValue(ctx, sig, transform.Source, argSources, returnTypeOps, typeValues)
	}
	if transform, ok := returns.AsCallbackReturn(transform); ok {
		return callbackReturnValue(ctx, sig, transform.CallbackParam, argSources, false, returnTypeOps, typeValues)
	}
	if transform, ok := returns.AsArrayOfCallbackReturn(transform); ok {
		return callbackReturnValue(ctx, sig, transform.CallbackParam, argSources, true, returnTypeOps, typeValues)
	}
	if transform, ok := returns.AsTypeProjection(transform); ok {
		return typeProjectionReturnValue(ctx, sig, transform, argSources, returnTypeOps, typeValues)
	}
	if transform, ok := returns.AsConditionalType(transform); ok {
		return conditionalTypeReturnValue(ctx, sources, expressionRefinements, sig, transform, argSources, in, read, returnTypeOps, typeValues)
	}
	return product.Value{}, false
}

func sameAsReturnValue(
	ctx transfer.NodeContext,
	sources sourcevalue.SourceValues,
	expressionRefinements sourcevalue.ExpressionRefinements,
	ref effect.ParamRef,
	argSources signatureArgumentReader,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	if sources == nil {
		return product.Value{}, false
	}
	argIndex, ok := effect.ResolveParamIndex(ref, argSources.ArgumentSourceCount())
	if !ok {
		return product.Value{}, false
	}
	source, ok := argSources.ArgumentSourceAt(argIndex)
	if !ok {
		return product.Value{}, false
	}
	return expressionRefinements.Bind(ctx.Registry, sources).ValueOfSource(ctx.Point, source, in, read)
}

func elementOfReturnValue(
	ctx transfer.NodeContext,
	sig signature.Function,
	ref effect.ParamRef,
	argSources signatureArgumentReader,
	returnTypeOps ReturnTypeOps,
	typeValues *typevalue.Cache,
) (product.Value, bool) {
	if returnTypeOps.ElementOf == nil {
		return product.Value{}, false
	}
	argIndex, ok := effect.ResolveParamIndex(ref, argSources.ArgumentSourceCount())
	if !ok || sig.Type == nil || argIndex < 0 || argIndex >= len(sig.Type.Params) {
		return product.Value{}, false
	}
	elem, ok := returnTypeOps.ElementOf(sig.Type.Params[argIndex].Type)
	if !ok {
		return product.Value{}, false
	}
	return returnValueFromSignatureTypeCached(ctx.Registry, typeValues, sig.Type, elem), true
}

func optionalElementOfReturnValue(
	ctx transfer.NodeContext,
	sig signature.Function,
	ref effect.ParamRef,
	argSources signatureArgumentReader,
	returnTypeOps ReturnTypeOps,
	typeValues *typevalue.Cache,
) (product.Value, bool) {
	value, ok := elementOfReturnValue(ctx, sig, ref, argSources, returnTypeOps, typeValues)
	if !ok {
		return product.Value{}, false
	}
	return product.WithPresence(ctx.Registry, value, presence.Maybe()), true
}

func callbackReturnValue(
	ctx transfer.NodeContext,
	sig signature.Function,
	ref effect.ParamRef,
	argSources signatureArgumentReader,
	array bool,
	returnTypeOps ReturnTypeOps,
	typeValues *typevalue.Cache,
) (product.Value, bool) {
	if returnTypeOps.CallableReturn == nil {
		return product.Value{}, false
	}
	argIndex, ok := effect.ResolveParamIndex(ref, argSources.ArgumentSourceCount())
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
	return returnValueFromSignatureTypeCached(ctx.Registry, typeValues, sig.Type, ret), true
}

func typeProjectionReturnValue(
	ctx transfer.NodeContext,
	sig signature.Function,
	transform returns.TypeProjection,
	argSources signatureArgumentReader,
	returnTypeOps ReturnTypeOps,
	typeValues *typevalue.Cache,
) (product.Value, bool) {
	if returnTypeOps.TypeProjection == nil {
		return product.Value{}, false
	}
	argIndex, ok := effect.ResolveParamIndex(transform.Source, argSources.ArgumentSourceCount())
	if !ok || sig.Type == nil || argIndex < 0 || argIndex >= len(sig.Type.Params) {
		return product.Value{}, false
	}
	projected, ok := returnTypeOps.TypeProjection(sig.Type.Params[argIndex].Type, transform.Projection)
	if !ok {
		return product.Value{}, false
	}
	return returnValueFromSignatureTypeCached(ctx.Registry, typeValues, sig.Type, projected), true
}

func conditionalTypeReturnValue(
	ctx transfer.NodeContext,
	sources sourcevalue.SourceValues,
	expressionRefinements sourcevalue.ExpressionRefinements,
	sig signature.Function,
	transform returns.ConditionalType,
	argSources signatureArgumentReader,
	in state.State,
	read func(cfg.Point) state.State,
	returnTypeOps ReturnTypeOps,
	typeValues *typevalue.Cache,
) (product.Value, bool) {
	if returnTypeOps.TypeProjection == nil || sources == nil || transform.When == nil || transform.Then == nil {
		return product.Value{}, false
	}
	argIndex, ok := effect.ResolveParamIndex(transform.Source, argSources.ArgumentSourceCount())
	if !ok {
		return product.Value{}, false
	}
	source, ok := argSources.ArgumentSourceAt(argIndex)
	if !ok {
		return product.Value{}, false
	}
	value, ok := expressionRefinements.Bind(ctx.Registry, sources).ValueOfSource(ctx.Point, source, in, read)
	if !ok {
		return product.Value{}, false
	}
	actual, ok := typevalue.TypeOf(ctx.Registry, value)
	if !ok {
		return product.Value{}, false
	}
	projected, ok := returnTypeOps.TypeProjection(actual, transform.Projection)
	if !ok || projected == nil || !subtype.IsSubtype(projected, transform.When) {
		return product.Value{}, false
	}
	return returnValueFromSignatureTypeCached(ctx.Registry, typeValues, sig.Type, transform.Then), true
}

func returnValueFromType(reg *axis.Registry, t typ.Type) product.Value {
	return typevalue.WithWitness(reg, typevalue.FromType(reg, t), t)
}

func returnValueFromTypeCached(reg *axis.Registry, typeValues *typevalue.Cache, t typ.Type) product.Value {
	if typeValues != nil {
		return typeValues.FromTypeWithWitness(reg, t)
	}
	return returnValueFromType(reg, t)
}

func returnValueFromSignatureType(reg *axis.Registry, fn *typ.Function, t typ.Type) product.Value {
	return returnValueFromType(reg, closeUninferredSignatureTypeParams(fn, t))
}

func returnValueFromSignatureTypeCached(reg *axis.Registry, typeValues *typevalue.Cache, fn *typ.Function, t typ.Type) product.Value {
	return returnValueFromTypeCached(reg, typeValues, closeUninferredSignatureTypeParams(fn, t))
}

func closeUninferredSignatureTypeParams(fn *typ.Function, t typ.Type) typ.Type {
	if fn == nil || len(fn.TypeParams) == 0 || t == nil {
		return t
	}
	params := make([]*typ.TypeParam, 0, len(fn.TypeParams))
	args := make([]typ.Type, 0, len(fn.TypeParams))
	for _, param := range fn.TypeParams {
		if param == nil {
			continue
		}
		replacement := param.Constraint
		if replacement == nil {
			replacement = typ.Unknown
		}
		params = append(params, param)
		args = append(args, replacement)
	}
	if len(params) == 0 {
		return t
	}
	return subst.Params(t, params, args)
}

func instantiateSignatureForCall(
	ctx transfer.NodeContext,
	sources sourcevalue.SourceValues,
	expressionRefinements sourcevalue.ExpressionRefinements,
	argumentType SignatureArgumentTypeFunc,
	sig signature.Function,
	argSources signatureArgumentReader,
	in state.State,
	read func(cfg.Point) state.State,
	returnTypeOps ReturnTypeOps,
) signature.Function {
	if sig.Type == nil || len(sig.Type.TypeParams) == 0 || returnTypeOps.InstantiateGenericCall == nil ||
		(sources == nil && argumentType == nil) {
		return sig
	}
	args, ok := signatureCallArgumentTypes(ctx, sources, expressionRefinements, argumentType, argSources, in, read)
	if !ok {
		return sig
	}
	instantiated, ok := returnTypeOps.InstantiateGenericCall(sig.Type, args)
	if !ok || instantiated.Type == nil {
		return sig
	}
	sig.Type = instantiated.Type
	sig.OperationalEffects = signature.SubstituteOperationalTypes(sig.OperationalEffects, instantiated.TypeParams, instantiated.TypeArgs)
	return sig
}

func signatureCallArgumentTypes(
	ctx transfer.NodeContext,
	sources sourcevalue.SourceValues,
	expressionRefinements sourcevalue.ExpressionRefinements,
	argumentType SignatureArgumentTypeFunc,
	argSources signatureArgumentReader,
	in state.State,
	read func(cfg.Point) state.State,
) ([]typ.Type, bool) {
	count := argSources.ArgumentSourceCount()
	if count == 0 {
		return nil, false
	}
	resolver := expressionRefinements.Bind(ctx.Registry, sources)
	args := make([]typ.Type, count)
	seen := false
	argSources.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		t, ok := signatureCallArgumentType(ctx, source, in, read, argumentType, resolver)
		if !ok {
			return true
		}
		args[i] = t
		seen = true
		return true
	})
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
		if returns.IsNilReturnType(ret.Transform) {
			continue
		}
		if operationalReturnTransform(ret.Transform) {
			return ret.Transform, true
		}
	}
	return nil, false
}

func operationalReturnTransform(transform returns.ReturnType) bool {
	desc, ok := caplabel.DescriptorForReturnTransform(transform)
	return ok && desc.Status == capability.StatusOperational
}
