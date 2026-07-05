package factapply

import (
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
)

func applyNormalReturnFrozenTables(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.FrozenTables {
		targetPath, ok := ctx.substitute(fact.Target)
		if !ok {
			continue
		}
		if targetKey, ok := ctx.keyspaceKey(fact.Target); ok {
			out = out.WriteEffectDelta(effectdelta.Key{
				Target: targetKey,
				Site:   callboundary.FrozenTableEffectSite(),
				Kind:   effectdelta.Freeze,
			}, effectdelta.Top())
		}
		out = applyFrozenTableFact(ctx.node.Registry, ctx.resolver, ctx.projectPath, ctx.point, out, targetPath)
	}
	return out
}

func applyNormalReturnEffectDeltas(ctx normalReturnApplyContext, out state.State) state.State {
	for _, delta := range ctx.normalFacts.EffectDeltas {
		targetKey, ok := ctx.keyspaceKey(delta.Target)
		if !ok {
			continue
		}
		out = out.WriteEffectDelta(effectdelta.Key{
			Target: targetKey,
			Site:   delta.Site,
			Kind:   delta.Kind,
		}, delta.Value)
	}
	return out
}

func applyNormalReturnStoreRelations(ctx normalReturnApplyContext, out state.State) state.State {
	for _, relation := range ctx.normalFacts.StoreRelations {
		sourceStateKey, ok := ctx.stateKey(relation.Source)
		if !ok {
			continue
		}
		intoStateKey, ok := ctx.stateKey(relation.Into)
		if !ok {
			continue
		}
		out = out.AddStoreRelation(state.StoreRelation{
			Source: sourceStateKey,
			Into:   intoStateKey,
		})
	}
	return out
}

func applyNormalReturnLifecycleFacts(ctx normalReturnApplyContext, out state.State) state.State {
	for _, fact := range ctx.normalFacts.LifecycleFacts {
		targetStateKey, ok := ctx.visibleStateKey(fact.Target)
		if !ok || fact.Protocol == "" {
			continue
		}
		resource := out.CanonicalTypestateResource(ctx.resolver.KeySpace(), targetStateKey, fact.Protocol)
		switch fact.Kind {
		case callboundary.LifecycleAcquire:
			out = out.AcquireTypestate(resource, fact.To, fact.Obligation)
		case callboundary.LifecycleTransition:
			out = out.TransitionTypestate(resource, fact.From, fact.To)
		case callboundary.LifecycleEscape:
			out = out.EscapeTypestate(resource)
		}
	}
	return out
}

func applyNormalReturnEscapeEvents(ctx normalReturnApplyContext, out state.State) state.State {
	for _, event := range ctx.normalFacts.EscapeEvents {
		targetPath, ok := ctx.substitute(event.Target)
		if !ok {
			continue
		}
		targetStateKey, ok := ctx.visibleStateKey(event.Target)
		if !ok {
			continue
		}
		out = out.AddEscapeEvent(state.EscapeEvent{
			Target:    targetStateKey,
			Kind:      event.Kind,
			Recursive: event.Recursive,
		})
		out = applyEscapeEventPlacement(ctx.node.Registry, ctx.resolver, ctx.projectPath, ctx.point, out, targetPath, event)
	}
	return out
}
