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
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
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

func applyEscapeEventPlacement(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	event callboundary.EscapeEventFact,
) state.State {
	value, ok := escapeEventPlacement(event.Kind)
	if !ok {
		return out
	}
	target, ok := resolvePlacementTargetValueAt(reg, resolver, point, out, targetPath, projectPath)
	if !ok {
		return markEscapePathCandidatePlacements(reg, resolver, point, out, targetPath, value, event.Recursive, map[identity.ID]struct{}{})
	}
	id, ok := product.Get(reg, target.value, identity.Key).ID()
	if !ok {
		return markEscapePathCandidatePlacements(reg, resolver, point, out, targetPath, value, event.Recursive, map[identity.ID]struct{}{})
	}
	if !event.Recursive {
		return writeJoinedPlacement(out, id, value)
	}
	return markReachableHeapPlacement(reg, out, id, value, map[identity.ID]struct{}{})
}

func markEscapePathCandidatePlacements(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	value placement.Value,
	recursive bool,
	seen map[identity.ID]struct{},
) state.State {
	if resolver == nil || len(targetPath.Segments) == 0 {
		return out
	}
	parent := targetPath.ParentView()
	if parent.IsEmpty() {
		return out
	}
	last := targetPath.Segments[len(targetPath.Segments)-1]
	if tableKey, ok := factKeyspaceKeyAt(resolver, point, parent); ok {
		snapshot := out.DynamicIndexFactsSnapshot()
		if !snapshot.Top {
			for key, fact := range snapshot.Facts {
				if key.Table != tableKey ||
					fact.Admission == dynamicindex.AdmissionRejected ||
					!dynamicIndexFactCanEscapeThroughStaticSegment(reg, fact, last) {
					continue
				}
				out = markEscapeValuePlacement(reg, out, fact.Value, value, recursive, seen)
			}
		}
	}
	parentID, ok := dynamicIndexParentHeapID(reg, resolver, point, out, parent)
	if !ok {
		return out
	}
	object := out.ReadHeapTableObject(reg, parentID)
	for _, fact := range object.DynamicIndexFacts() {
		if fact.Admission == dynamicindex.AdmissionRejected ||
			!dynamicIndexFactCanEscapeThroughStaticSegment(reg, fact, last) {
			continue
		}
		out = markEscapeValuePlacement(reg, out, fact.Value, value, recursive, seen)
	}
	return out
}

func dynamicIndexFactCanEscapeThroughStaticSegment(reg *axis.Registry, fact dynamicindex.Fact, seg segment.Segment) bool {
	return dynamicIndexFactDefinitelyMatchesSegment(reg, fact, seg) ||
		dynamicIndexFactMayMatchSegment(reg, fact, seg)
}

func markEscapeValuePlacement(
	reg *axis.Registry,
	out state.State,
	target product.Value,
	value placement.Value,
	recursive bool,
	seen map[identity.ID]struct{},
) state.State {
	id, ok := product.Get(reg, target, identity.Key).ID()
	if !ok {
		return out
	}
	if !recursive {
		return writeJoinedPlacement(out, id, value)
	}
	return markReachableHeapPlacement(reg, out, id, value, seen)
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
	switch kind {
	case callboundary.EscapeEventSend, callboundary.EscapeEventExport, callboundary.EscapeEventOpaque:
		return placement.SharedHeap, true
	case callboundary.EscapeEventStore, callboundary.EscapeEventRetain:
		return placement.OwnedHeap, true
	default:
		return placement.Bottom, false
	}
}

func markReachableHeapPlacement(
	reg *axis.Registry,
	out state.State,
	id identity.ID,
	value placement.Value,
	seen map[identity.ID]struct{},
) state.State {
	if id == (identity.ID{}) {
		return out
	}
	if _, ok := seen[id]; ok {
		return out
	}
	seen[id] = struct{}{}
	out = writeJoinedPlacement(out, id, value)
	object := out.ReadHeapTableObject(reg, id)
	objectDomain := heapidentity.ObjectDomain(reg)
	if objectDomain.Equal(object, objectDomain.Bottom()) {
		return out
	}
	out = markReachableHeapValuePlacement(reg, out, object.Root(), value, seen)
	for _, member := range object.StaticMembers() {
		out = markReachableHeapValuePlacement(reg, out, member, value, seen)
	}
	for _, fact := range object.DynamicIndexFacts() {
		out = markReachableHeapValuePlacement(reg, out, fact.KeyValue, value, seen)
		out = markReachableHeapValuePlacement(reg, out, fact.Value, value, seen)
	}
	return out
}

func markReachableHeapValuePlacement(
	reg *axis.Registry,
	out state.State,
	value product.Value,
	target placement.Value,
	seen map[identity.ID]struct{},
) state.State {
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok {
		return out
	}
	return markReachableHeapPlacement(reg, out, id, target, seen)
}

func markReachableHeapObjectValuePlacement(
	reg *axis.Registry,
	out state.State,
	value product.Value,
	target placement.Value,
	seen map[identity.ID]struct{},
) state.State {
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok {
		return out
	}
	return markReachableHeapObjectPlacement(reg, out, id, target, seen)
}

func markReachableHeapObjectPlacement(
	reg *axis.Registry,
	out state.State,
	id identity.ID,
	value placement.Value,
	seen map[identity.ID]struct{},
) state.State {
	if id == (identity.ID{}) {
		return out
	}
	if _, ok := seen[id]; ok {
		return out
	}
	object := out.ReadHeapTableObject(reg, id)
	objectDomain := heapidentity.ObjectDomain(reg)
	if objectDomain.Equal(object, objectDomain.Bottom()) {
		return out
	}
	seen[id] = struct{}{}
	out = writeJoinedPlacement(out, id, value)
	out = markReachableHeapObjectValuePlacement(reg, out, object.Root(), value, seen)
	for _, member := range object.StaticMembers() {
		out = markReachableHeapObjectValuePlacement(reg, out, member, value, seen)
	}
	for _, fact := range object.DynamicIndexFacts() {
		out = markReachableHeapObjectValuePlacement(reg, out, fact.KeyValue, value, seen)
		out = markReachableHeapObjectValuePlacement(reg, out, fact.Value, value, seen)
	}
	return out
}

func writeJoinedPlacement(out state.State, id identity.ID, value placement.Value) state.State {
	if id == (identity.ID{}) {
		return out
	}
	return out.WritePlacement(id, placement.Join(out.ReadPlacement(id), value))
}
