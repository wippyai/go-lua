package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func applyCallOutcomePresenceRelationPublishes(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	cache *callOutcomeTraversalCache,
	outcomeProvider callpayload.CallOutcomeProvider,
	resolver *visibility.Resolver,
	read func(cfg.Point) state.State,
	out state.State,
) state.State {
	if outcomeProvider == nil || resolver == nil || ctx.Graph == nil {
		return out
	}
	if read == nil {
		read = emptyStateRead
	}
	if cache == nil {
		cache = &callOutcomeTraversalCache{}
	}
	for _, callPoint := range cache.graphRPO(ctx.Graph) {
		siteView, ok := facts.CallSiteView(callPoint)
		if !ok {
			continue
		}
		if !callOutcomePresenceRelationCanPublishAt(ctx.Graph, facts, cache, callPoint, siteView, ctx.Point) {
			continue
		}
		outcome := outcomeProvider(callContextAt(ctx, callPoint, read), siteView, out, read)
		for _, relation := range outcome.ReturnPresenceRelations {
			out = publishCallReturnPresenceImplication(ctx, facts, cache, resolver, callPoint, siteView, relation, out)
		}
	}
	return out
}

func callOutcomePresenceRelationCanPublishAt(
	graph cfg.Graph,
	facts factflow.Facts,
	cache *callOutcomeTraversalCache,
	callPoint cfg.Point,
	site factflow.CallSiteView,
	point cfg.Point,
) bool {
	if graph == nil {
		return false
	}
	assignment, ok := facts.RootAssignment(point)
	if !ok || !callOutcomeAssignmentConsumesCall(assignment, callPoint) {
		return false
	}
	if site.ResultTargetCount() == 0 {
		return false
	}
	canPublish := false
	site.ForEachResultTarget(func(triggerTarget factflow.CallResultTargetView) bool {
		if !callOutcomeRelatableTarget(triggerTarget) {
			return true
		}
		triggerAssign, ok := callOutcomeResultAssignmentPoint(cache, graph, facts, callPoint, triggerTarget, triggerTarget.ResultIndex())
		if !ok {
			return true
		}
		site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
			if !callOutcomeRelatableTarget(target) {
				return true
			}
			targetAssign, ok := callOutcomeResultAssignmentPoint(cache, graph, facts, callPoint, target, target.ResultIndex())
			if !ok {
				return true
			}
			if callOutcomeLaterPoint(cache, graph, targetAssign, triggerAssign) == point {
				canPublish = true
				return false
			}
			return true
		})
		return !canPublish
	})
	return canPublish
}

func callOutcomeAssignmentConsumesCall(assignment factflow.RootAssignment, callPoint cfg.Point) bool {
	source := assignment.Source()
	return source.Kind == factflow.ValueSourceCall && source.HasCallPoint && source.CallPoint == callPoint
}

func publishCallReturnPresenceImplication(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	cache *callOutcomeTraversalCache,
	resolver *visibility.Resolver,
	callPoint cfg.Point,
	site factflow.CallSiteView,
	relation callpayload.CallReturnPresenceRelation,
	out state.State,
) state.State {
	triggerTarget, ok := callOutcomeTargetForResult(site, relation.TriggerIndex)
	if !ok || !callOutcomeRelatableTarget(triggerTarget) {
		return out
	}
	target, ok := callOutcomeTargetForResult(site, relation.TargetIndex)
	if !ok || !callOutcomeRelatableTarget(target) {
		return out
	}
	triggerAssign, ok := callOutcomeResultAssignmentPoint(cache, ctx.Graph, facts, callPoint, triggerTarget, relation.TriggerIndex)
	if !ok {
		return out
	}
	targetAssign, ok := callOutcomeResultAssignmentPoint(cache, ctx.Graph, facts, callPoint, target, relation.TargetIndex)
	if !ok {
		return out
	}
	if ctx.Point != callOutcomeLaterPoint(cache, ctx.Graph, targetAssign, triggerAssign) {
		return out
	}
	trigger, ok := factKeyspaceKeyAt(resolver, ctx.Point, triggerTarget.TargetPathRef())
	if !ok {
		return out
	}
	targetK, ok := factKeyspaceKeyAt(resolver, ctx.Point, target.TargetPathRef())
	if !ok {
		return out
	}
	out = out.AddPathPresenceImplication(pathevidence.NewPathPresenceImplication(
		trigger,
		relation.TriggerPresence,
		targetK,
		relation.TargetPresence,
	))
	return activatePathPresenceImplications(ctx.Registry, resolver, ctx.Point, out)
}

func applyPathValuePresenceImplication(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	fact factflow.PathValuePresenceImplication,
) state.State {
	if resolver == nil {
		return out
	}
	trigger, ok := factKeyspaceKeyAt(resolver, ctx.Point, fact.TriggerPathRef())
	if !ok {
		return out
	}
	target, ok := factKeyspaceKeyAt(resolver, ctx.Point, fact.TargetPathRef())
	if !ok {
		return out
	}
	out = out.AddPathPresenceImplication(pathevidence.NewPathValuePresenceImplication(
		trigger,
		fact.TriggerValue(),
		target,
		fact.TargetPresence(),
	))
	return activatePathPresenceImplications(ctx.Registry, resolver, ctx.Point, out)
}

func activatePathPresenceImplicationsForPath(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
) state.State {
	pathKey := factPathKeyAt(resolver, point, targetPath)
	if pathKey == "" {
		return out
	}
	return activatePathPresenceImplications(reg, resolver, point, out)
}

func activatePathPresenceImplications(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
) state.State {
	stateDomain := state.Domain(reg)
	for {
		next := out
		snapshot := next.PathPresenceImplicationsSnapshot(resolver.KeySpace())
		if snapshot.Bottom || len(snapshot.Implications) == 0 {
			return next
		}
		for _, implication := range snapshot.Implications {
			if !pathPresenceImplicationTriggered(reg, resolver, point, next, implication) {
				continue
			}
			next = applyPathPresenceImplicationTarget(reg, resolver, point, next, implication)
		}
		if stateDomain.Equal(next, out) {
			return out
		}
		out = next
	}
}

func pathPresenceImplicationTriggered(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	implication pathevidence.PathPresenceImplication,
) bool {
	if implication.HasTriggerValue {
		current, ok := readPathKeyValue(reg, resolver, point, out, resolver.KeySpace().Format(implication.Trigger))
		if !ok || product.Equal(reg, current, product.Bottom(reg)) {
			return false
		}
		return product.Domain(reg).LessOrEq(current, implication.TriggerValue)
	}
	if !presenceIsConcrete(implication.TriggerPresence) {
		return false
	}
	current, ok := readPathKeyPresence(reg, resolver, point, out, resolver.KeySpace().Format(implication.Trigger))
	return ok && presence.Equal(current, implication.TriggerPresence)
}

func applyPathPresenceImplicationTarget(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	implication pathevidence.PathPresenceImplication,
) state.State {
	ks := resolver.KeySpace()
	targetKey := ks.Format(implication.Target)
	if !presenceIsConcrete(implication.TargetPresence) || !pathKeyCurrentlyVisible(resolver, point, targetKey) {
		return out
	}
	constraint := product.NewWithPresence(reg, product.ShapeTop, implication.TargetPresence)
	if sym, ok := rootSymbolForResolverPathKey(ks, targetKey); ok {
		if presenceImplicationTargetInvalidatesDescendants(implication.TargetPresence) {
			if invalidated, valid := out.InvalidatePathKeyDescendants(ks, targetKey); valid {
				out = invalidated
			}
		}
		slot := key.SymbolValue(sym)
		return out.UpdateValue(reg, slot, func(value product.Value) product.Value {
			return product.Meet(reg, value, constraint)
		})
	}
	current := out.ReadPathKey(reg, ks, targetKey)
	if product.Equal(reg, current, product.Bottom(reg)) {
		return out.WritePathKey(reg, ks, targetKey, constraint)
	}
	return out.WritePathKey(reg, ks, targetKey, product.Meet(reg, current, constraint))
}

func presenceImplicationTargetInvalidatesDescendants(target presence.Value) bool {
	return presence.Equal(target, presence.Absent())
}

func readPathKeyPresence(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	pathKey pathdom.PathKey,
) (presence.Value, bool) {
	value, ok := readPathKeyValue(reg, resolver, point, out, pathKey)
	if !ok {
		return presence.Bottom(), false
	}
	return product.PresenceOf(value), true
}

func readPathKeyValue(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	pathKey pathdom.PathKey,
) (product.Value, bool) {
	if !pathKeyCurrentlyVisible(resolver, point, pathKey) {
		return product.Value{}, false
	}
	if sym, ok := rootSymbolForResolverPathKey(resolver.KeySpace(), pathKey); ok {
		return out.ReadValue(reg, key.SymbolValue(sym)), true
	}
	return out.ReadPathKey(reg, resolver.KeySpace(), pathKey), true
}

func pathKeyCurrentlyVisible(resolver *visibility.Resolver, point cfg.Point, pathKey pathdom.PathKey) bool {
	if resolver == nil {
		return false
	}
	k, ok := resolver.KeySpace().FromStateKey(pathKey)
	if !ok {
		return true
	}
	segments, ok := resolver.KeySpace().SegmentsView(k)
	if !ok {
		return false
	}
	current := factPathKeyAt(resolver, point, pathdom.Path{Symbol: k.Sym, Segments: segments})
	return current == pathKey
}

func rootSymbolForResolverPathKey(ks *keyspace.KeySpace, pathKey pathdom.PathKey) (symbol.ID, bool) {
	k, ok := ks.FromStateKey(pathKey)
	if !ok || k.Segs != 0 {
		return 0, false
	}
	return k.Sym, k.Sym != 0
}

func presenceIsConcrete(value presence.Value) bool {
	return presence.Equal(value, presence.Present()) || presence.Equal(value, presence.Absent())
}
