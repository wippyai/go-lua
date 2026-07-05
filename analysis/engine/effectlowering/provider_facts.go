package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

type signatureArgumentReader struct {
	site              factflow.CallSiteView
	receiverSource    factflow.ValueSource
	hasReceiverSource bool
	keySpace          *keyspace.KeySpace
}

func signatureArgumentsFromView(site factflow.CallSiteView) signatureArgumentReader {
	return signatureArgumentReader{site: site}
}

func signatureArgumentsFromViewWithKeySpace(site factflow.CallSiteView, ks *keyspace.KeySpace) signatureArgumentReader {
	return signatureArgumentReader{site: site, keySpace: ks}
}

func signatureArgumentsFromMethodView(site factflow.CallSiteView) signatureArgumentReader {
	receiver, ok := site.ReceiverSource()
	if !ok {
		return signatureArgumentsFromView(site)
	}
	return signatureArgumentReader{
		site:              site,
		receiverSource:    receiver,
		hasReceiverSource: true,
	}
}

func signatureArgumentsFromMethodViewWithKeySpace(site factflow.CallSiteView, ks *keyspace.KeySpace) signatureArgumentReader {
	args := signatureArgumentsFromMethodView(site)
	args.keySpace = ks
	return args
}

func (r signatureArgumentReader) ArgumentSourceCount() int {
	if r.hasReceiverSource {
		return r.site.ArgumentSourceCount() + 1
	}
	return r.site.ArgumentSourceCount()
}

func (r signatureArgumentReader) ArgumentSourceAt(index int) (factflow.ValueSource, bool) {
	if r.hasReceiverSource {
		if index == 0 {
			return r.receiverSource, true
		}
		return r.site.ArgumentSourceAt(index - 1)
	}
	return r.site.ArgumentSourceAt(index)
}

func (r signatureArgumentReader) ForEachArgumentSource(fn func(index int, source factflow.ValueSource) bool) {
	if r.hasReceiverSource {
		if !fn(0, r.receiverSource) {
			return
		}
		r.site.ForEachArgumentSource(func(index int, source factflow.ValueSource) bool {
			return fn(index+1, source)
		})
		return
	}
	r.site.ForEachArgumentSource(fn)
}

func signatureParamPathInvalidationsForReader(sig signature.Function, args signatureArgumentReader) []callpayload.CallParamPathInvalidation {
	targets := activePathInvalidationTargets(sig)
	if len(targets) == 0 {
		return nil
	}
	var out []callpayload.CallParamPathInvalidation
	seen := make(map[int]struct{}, len(targets))
	for _, target := range targets {
		argIndex, ok := effect.ResolveParamIndex(target.Ref, args.ArgumentSourceCount())
		source, sourceOK := args.ArgumentSourceAt(argIndex)
		if !ok || !sourceOK || !args.callArgumentSourceCanBindPath(source) {
			continue
		}
		if _, ok := seen[argIndex]; ok {
			if !target.PreserveStructuralWitness {
				for i := range out {
					if out[i].Path.PlaceholderIndex() == argIndex {
						out[i].PreserveStructuralWitness = false
					}
				}
			}
			continue
		}
		seen[argIndex] = struct{}{}
		out = append(out, callpayload.CallParamPathInvalidation{
			Path:                      pathdom.NewPlaceholder(argIndex),
			PreserveStructuralWitness: target.PreserveStructuralWitness,
		})
	}
	return out
}

func signatureNormalReturnPathInvalidations(in []callpayload.CallParamPathInvalidation) []callboundary.PathInvalidationFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.PathInvalidationFact, 0, len(in))
	for _, fact := range in {
		out = append(out, callboundary.PathInvalidationFact{
			Path:                      fact.Path,
			PreserveStructuralWitness: fact.PreserveStructuralWitness,
		})
	}
	return out
}

func signatureParamLengthFloorsForReader(sig signature.Function, args signatureArgumentReader) []callpayload.CallParamLengthFloor {
	if len(sig.Effect.Labels) == 0 {
		return nil
	}
	var out []callpayload.CallParamLengthFloor
	for _, label := range sig.Effect.Labels {
		target, delta, ok := mutation.PositiveLengthFloor(label)
		if !ok {
			continue
		}
		argIndex, ok := effect.ResolveParamIndex(target, args.ArgumentSourceCount())
		source, sourceOK := args.ArgumentSourceAt(argIndex)
		if !ok || !sourceOK || !args.callArgumentSourceCanBindPath(source) {
			continue
		}
		out = append(out, callpayload.CallParamLengthFloor{
			Path:  pathdom.NewPlaceholder(argIndex),
			Floor: int64(delta),
		})
	}
	return out
}

type pathInvalidationTarget struct {
	Ref                       effect.ParamRef
	PreserveStructuralWitness bool
}

func activePathInvalidationTargets(sig signature.Function) []pathInvalidationTarget {
	if len(sig.Effect.Labels) == 0 {
		return nil
	}
	out := make([]pathInvalidationTarget, 0, len(sig.Effect.Labels))
	for _, label := range sig.Effect.Labels {
		if target, ok := pathInvalidationTargetForLabel(label); ok {
			out = append(out, target)
			continue
		}
		switch normalized := effect.NormalizeLabel(label).(type) {
		case ownership.Store:
			if normalized.Into.Index >= 0 {
				out = append(out, pathInvalidationTarget{Ref: normalized.Into, PreserveStructuralWitness: true})
			}
		}
	}
	return out
}

func pathInvalidationTargetForLabel(label effect.Label) (pathInvalidationTarget, bool) {
	switch normalized := effect.NormalizeLabel(label).(type) {
	case mutation.Mutate:
		return pathInvalidationTarget{
			Ref:                       normalized.Target,
			PreserveStructuralWitness: mutationTransformPreservesStructuralWitness(normalized.Transform),
		}, true
	case mutation.TableMutator:
		return pathInvalidationTarget{Ref: normalized.Target, PreserveStructuralWitness: true}, true
	case mutation.LengthChange:
		return pathInvalidationTarget{Ref: normalized.Target, PreserveStructuralWitness: true}, true
	default:
		return pathInvalidationTarget{}, false
	}
}

func mutationTransformPreservesStructuralWitness(transform mutation.TypeTransform) bool {
	switch transform.(type) {
	case mutation.Unchanged, *mutation.Unchanged:
		return true
	default:
		return false
	}
}

func signatureEscapeEventsForReader(sig signature.Function, args signatureArgumentReader) []callboundary.EscapeEventFact {
	if len(sig.Effect.Labels) == 0 {
		return nil
	}
	out := make([]callboundary.EscapeEventFact, 0, len(sig.Effect.Labels))
	appendArg := func(index int, kind callboundary.EscapeEventKind) {
		source, ok := args.ArgumentSourceAt(index)
		if !ok || !args.callArgumentSourceCanBindPath(source) {
			return
		}
		out = append(out, callboundary.EscapeEventFact{
			Target:    pathdom.NewPlaceholder(index),
			Kind:      kind,
			Recursive: true,
		})
	}
	appendParam := func(ref effect.ParamRef, kind callboundary.EscapeEventKind) {
		index, ok := effect.ResolveParamIndex(ref, args.ArgumentSourceCount())
		if !ok {
			return
		}
		appendArg(index, kind)
	}
	for _, label := range sig.Effect.Labels {
		switch normalized := effect.NormalizeLabel(label).(type) {
		case ownership.Borrow:
			appendParam(normalized.Param, callboundary.EscapeEventBorrow)
		case ownership.BorrowAll:
			for i := 0; i < args.ArgumentSourceCount(); i++ {
				appendArg(i, callboundary.EscapeEventBorrow)
			}
		case ownership.Retain:
			appendParam(normalized.Param, callboundary.EscapeEventRetain)
		case ownership.Send:
			start, ok := effect.ResolveParamIndex(effect.ParamRef{Index: normalized.FromParam}, args.ArgumentSourceCount())
			if !ok {
				continue
			}
			for i := start; i < args.ArgumentSourceCount(); i++ {
				appendArg(i, callboundary.EscapeEventSend)
			}
		case ownership.SendParam:
			appendParam(normalized.Param, callboundary.EscapeEventSend)
		case ownership.Export:
			appendParam(normalized.Param, callboundary.EscapeEventExport)
		case ownership.Opaque:
			appendParam(normalized.Param, callboundary.EscapeEventOpaque)
		case ownership.Store:
			appendParam(normalized.Param, callboundary.EscapeEventStore)
		}
	}
	return out
}

func signatureFrozenTablesForReader(sig signature.Function, args signatureArgumentReader) []callboundary.FrozenTableFact {
	if len(sig.Effect.Labels) == 0 {
		return nil
	}
	out := make([]callboundary.FrozenTableFact, 0, len(sig.Effect.Labels))
	appendParam := func(ref effect.ParamRef) {
		index, ok := effect.ResolveParamIndex(ref, args.ArgumentSourceCount())
		source, sourceOK := args.ArgumentSourceAt(index)
		if !ok || !sourceOK || !args.callArgumentSourceCanBindPath(source) {
			return
		}
		out = append(out, callboundary.FrozenTableFact{
			Target: pathdom.NewPlaceholder(index),
		})
	}
	for _, label := range sig.Effect.Labels {
		switch normalized := effect.NormalizeLabel(label).(type) {
		case ownership.Freeze:
			appendParam(normalized.Param)
		}
	}
	return out
}

func signatureStoreRelationsForReader(sig signature.Function, args signatureArgumentReader) []callboundary.StoreRelationFact {
	if len(sig.Effect.Labels) == 0 {
		return nil
	}
	out := make([]callboundary.StoreRelationFact, 0, len(sig.Effect.Labels))
	appendStore := func(sourceRef, intoRef effect.ParamRef) {
		if intoRef.Index < 0 {
			return
		}
		source, ok := effect.ResolveParamIndex(sourceRef, args.ArgumentSourceCount())
		sourceValue, sourceOK := args.ArgumentSourceAt(source)
		if !ok || !sourceOK || !args.callArgumentSourceCanBindPath(sourceValue) {
			return
		}
		into, ok := effect.ResolveParamIndex(intoRef, args.ArgumentSourceCount())
		intoValue, intoOK := args.ArgumentSourceAt(into)
		if !ok || !intoOK || !args.callArgumentSourceCanBindPath(intoValue) {
			return
		}
		out = append(out, callboundary.StoreRelationFact{
			Source: pathdom.NewPlaceholder(source),
			Into:   pathdom.NewPlaceholder(into),
		})
	}
	for _, label := range sig.Effect.Labels {
		switch normalized := effect.NormalizeLabel(label).(type) {
		case ownership.Store:
			appendStore(normalized.Param, normalized.Into)
		}
	}
	return out
}

func signatureLifecycleFactsForReader(sig signature.Function, args signatureArgumentReader) []callboundary.LifecycleFact {
	if len(sig.Effect.Labels) == 0 {
		return nil
	}
	out := make([]callboundary.LifecycleFact, 0, len(sig.Effect.Labels))
	appendFact := func(ref effect.ParamRef, fact callboundary.LifecycleFact) {
		index, ok := effect.ResolveParamIndex(ref, args.ArgumentSourceCount())
		source, sourceOK := args.ArgumentSourceAt(index)
		if !ok || !sourceOK || !args.callArgumentSourceCanBindPath(source) {
			return
		}
		fact.Target = pathdom.NewPlaceholder(index)
		out = append(out, fact)
	}
	for _, label := range sig.Effect.Labels {
		switch normalized := effect.NormalizeLabel(label).(type) {
		case lifecycle.Acquire:
			appendFact(normalized.Target, callboundary.LifecycleFact{
				Kind:       callboundary.LifecycleAcquire,
				Protocol:   normalized.Protocol,
				To:         normalized.State,
				Obligation: normalized.Obligation,
			})
		case lifecycle.Transition:
			appendFact(normalized.Target, callboundary.LifecycleFact{
				Kind:     callboundary.LifecycleTransition,
				Protocol: normalized.Protocol,
				From:     normalized.From,
				To:       normalized.To,
			})
		case lifecycle.Escape:
			appendFact(normalized.Target, callboundary.LifecycleFact{
				Kind:     callboundary.LifecycleEscape,
				Protocol: normalized.Protocol,
			})
		}
	}
	return out
}

func signatureParamPathRefinementsForReader(
	ctx transfer.NodeContext,
	sig signature.Function,
	args signatureArgumentReader,
) []callpayload.CallParamPathRefinement {
	labels := activeNormalReturnRefinementLabels(sig)
	if len(labels) == 0 {
		return nil
	}
	out := make([]callpayload.CallParamPathRefinement, 0, len(labels))
	for _, label := range labels {
		argIndex, ok := effect.ResolveParamIndex(label.Target, args.ArgumentSourceCount())
		source, sourceOK := args.ArgumentSourceAt(argIndex)
		if !ok || !sourceOK || !args.callArgumentSourceCanBindPath(source) {
			continue
		}
		value, ok := signaturePostconditionValue(ctx, label.Refinement)
		if !ok {
			continue
		}
		out = append(out, callpayload.CallParamPathRefinement{
			Path:  pathdom.NewPlaceholder(argIndex),
			Value: value,
		})
	}
	return out
}

func (r signatureArgumentReader) callArgumentSourceCanBindPath(source factflow.ValueSource) bool {
	switch source.Kind {
	case factflow.ValueSourceExpression:
		return source.HasExpr
	case factflow.ValueSourcePath:
		_, ok := sourcePathFromPathKey(r.keySpace, source.PathKey)
		return ok
	default:
		return false
	}
}

func sourcePathFromPathKey(ks *keyspace.KeySpace, raw pathdom.PathKey) (pathdom.Path, bool) {
	if ks == nil || raw == "" {
		return pathdom.Path{}, false
	}
	key, ok := ks.FromStateKey(raw)
	if !ok || key.Sym == 0 {
		return pathdom.Path{}, false
	}
	return pathdom.Path{
		Symbol:   key.Sym,
		Segments: ks.Segments(key),
	}, true
}

func activeNormalReturnRefinementLabels(sig signature.Function) []postcondition.NormalReturnRefinement {
	return activeReturnLabels[postcondition.NormalReturnRefinement](sig)
}

// activeReturnLabels collects every effect label of sig that normalizes to T.
func activeReturnLabels[T any](sig signature.Function) []T {
	if len(sig.Effect.Labels) == 0 {
		return nil
	}
	out := make([]T, 0, len(sig.Effect.Labels))
	for _, label := range sig.Effect.Labels {
		if v, ok := any(effect.NormalizeLabel(label)).(T); ok {
			out = append(out, v)
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
	case postcondition.Absent:
		return product.Absent(ctx.Registry), true
	case *postcondition.Absent:
		if r != nil {
			return product.Absent(ctx.Registry), true
		}
	}
	return product.Value{}, false
}

func signatureReturnPresenceRelations(sig signature.Function) []callpayload.CallReturnPresenceRelation {
	return errorReturnPresenceRelations(activeErrorReturnLabels(sig))
}

// errorReturnPresenceRelations expands each value/error correlation into the
// anticorrelated presence relations the narrowing engine consumes. A proven
// error state determines the value state; additionally, for canonical
// error-return contracts, proving the value absent proves the error present.
func errorReturnPresenceRelations(labels []returns.ErrorReturn) []callpayload.CallReturnPresenceRelation {
	if len(labels) == 0 {
		return nil
	}
	out := make([]callpayload.CallReturnPresenceRelation, 0, len(labels)*3)
	for _, label := range labels {
		if label.ValueIndex < 0 || label.ErrorIndex < 0 {
			continue
		}
		out = append(out,
			callpayload.CallReturnPresenceRelation{
				TriggerIndex:    label.ErrorIndex,
				TriggerPresence: presence.Present(),
				TargetIndex:     label.ValueIndex,
				TargetPresence:  presence.Absent(),
			},
			callpayload.CallReturnPresenceRelation{
				TriggerIndex:    label.ErrorIndex,
				TriggerPresence: presence.Absent(),
				TargetIndex:     label.ValueIndex,
				TargetPresence:  presence.Present(),
			},
			callpayload.CallReturnPresenceRelation{
				TriggerIndex:    label.ValueIndex,
				TriggerPresence: presence.Absent(),
				TargetIndex:     label.ErrorIndex,
				TargetPresence:  presence.Present(),
			},
		)
	}
	return out
}

func activeErrorReturnLabels(sig signature.Function) []returns.ErrorReturn {
	return activeReturnLabels[returns.ErrorReturn](sig)
}
