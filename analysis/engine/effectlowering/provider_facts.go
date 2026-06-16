package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

func signatureParamPathInvalidations(sig signature.Function, site factflow.CallSite) []factapply.CallParamPathInvalidation {
	targets := activeMutationTargets(sig)
	if len(targets) == 0 {
		return nil
	}
	args := site.ArgumentSources()
	var out []factapply.CallParamPathInvalidation
	for _, target := range targets {
		argIndex, ok := effect.ResolveParamIndex(target, len(args))
		if !ok || !callArgumentSourceCanBindPath(args[argIndex]) {
			continue
		}
		if len(out) != 0 {
			continue
		}
		out = append(out, factapply.CallParamPathInvalidation{
			Path: pathdom.NewPlaceholder(argIndex),
		})
	}
	return out
}

func signatureNormalReturnPathInvalidations(in []factapply.CallParamPathInvalidation) []callboundary.PathInvalidationFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.PathInvalidationFact, 0, len(in))
	for _, fact := range in {
		out = append(out, callboundary.PathInvalidationFact{Path: fact.Path})
	}
	return out
}

func activeMutationTargets(sig signature.Function) []effect.ParamRef {
	if len(sig.Effect.Labels) == 0 {
		return nil
	}
	out := make([]effect.ParamRef, 0, len(sig.Effect.Labels))
	for _, label := range sig.Effect.Labels {
		switch normalized := effect.NormalizeLabel(label).(type) {
		case mutation.Mutate:
			out = append(out, normalized.Target)
		case *mutation.Mutate:
			if normalized != nil {
				out = append(out, normalized.Target)
			}
		case mutation.TableMutator:
			out = append(out, normalized.Target)
		case *mutation.TableMutator:
			if normalized != nil {
				out = append(out, normalized.Target)
			}
		case mutation.LengthChange:
			out = append(out, normalized.Target)
		case *mutation.LengthChange:
			if normalized != nil {
				out = append(out, normalized.Target)
			}
		case ownership.Store:
			if normalized.Into.Index >= 0 {
				out = append(out, normalized.Into)
			}
		case *ownership.Store:
			if normalized != nil && normalized.Into.Index >= 0 {
				out = append(out, normalized.Into)
			}
		}
	}
	return out
}

func signatureEscapeEvents(sig signature.Function, site factflow.CallSite) []callboundary.EscapeEventFact {
	if len(sig.Effect.Labels) == 0 {
		return nil
	}
	args := site.ArgumentSources()
	out := make([]callboundary.EscapeEventFact, 0, len(sig.Effect.Labels))
	appendArg := func(index int, kind callboundary.EscapeEventKind) {
		if index < 0 || index >= len(args) || !callArgumentSourceCanBindPath(args[index]) {
			return
		}
		out = append(out, callboundary.EscapeEventFact{
			Target:    pathdom.NewPlaceholder(index),
			Kind:      kind,
			Recursive: true,
		})
	}
	appendParam := func(ref effect.ParamRef, kind callboundary.EscapeEventKind) {
		index, ok := effect.ResolveParamIndex(ref, len(args))
		if !ok {
			return
		}
		appendArg(index, kind)
	}
	for _, label := range sig.Effect.Labels {
		switch normalized := effect.NormalizeLabel(label).(type) {
		case ownership.Borrow:
			appendParam(normalized.Param, callboundary.EscapeEventBorrow)
		case *ownership.Borrow:
			if normalized != nil {
				appendParam(normalized.Param, callboundary.EscapeEventBorrow)
			}
		case ownership.BorrowAll:
			for i := range args {
				appendArg(i, callboundary.EscapeEventBorrow)
			}
		case *ownership.BorrowAll:
			if normalized != nil {
				for i := range args {
					appendArg(i, callboundary.EscapeEventBorrow)
				}
			}
		case ownership.Retain:
			appendParam(normalized.Param, callboundary.EscapeEventRetain)
		case *ownership.Retain:
			if normalized != nil {
				appendParam(normalized.Param, callboundary.EscapeEventRetain)
			}
		case ownership.Send:
			start, ok := sendStartIndex(normalized.FromParam, len(args))
			if !ok {
				continue
			}
			for i := start; i < len(args); i++ {
				appendArg(i, callboundary.EscapeEventSend)
			}
		case ownership.SendParam:
			appendParam(normalized.Param, callboundary.EscapeEventSend)
		case *ownership.Send:
			if normalized != nil {
				start, ok := sendStartIndex(normalized.FromParam, len(args))
				if !ok {
					continue
				}
				for i := start; i < len(args); i++ {
					appendArg(i, callboundary.EscapeEventSend)
				}
			}
		case *ownership.SendParam:
			if normalized != nil {
				appendParam(normalized.Param, callboundary.EscapeEventSend)
			}
		case ownership.Export:
			appendParam(normalized.Param, callboundary.EscapeEventExport)
		case *ownership.Export:
			if normalized != nil {
				appendParam(normalized.Param, callboundary.EscapeEventExport)
			}
		case ownership.Opaque:
			appendParam(normalized.Param, callboundary.EscapeEventOpaque)
		case *ownership.Opaque:
			if normalized != nil {
				appendParam(normalized.Param, callboundary.EscapeEventOpaque)
			}
		case ownership.Store:
			appendParam(normalized.Param, callboundary.EscapeEventStore)
		case *ownership.Store:
			if normalized != nil {
				appendParam(normalized.Param, callboundary.EscapeEventStore)
			}
		}
	}
	return out
}

func signatureFrozenTables(sig signature.Function, site factflow.CallSite) []callboundary.FrozenTableFact {
	if len(sig.Effect.Labels) == 0 {
		return nil
	}
	args := site.ArgumentSources()
	out := make([]callboundary.FrozenTableFact, 0, len(sig.Effect.Labels))
	appendParam := func(ref effect.ParamRef) {
		index, ok := effect.ResolveParamIndex(ref, len(args))
		if !ok || !callArgumentSourceCanBindPath(args[index]) {
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
		case *ownership.Freeze:
			if normalized != nil {
				appendParam(normalized.Param)
			}
		}
	}
	return out
}

func signatureStoreRelations(sig signature.Function, site factflow.CallSite) []callboundary.StoreRelationFact {
	if len(sig.Effect.Labels) == 0 {
		return nil
	}
	args := site.ArgumentSources()
	out := make([]callboundary.StoreRelationFact, 0, len(sig.Effect.Labels))
	appendStore := func(sourceRef, intoRef effect.ParamRef) {
		source, ok := effect.ResolveParamIndex(sourceRef, len(args))
		if !ok || !callArgumentSourceCanBindPath(args[source]) {
			return
		}
		into, ok := effect.ResolveParamIndex(intoRef, len(args))
		if !ok || !callArgumentSourceCanBindPath(args[into]) {
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
		case *ownership.Store:
			if normalized != nil {
				appendStore(normalized.Param, normalized.Into)
			}
		}
	}
	return out
}

func sendStartIndex(fromParam, argCount int) (int, bool) {
	if argCount <= 0 {
		return 0, false
	}
	start := fromParam
	if start < 0 {
		start = argCount + start
	}
	if start < 0 || start >= argCount {
		return 0, false
	}
	return start, true
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
	case postcondition.Absent:
		return product.Absent(ctx.Registry), true
	case *postcondition.Absent:
		if r != nil {
			return product.Absent(ctx.Registry), true
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
