package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func applyFrozenTableFact(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
) state.State {
	target, ok := resolvePlacementTargetValueAt(reg, resolver, point, out, targetPath, projectPath)
	if !ok {
		return out
	}
	id, ok := product.Get(reg, target.value, identity.Key).ID()
	if !ok {
		return out
	}
	return out.FreezeTable(id)
}

func dynamicIndexFactCanEscapeThroughStaticSegment(reg *axis.Registry, fact dynamicindex.Fact, seg segment.Segment) bool {
	return dynamicIndexFactDefinitelyMatchesSegment(reg, fact, seg) ||
		dynamicIndexFactMayMatchSegment(reg, fact, seg)
}

func resolvePlacementTargetValueAt(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	projectPath PathTypeProjector,
) (pathValue, bool) {
	target, ok := resolvePathValueAt(reg, resolver, point, out, targetPath, projectPath)
	if ok {
		if _, hasID := product.Get(reg, target.value, identity.Key).ID(); hasID {
			return target, true
		}
	}
	if len(targetPath.Segments) == 0 {
		return target, ok
	}
	if projected, projectedOK := projectPathDynamicIndexValue(reg, resolver, point, out, targetPath); projectedOK {
		if recovered, recoveredOK := mergePlacementIdentityProjection(reg, target, ok, projected); recoveredOK {
			return recovered, true
		}
	}
	if projected, projectedOK := projectPathHeapStaticMemberValue(reg, resolver, point, out, targetPath); projectedOK {
		if recovered, recoveredOK := mergePlacementIdentityProjection(reg, target, ok, projected); recoveredOK {
			return recovered, true
		}
	}
	if projected, projectedOK := projectPathOriginValue(nil, reg, out, targetPath, projectPath); projectedOK {
		if recovered, recoveredOK := mergePlacementIdentityProjection(reg, target, ok, projected); recoveredOK {
			return recovered, true
		}
	}
	return target, ok
}

func mergePlacementIdentityProjection(
	reg *axis.Registry,
	target pathValue,
	hasTarget bool,
	projected product.Value,
) (pathValue, bool) {
	if _, hasID := product.Get(reg, projected, identity.Key).ID(); !hasID {
		return pathValue{}, false
	}
	if hasTarget {
		if merged := product.Meet(reg, target.value, projected); !product.Equal(reg, merged, product.Bottom(reg)) {
			target.value = merged
			return target, true
		}
	}
	return pathValue{value: projected}, true
}

func escapeEventPlacement(kind callboundary.EscapeEventKind) (placement.Value, bool) {
	return escapeEventTransition(kind).Placement()
}

var escapeEventTransitions = map[callboundary.EscapeEventKind]placement.EscapeTransition{
	callboundary.EscapeEventRetain: placement.EscapeTransitionRetain,
	callboundary.EscapeEventStore:  placement.EscapeTransitionStore,
	callboundary.EscapeEventSend:   placement.EscapeTransitionSend,
	callboundary.EscapeEventExport: placement.EscapeTransitionExport,
	callboundary.EscapeEventOpaque: placement.EscapeTransitionOpaque,
}

func escapeEventTransition(kind callboundary.EscapeEventKind) placement.EscapeTransition {
	return escapeEventTransitions[kind]
}
