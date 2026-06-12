// Package callresult bridges summary values into factflow call results.
package callresult

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factflow/apply"
	"github.com/wippyai/go-lua/analysis/engine/factflow/source"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// KeyFunc maps one call producer in context to an exact summary key.
type KeyFunc func(ctx transfer.NodeContext, call factflow.CallProducer) (summary.SummaryKey, bool)

// NameFunc maps one call producer in context to a stable signature name.
type NameFunc func(ctx transfer.NodeContext, call factflow.CallProducer) (string, bool)

// SignatureLookup is the bounded read view required for signature-backed call
// results.
type SignatureLookup interface {
	Lookup(name string) (signature.Function, bool)
}

// SignatureProviderConfig carries the signature/effect lookup plus the generic
// fact/source read models needed to resolve call argument values.
type SignatureProviderConfig struct {
	Signatures SignatureLookup
	NameFor    NameFunc
	Facts      factflow.Facts
	Sources    source.SourceValues
}

// Provider returns a factflow call-result provider backed by exact summary reads.
func Provider(summaries summary.Reader, keyFor KeyFunc) apply.CallResultProvider {
	return func(ctx transfer.NodeContext, call factflow.CallProducer, _ state.State, _ func(cfg.Point) state.State) []apply.CallResult {
		if summaries == nil || keyFor == nil {
			return nil
		}
		key, ok := keyFor(ctx, call)
		if !ok {
			return nil
		}
		got, ok := summaries.Read(key)
		if !ok || len(got.Returns) == 0 {
			return nil
		}
		results := make([]apply.CallResult, len(got.Returns))
		for i, value := range got.Returns {
			results[i] = apply.CallResult{Index: i, Value: value}
		}
		return results
	}
}

// SignatureProvider materializes declared signature return types into call
// return slots.
func SignatureProvider(config SignatureProviderConfig) apply.CallResultProvider {
	signatures := config.Signatures
	nameFor := config.NameFor
	facts := config.Facts
	sources := config.Sources
	return func(ctx transfer.NodeContext, call factflow.CallProducer, in state.State, read func(cfg.Point) state.State) []apply.CallResult {
		if signatures == nil || nameFor == nil {
			return nil
		}
		name, ok := nameFor(ctx, call)
		if !ok {
			return nil
		}
		sig, ok := signatures.Lookup(name)
		if !ok || sig.Type == nil || len(sig.Type.Returns) == 0 {
			return nil
		}
		results := make([]apply.CallResult, 0, len(sig.Type.Returns))
		for i, ret := range sig.Type.Returns {
			value, ok := signatureReturnValue(ctx, facts, sources, sig, i, in, read)
			if !ok && ret != nil {
				value, ok = valueFromType(ctx.Registry, ret), true
			}
			if !ok {
				continue
			}
			results = append(results, apply.CallResult{
				Index: i,
				Value: value,
			})
		}
		return results
	}
}

func signatureReturnValue(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sources source.SourceValues,
	sig signature.Function,
	index int,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	transform, ok := returnTransform(sig, index)
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
		return elementOfReturnValue(ctx, facts, sig, transform.Source)
	case *returns.OptionalElementOf:
		if transform == nil {
			return product.Value{}, false
		}
		return elementOfReturnValue(ctx, facts, sig, transform.Source)
	default:
		return product.Value{}, false
	}
}

func sameAsReturnValue(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sources source.SourceValues,
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
	return source.WithValueOverlays(ctx.Registry, sources, facts.ValueOverlays()).ValueOfSource(ctx.Point, args[argIndex], in, read)
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
	return valueFromType(ctx.Registry, elem), true
}

func returnTransform(sig signature.Function, index int) (returns.ReturnType, bool) {
	for _, label := range sig.Effect.Labels {
		ret, ok := effect.NormalizeLabel(label).(returns.Return)
		if !ok || ret.ReturnIndex != index {
			continue
		}
		switch transform := ret.Transform.(type) {
		case returns.SameAs, returns.ElementOf, returns.OptionalElementOf:
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

// Fallback composes two call result providers. Primary results win by index;
// fallback results fill only missing result slots.
func Fallback(primary, fallback apply.CallResultProvider) apply.CallResultProvider {
	if primary == nil {
		return fallback
	}
	if fallback == nil {
		return primary
	}
	return func(ctx transfer.NodeContext, call factflow.CallProducer, in state.State, read func(cfg.Point) state.State) []apply.CallResult {
		first := primary(ctx, call, in, read)
		second := fallback(ctx, call, in, read)
		if len(first) == 0 {
			return second
		}
		if len(second) == 0 {
			return first
		}
		seen := make(map[int]struct{}, len(first))
		out := append([]apply.CallResult(nil), first...)
		for _, result := range first {
			seen[result.Index] = struct{}{}
		}
		for _, result := range second {
			if _, ok := seen[result.Index]; ok {
				continue
			}
			out = append(out, result)
		}
		return out
	}
}

// StaticName resolves one known stable name. It is useful in tests and small
// static compositions.
func StaticName(name string) NameFunc {
	name = strings.TrimSpace(name)
	return func(transfer.NodeContext, factflow.CallProducer) (string, bool) {
		return name, name != ""
	}
}

// ByCalleeSymbol maps callee symbol IDs to exact summary keys.
func ByCalleeSymbol(keys map[symbol.ID]summary.SummaryKey) KeyFunc {
	cloned := make(map[symbol.ID]summary.SummaryKey, len(keys))
	for id, key := range keys {
		cloned[id] = key
	}
	return func(_ transfer.NodeContext, call factflow.CallProducer) (summary.SummaryKey, bool) {
		key, ok := cloned[call.CalleeSymbol()]
		return key, ok
	}
}
