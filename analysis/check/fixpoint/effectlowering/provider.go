// Package effectlowering lowers declared signature effects into factflow facts
// and factapply call outcomes.
package effectlowering

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// SignatureNameFunc maps one call producer in context to a stable signature name.
type SignatureNameFunc func(ctx transfer.NodeContext, call factflow.CallProducer) (string, bool)

// SignatureLookup is the bounded read view required for signature-backed call
// results.
type SignatureLookup interface {
	Lookup(name string) (signature.Function, bool)
}

// SignatureOutcomeProviderConfig carries the signature/effect lookup plus the generic
// fact/source read models needed to resolve call argument values.
type SignatureOutcomeProviderConfig struct {
	Signatures SignatureLookup
	NameFor    SignatureNameFunc
	Facts      factflow.Facts
	Sources    sourcevalue.SourceValues
}

// SignatureOutcomeProvider materializes declared signature return types into
// call outcome return slots.
func SignatureOutcomeProvider(config SignatureOutcomeProviderConfig) factapply.CallOutcomeProvider {
	signatures := config.Signatures
	nameFor := config.NameFor
	facts := config.Facts
	sources := config.Sources
	return func(ctx transfer.NodeContext, site factflow.CallSite, in state.State, read func(cfg.Point) state.State) factapply.CallOutcome {
		if signatures == nil || nameFor == nil {
			return factapply.CallOutcome{}
		}
		call := factflow.CallProducerFromSite(site)
		name, ok := nameFor(ctx, call)
		if !ok {
			return factapply.CallOutcome{}
		}
		sig, ok := signatures.Lookup(name)
		if !ok {
			return factapply.CallOutcome{}
		}
		out := factapply.CallOutcome{
			ReturnPresenceRelations: signatureReturnPresenceRelations(sig),
			ParamPathRefinements:    signatureParamPathRefinements(ctx, sig, site),
		}
		if sig.Type == nil || len(sig.Type.Returns) == 0 {
			return out
		}
		results := make([]factapply.CallResult, 0, len(sig.Type.Returns))
		for i, ret := range sig.Type.Returns {
			value, ok := signatureReturnValue(ctx, facts, sources, sig, i, in, read)
			if !ok && ret != nil {
				value, ok = typevalue.FromType(ctx.Registry, ret), true
			}
			if !ok {
				continue
			}
			results = append(results, factapply.CallResult{
				Index: i,
				Value: value,
			})
		}
		out.Results = results
		return out
	}
}

func signatureParamPathRefinements(
	ctx transfer.NodeContext,
	sig signature.Function,
	site factflow.CallSite,
) []factapply.CallParamPathRefinement {
	labels := activeNormalReturnRefinementLabels(sig)
	if len(labels) == 0 {
		return nil
	}
	args := site.ArgumentSources()
	out := make([]factapply.CallParamPathRefinement, 0, len(labels))
	for _, label := range labels {
		argIndex, ok := effect.ResolveParamIndex(label.Target, len(args))
		if !ok || !callArgumentSourceCanBindPath(args[argIndex]) {
			continue
		}
		value, ok := signaturePostconditionValue(ctx, label.Refinement)
		if !ok {
			continue
		}
		out = append(out, factapply.CallParamPathRefinement{
			Path:  pathdom.NewPlaceholder(argIndex),
			Value: value,
		})
	}
	return out
}

func callArgumentSourceCanBindPath(source factflow.ValueSource) bool {
	return source.Kind == factflow.ValueSourceExpression && source.HasExpr
}

func activeNormalReturnRefinementLabels(sig signature.Function) []postcondition.NormalReturnRefinement {
	if len(sig.Effect.Labels) == 0 {
		return nil
	}
	out := make([]postcondition.NormalReturnRefinement, 0, len(sig.Effect.Labels))
	for _, label := range sig.Effect.Labels {
		switch normalized := effect.NormalizeLabel(label).(type) {
		case postcondition.NormalReturnRefinement:
			out = append(out, normalized)
		case *postcondition.NormalReturnRefinement:
			if normalized != nil {
				out = append(out, *normalized)
			}
		}
	}
	return out
}

func signaturePostconditionValue(ctx transfer.NodeContext, refinement postcondition.Refinement) (product.Value, bool) {
	if ctx.Registry == nil {
		return product.Value{}, false
	}
	switch r := refinement.(type) {
	case postcondition.Present:
		return product.NewWithPresence(ctx.Registry, product.ShapeTop, presence.Present()), true
	case *postcondition.Present:
		if r != nil {
			return product.NewWithPresence(ctx.Registry, product.ShapeTop, presence.Present()), true
		}
	}
	return product.Value{}, false
}

func signatureReturnPresenceRelations(sig signature.Function) []factapply.CallReturnPresenceRelation {
	labels := activeErrorReturnLabels(sig)
	if len(labels) == 0 {
		return nil
	}
	out := make([]factapply.CallReturnPresenceRelation, 0, len(labels)*2)
	for _, label := range labels {
		if label.ValueIndex < 0 || label.ErrorIndex < 0 {
			continue
		}
		out = append(out,
			factapply.CallReturnPresenceRelation{
				TriggerIndex:    label.ErrorIndex,
				TriggerPresence: presence.Present(),
				TargetIndex:     label.ValueIndex,
				TargetPresence:  presence.Absent(),
			},
			factapply.CallReturnPresenceRelation{
				TriggerIndex:    label.ErrorIndex,
				TriggerPresence: presence.Absent(),
				TargetIndex:     label.ValueIndex,
				TargetPresence:  presence.Present(),
			},
		)
	}
	return out
}

func activeErrorReturnLabels(sig signature.Function) []returns.ErrorReturn {
	if len(sig.Effect.Labels) == 0 {
		return nil
	}
	out := make([]returns.ErrorReturn, 0, len(sig.Effect.Labels))
	for _, label := range sig.Effect.Labels {
		switch normalized := effect.NormalizeLabel(label).(type) {
		case returns.ErrorReturn:
			out = append(out, normalized)
		case *returns.ErrorReturn:
			if normalized != nil {
				out = append(out, *normalized)
			}
		}
	}
	return out
}

func signatureReturnValue(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	sig signature.Function,
	index int,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	transform, ok := activeReturnTransform(sig, index)
	if !ok {
		return product.Value{}, false
	}
	switch transform := transform.(type) {
	case returns.SameAs:
		return sameAsReturnValue(ctx, facts, sources, transform.Source, in, read)
	case *returns.SameAs:
		if transform == nil {
			return product.Value{}, false
		}
		return sameAsReturnValue(ctx, facts, sources, transform.Source, in, read)
	case returns.ElementOf:
		return elementOfReturnValue(ctx, facts, sig, transform.Source)
	case *returns.ElementOf:
		if transform == nil {
			return product.Value{}, false
		}
		return elementOfReturnValue(ctx, facts, sig, transform.Source)
	case returns.OptionalElementOf:
		return optionalElementOfReturnValue(ctx, facts, sig, transform.Source)
	case *returns.OptionalElementOf:
		if transform == nil {
			return product.Value{}, false
		}
		return optionalElementOfReturnValue(ctx, facts, sig, transform.Source)
	case returns.CallbackReturn:
		return callbackReturnValue(ctx, facts, sig, transform.CallbackParam, false)
	case *returns.CallbackReturn:
		if transform == nil {
			return product.Value{}, false
		}
		return callbackReturnValue(ctx, facts, sig, transform.CallbackParam, false)
	case returns.ArrayOfCallbackReturn:
		return callbackReturnValue(ctx, facts, sig, transform.CallbackParam, true)
	case *returns.ArrayOfCallbackReturn:
		if transform == nil {
			return product.Value{}, false
		}
		return callbackReturnValue(ctx, facts, sig, transform.CallbackParam, true)
	case returns.TypeProjection:
		return typeProjectionReturnValue(ctx, facts, sig, transform)
	case *returns.TypeProjection:
		if transform == nil {
			return product.Value{}, false
		}
		return typeProjectionReturnValue(ctx, facts, sig, *transform)
	default:
		return product.Value{}, false
	}
}

func sameAsReturnValue(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
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
	return sourcevalue.WithExpressionRefinements(ctx.Registry, sources, facts.ExpressionRefinements()).ValueOfSource(ctx.Point, args[argIndex], in, read)
}

func elementOfReturnValue(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sig signature.Function,
	ref effect.ParamRef,
) (product.Value, bool) {
	site, ok := facts.CallSite(ctx.Point)
	if !ok {
		return product.Value{}, false
	}
	args := site.ArgumentSources()
	argIndex, ok := effect.ResolveParamIndex(ref, len(args))
	if !ok || sig.Type == nil || argIndex < 0 || argIndex >= len(sig.Type.Params) {
		return product.Value{}, false
	}
	elem, ok := elementTypeOf(sig.Type.Params[argIndex].Type)
	if !ok {
		return product.Value{}, false
	}
	return typevalue.FromType(ctx.Registry, elem), true
}

func optionalElementOfReturnValue(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sig signature.Function,
	ref effect.ParamRef,
) (product.Value, bool) {
	value, ok := elementOfReturnValue(ctx, facts, sig, ref)
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
) (product.Value, bool) {
	site, ok := facts.CallSite(ctx.Point)
	if !ok {
		return product.Value{}, false
	}
	args := site.ArgumentSources()
	argIndex, ok := effect.ResolveParamIndex(ref, len(args))
	if !ok || sig.Type == nil || argIndex < 0 || argIndex >= len(sig.Type.Params) {
		return product.Value{}, false
	}
	ret, ok := typecall.CallableReturn(sig.Type.Params[argIndex].Type)
	if !ok {
		return product.Value{}, false
	}
	if array {
		ret = typ.NewArray(ret)
	}
	return typevalue.FromType(ctx.Registry, ret), true
}

func typeProjectionReturnValue(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sig signature.Function,
	transform returns.TypeProjection,
) (product.Value, bool) {
	site, ok := facts.CallSite(ctx.Point)
	if !ok {
		return product.Value{}, false
	}
	args := site.ArgumentSources()
	argIndex, ok := effect.ResolveParamIndex(transform.Source, len(args))
	if !ok || sig.Type == nil || argIndex < 0 || argIndex >= len(sig.Type.Params) {
		return product.Value{}, false
	}
	projected, ok := typeprojection.Apply(sig.Type.Params[argIndex].Type, transform.Projection)
	if !ok {
		return product.Value{}, false
	}
	return typevalue.FromType(ctx.Registry, projected), true
}

func activeReturnTransform(sig signature.Function, index int) (returns.ReturnType, bool) {
	for _, label := range sig.Effect.Labels {
		ret, ok := effect.NormalizeLabel(label).(returns.Return)
		if !ok || ret.ReturnIndex != index {
			continue
		}
		switch transform := ret.Transform.(type) {
		case returns.SameAs, returns.ElementOf, returns.OptionalElementOf, returns.CallbackReturn, returns.ArrayOfCallbackReturn, returns.TypeProjection:
			return ret.Transform, true
		case *returns.SameAs:
			if transform != nil {
				return transform, true
			}
		case *returns.ElementOf:
			if transform != nil {
				return transform, true
			}
		case *returns.OptionalElementOf:
			if transform != nil {
				return transform, true
			}
		case *returns.CallbackReturn:
			if transform != nil {
				return transform, true
			}
		case *returns.ArrayOfCallbackReturn:
			if transform != nil {
				return transform, true
			}
		case *returns.TypeProjection:
			if transform != nil {
				return transform, true
			}
		}
	}
	return nil, false
}

func elementTypeOf(t typ.Type) (typ.Type, bool) {
	return elementTypeOfDepth(t, 0)
}

func elementTypeOfDepth(t typ.Type, depth int) (typ.Type, bool) {
	if depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	t = typ.NormalizeNilType(t)
	if t == nil {
		return nil, false
	}
	switch tt := t.(type) {
	case *typ.Annotated:
		return elementTypeOfDepth(tt.Inner, depth+1)
	case *typ.Alias:
		return elementTypeOfDepth(tt.UnaliasedTarget(), depth+1)
	case *typ.Optional:
		return elementTypeOfDepth(tt.Inner, depth+1)
	case *typ.Array:
		if typ.NormalizeNilType(tt.Element) == nil {
			return nil, false
		}
		return tt.Element, true
	case *typ.Map:
		if typ.NormalizeNilType(tt.Value) == nil {
			return nil, false
		}
		return tt.Value, true
	case *typ.ReadonlyMap:
		if typ.NormalizeNilType(tt.Value) == nil {
			return nil, false
		}
		return tt.Value, true
	case *typ.Tuple:
		if len(tt.Elements) == 0 {
			return nil, false
		}
		if len(tt.Elements) == 1 {
			if typ.NormalizeNilType(tt.Elements[0]) == nil {
				return nil, false
			}
			return tt.Elements[0], true
		}
		return typ.NewUnion(tt.Elements...), true
	case *typ.Union:
		members := make([]typ.Type, 0, len(tt.Members))
		for _, member := range tt.Members {
			member = typ.NormalizeNilType(member)
			if member == nil {
				continue
			}
			if member.Kind() == kind.Nil {
				continue
			}
			elem, ok := elementTypeOfDepth(member, depth+1)
			if !ok {
				return nil, false
			}
			members = append(members, elem)
		}
		if len(members) == 0 {
			return nil, false
		}
		return typ.NewUnion(members...), true
	default:
		return nil, false
	}
}

// StaticName resolves one known stable name. It is useful in tests and small
// static compositions.
func StaticName(name string) SignatureNameFunc {
	name = strings.TrimSpace(name)
	return func(transfer.NodeContext, factflow.CallProducer) (string, bool) {
		return name, name != ""
	}
}
