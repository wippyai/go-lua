package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

func applyOperationalEffects(ctx transfer.NodeContext, out factapply.CallOutcome, effects *signature.OperationalEffects) factapply.CallOutcome {
	if effects == nil {
		return out
	}
	out.ReturnPresenceRelations = operationalReturnPresenceRelations(*effects)
	out.NormalReturnFacts.PathRefinements = operationalPathPresenceRefinements(ctx, *effects)
	out.NormalReturnFacts.PathInvalidations = operationalPathInvalidations(*effects)
	out.NormalReturnFacts.FrozenTables = operationalFrozenTables(*effects)
	out.NormalReturnFacts.EscapeEvents = operationalEscapeEvents(*effects)
	out.NormalReturnFacts.StoreRelations = operationalStoreRelations(*effects)
	return out
}

func operationalReturnPresenceRelations(e signature.OperationalEffects) []factapply.CallReturnPresenceRelation {
	if len(e.ReturnPresenceRelations) == 0 {
		return nil
	}
	out := make([]factapply.CallReturnPresenceRelation, 0, len(e.ReturnPresenceRelations))
	for _, relation := range e.ReturnPresenceRelations {
		out = append(out, factapply.CallReturnPresenceRelation{
			TriggerIndex:    relation.TriggerIndex,
			TriggerPresence: relation.TriggerPresence,
			TargetIndex:     relation.TargetIndex,
			TargetPresence:  relation.TargetPresence,
		})
	}
	return out
}

func operationalPathPresenceRefinements(ctx transfer.NodeContext, e signature.OperationalEffects) []callboundary.PathValueFact {
	if len(e.NormalReturnPresenceRefinements) == 0 {
		return nil
	}
	out := make([]callboundary.PathValueFact, 0, len(e.NormalReturnPresenceRefinements))
	for _, refinement := range e.NormalReturnPresenceRefinements {
		value, ok := operationalPresenceValue(ctx, refinement.Presence)
		if !ok {
			continue
		}
		out = append(out, callboundary.PathValueFact{
			Path:  refinement.Path,
			Value: value,
		})
	}
	return out
}

func operationalPresenceValue(ctx transfer.NodeContext, p presence.Value) (product.Value, bool) {
	if ctx.Registry == nil {
		return product.Value{}, false
	}
	switch {
	case presence.Equal(p, presence.Present()):
		return product.NewWithPresence(ctx.Registry, product.ShapeTop, presence.Present()), true
	case presence.Equal(p, presence.Absent()):
		return product.Absent(ctx.Registry), true
	default:
		return product.Value{}, false
	}
}

func operationalPathInvalidations(e signature.OperationalEffects) []callboundary.PathInvalidationFact {
	if len(e.PathInvalidations) == 0 {
		return nil
	}
	out := make([]callboundary.PathInvalidationFact, 0, len(e.PathInvalidations))
	for _, fact := range e.PathInvalidations {
		out = append(out, callboundary.PathInvalidationFact{Path: fact.Path})
	}
	return out
}

func operationalFrozenTables(e signature.OperationalEffects) []callboundary.FrozenTableFact {
	if len(e.FrozenTables) == 0 {
		return nil
	}
	out := make([]callboundary.FrozenTableFact, 0, len(e.FrozenTables))
	for _, fact := range e.FrozenTables {
		out = append(out, callboundary.FrozenTableFact{Target: fact.Target})
	}
	return out
}

func operationalEscapeEvents(e signature.OperationalEffects) []callboundary.EscapeEventFact {
	if len(e.EscapeEvents) == 0 {
		return nil
	}
	out := make([]callboundary.EscapeEventFact, 0, len(e.EscapeEvents))
	for _, event := range e.EscapeEvents {
		kind, ok := operationalEscapeKind(event.Kind)
		if !ok {
			continue
		}
		out = append(out, callboundary.EscapeEventFact{
			Target:    event.Target,
			Kind:      kind,
			Recursive: event.Recursive,
		})
	}
	return out
}

func operationalEscapeKind(kind signature.EscapeKind) (callboundary.EscapeEventKind, bool) {
	switch kind {
	case signature.EscapeBorrow:
		return callboundary.EscapeEventBorrow, true
	case signature.EscapeRetain:
		return callboundary.EscapeEventRetain, true
	case signature.EscapeStore:
		return callboundary.EscapeEventStore, true
	case signature.EscapeSend:
		return callboundary.EscapeEventSend, true
	case signature.EscapeExport:
		return callboundary.EscapeEventExport, true
	case signature.EscapeOpaque:
		return callboundary.EscapeEventOpaque, true
	default:
		return callboundary.EscapeEventNone, false
	}
}

func operationalStoreRelations(e signature.OperationalEffects) []callboundary.StoreRelationFact {
	if len(e.StoreRelations) == 0 {
		return nil
	}
	out := make([]callboundary.StoreRelationFact, 0, len(e.StoreRelations))
	for _, relation := range e.StoreRelations {
		out = append(out, callboundary.StoreRelationFact{
			Source: relation.Source,
			Into:   relation.Into,
		})
	}
	return out
}
