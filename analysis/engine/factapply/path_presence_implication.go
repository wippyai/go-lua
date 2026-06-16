package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
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
	outcomeProvider CallOutcomeProvider,
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
	cache := &callOutcomeTraversalCache{}
	for _, callPoint := range cache.graphRPO(ctx.Graph) {
		siteView, ok := facts.CallSiteView(callPoint)
		if !ok {
			continue
		}
		if !callOutcomePresenceRelationCanPublishAt(ctx.Graph, facts, cache, callPoint, siteView, ctx.Point) {
			continue
		}
		site := siteView.CallSite()
		outcome := outcomeProvider(callContextAt(ctx, callPoint, read), site, out, read)
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
	relation CallReturnPresenceRelation,
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
	triggerKey := factPathKeyAt(resolver, ctx.Point, triggerTarget.TargetPath())
	targetKey := factPathKeyAt(resolver, ctx.Point, target.TargetPath())
	if triggerKey == "" || targetKey == "" {
		return out
	}
	out = out.AddPathPresenceImplication(pathevidence.PathPresenceImplication{
		Trigger:         triggerKey,
		TriggerPresence: relation.TriggerPresence,
		Target:          targetKey,
		TargetPresence:  relation.TargetPresence,
	})
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
		snapshot := next.PathPresenceImplicationsSnapshot()
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
	if !presenceIsConcrete(implication.TriggerPresence) {
		return false
	}
	current, ok := readPathKeyPresence(reg, resolver, point, out, implication.Trigger)
	return ok && presence.Equal(current, implication.TriggerPresence)
}

func applyPathPresenceImplicationTarget(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	implication pathevidence.PathPresenceImplication,
) state.State {
	if !presenceIsConcrete(implication.TargetPresence) || !pathKeyCurrentlyVisible(resolver, point, implication.Target) {
		return out
	}
	constraint := product.NewWithPresence(reg, product.ShapeTop, implication.TargetPresence)
	if sym, ok := rootSymbolForResolverPathKey(implication.Target); ok {
		if invalidated, valid := out.InvalidatePathKeyDescendants(implication.Target); valid {
			out = invalidated
		}
		slot := key.SymbolValue(sym)
		return out.UpdateValue(reg, slot, func(value product.Value) product.Value {
			return product.Meet(reg, value, constraint)
		})
	}
	current := out.ReadPathKey(reg, implication.Target)
	if product.Equal(reg, current, product.Bottom(reg)) {
		return out.WritePathKey(reg, implication.Target, constraint)
	}
	return out.WritePathKey(reg, implication.Target, product.Meet(reg, current, constraint))
}

func readPathKeyPresence(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	pathKey pathdom.PathKey,
) (presence.Value, bool) {
	if !pathKeyCurrentlyVisible(resolver, point, pathKey) {
		return presence.Bottom(), false
	}
	if sym, ok := rootSymbolForResolverPathKey(pathKey); ok {
		return product.PresenceOf(out.ReadValue(reg, key.SymbolValue(sym))), true
	}
	return product.PresenceOf(out.ReadPathKey(reg, pathKey)), true
}

func pathKeyCurrentlyVisible(resolver *visibility.Resolver, point cfg.Point, pathKey pathdom.PathKey) bool {
	path, ok := resolverPathKeyPath(pathKey)
	if !ok {
		return true
	}
	if resolver == nil {
		return false
	}
	current := resolver.KeyAt(point, pathdom.Path{Symbol: path.Symbol, Segments: path.Segments})
	return current == pathKey
}

func rootSymbolForResolverPathKey(pathKey pathdom.PathKey) (symbol.ID, bool) {
	path, ok := resolverPathKeyPath(pathKey)
	if !ok || len(path.Segments) != 0 {
		return 0, false
	}
	return path.Symbol, path.Symbol != 0
}

func resolverPathKeyPath(pathKey pathdom.PathKey) (pathdom.Path, bool) {
	sym, version, suffix, ok := pathaddr.ParseResolverPath(pathKey)
	if !ok || version <= 0 {
		return pathdom.Path{}, false
	}
	var segments []segment.Segment
	if suffix != "" {
		var parsed bool
		segments, parsed = segment.ParseFormattedSegments(suffix)
		if !parsed {
			return pathdom.Path{}, false
		}
	}
	return pathdom.Path{Symbol: sym, Version: version, Segments: segments}, true
}

func presenceIsConcrete(value presence.Value) bool {
	return presence.Equal(value, presence.Present()) || presence.Equal(value, presence.Absent())
}
