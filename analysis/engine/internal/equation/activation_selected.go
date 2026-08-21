package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/internal/canonical"
)

// SelectedStructuralFactorEdge is one candidate-local transport admitted
// only for an accepted activation Member. It never mutates the base Graph.
type SelectedStructuralFactorEdge struct {
	key    composition.Key
	source Point
	target Point
	input  Input
	factor composition.Key
}

func (edge SelectedStructuralFactorEdge) Available() bool {
	return edge.key.Available() && edge.source.Available() && edge.target.Available() && edge.input.Available() && edge.factor.Available()
}

func (edge SelectedStructuralFactorEdge) Key() composition.Key    { return edge.key }
func (edge SelectedStructuralFactorEdge) Source() Point           { return edge.source }
func (edge SelectedStructuralFactorEdge) Target() Point           { return edge.target }
func (edge SelectedStructuralFactorEdge) Input() Input            { return edge.input }
func (edge SelectedStructuralFactorEdge) Factor() composition.Key { return edge.factor }

// SelectedStructuralFactorEdges lowers only the direct transport bindings
// owned by accepted Members. It does not enumerate inactive candidates, and
// it attaches each member's exact premise at an endpoint scope before the
// runtime overlay observes the edge.
func (topology *Topology) SelectedStructuralFactorEdges(base *Graph, accepted []AcceptedMember) ([]SelectedStructuralFactorEdge, bool) {
	if topology == nil || base == nil || !topology.OwnsGraph(base) || !topology.validAccepted(accepted) {
		return nil, false
	}
	result := make([]SelectedStructuralFactorEdge, 0)
	for _, acceptedMember := range accepted {
		member := acceptedMember.Member()
		if !topology.ownsMember(member) {
			return nil, false
		}
		if topology.activation != nil {
			transports, found := topology.activation.transports(member.Binding())
			if !found {
				return nil, false
			}
			for _, transport := range transports {
				edge, edgeOK := topology.selectedDirectActivationEdge(base, member.Binding(), acceptedMember.Premise(), transport)
				if !edgeOK {
					return nil, false
				}
				result = append(result, edge)
			}
			continue
		}
		return nil, false
	}
	sort.Slice(result, func(left, right int) bool { return lessKey(result[left].key, result[right].key) })
	for index := 1; index < len(result); index++ {
		if result[index-1].key == result[index].key {
			return nil, false
		}
	}
	return result, true
}

// ActivationGraphOverlay derives the exact recurrence certificate for a
// prepared direct-activation delta. It shares the sealed points/groups and
// cold metadata, while appending only the newly admitted Factor edges and
// rebuilding the compact WTO/region rows from the already prepared schedule.
// It is deliberately not an accepted Graph oracle: callers must supply the
// exact selected edge delta and schedule proven by the runtime overlay.
func (graph *Graph) ActivationGraphOverlay(execution *schedule.Schedule, additions []SelectedStructuralFactorEdge) (*Graph, bool) {
	if graph == nil || !graph.valid() || execution == nil || execution.NodeCount() != len(graph.points) {
		return nil, false
	}
	view := *graph
	view.self = &view
	view.payload = graph
	view.schedule = execution
	view.factorEdges = append([]FactorEdgeNode(nil), graph.factorEdges...)
	view.factorIncoming = cloneIntRows(graph.factorIncoming)
	view.factorOutgoing = cloneIntRows(graph.factorOutgoing)
	existing := make(map[composition.Key]struct{}, len(view.factorEdges)+len(additions))
	for _, edge := range view.factorEdges {
		if !edge.key.Available() {
			return nil, false
		}
		existing[edge.key] = struct{}{}
	}
	for _, selected := range additions {
		if !selected.Available() || !graph.OwnsPoint(selected.source) || !graph.OwnsPoint(selected.target) || selected.input.Point().Key() != selected.source.Key() {
			return nil, false
		}
		if _, duplicate := existing[selected.key]; duplicate {
			return nil, false
		}
		source, sourceOK := graph.PointIndex(selected.source)
		target, targetOK := graph.PointIndex(selected.target)
		if !sourceOK || !targetOK || source < 0 || source >= len(view.factorOutgoing) || target < 0 || target >= len(view.factorIncoming) {
			return nil, false
		}
		index := len(view.factorEdges)
		view.factorEdges = append(view.factorEdges, FactorEdgeNode{graph: &view, key: selected.key, target: selected.target, input: selected.input, factor: selected.factor})
		view.factorOutgoing[source] = append(view.factorOutgoing[source], index)
		view.factorIncoming[target] = append(view.factorIncoming[target], index)
		existing[selected.key] = struct{}{}
	}
	view.eventNodes = make([]int, execution.EventCount()+1)
	view.eventPoints = make([]schedule.Node, 0, len(view.points))
	view.pointOrder = make([]int, len(view.points))
	view.pointRegion = make([]int, len(view.points))
	for index := range view.pointOrder {
		view.pointOrder[index] = -1
		view.pointRegion[index] = schedule.NoRegion
	}
	for index := 0; index < execution.EventCount(); index++ {
		event, eventOK := execution.EventAt(index)
		if !eventOK || event.Node < 0 || int(event.Node) >= len(view.points) {
			return nil, false
		}
		view.eventNodes[index+1] = view.eventNodes[index]
		if event.Kind == schedule.EventNode {
			if view.pointOrder[event.Node] != -1 || event.Region < schedule.NoRegion || event.Region >= execution.RegionCount() {
				return nil, false
			}
			view.pointOrder[event.Node] = len(view.eventPoints)
			view.pointRegion[event.Node] = event.Region
			view.eventPoints = append(view.eventPoints, event.Node)
			view.eventNodes[index+1]++
		}
	}
	if len(view.eventPoints) != len(view.points) {
		return nil, false
	}
	for _, order := range view.pointOrder {
		if order < 0 || order >= len(view.eventPoints) {
			return nil, false
		}
	}
	view.regionNodes = make([]int, execution.RegionCount())
	for index := range view.regionNodes {
		region, regionOK := execution.RegionAt(index)
		if !regionOK || region.Enter < 0 || region.Exit < region.Enter || region.Exit >= execution.EventCount() {
			return nil, false
		}
		view.regionNodes[index] = view.eventNodes[region.Exit+1] - view.eventNodes[region.Enter]
		if view.regionNodes[index] == 0 {
			return nil, false
		}
	}
	view.regions = nil
	view.regionInterfaces = nil
	view.regionExternal = nil
	view.regionBack = nil
	view.regionInternal = nil
	view.regionInternalInputs = nil
	view.regionEnvironmentExternal = nil
	view.regionEnvironmentBack = nil
	view.regionFactorExternal = nil
	view.regionFactorBack = nil
	view.regionFactorInternal = nil
	view.regionFactors = nil
	if !view.deriveRegions() {
		return nil, false
	}
	return &view, true
}

func cloneIntRows(rows [][]int) [][]int {
	result := make([][]int, len(rows))
	for index, row := range rows {
		result[index] = append([]int(nil), row...)
	}
	return result
}

func (topology *Topology) selectedDirectActivationEdge(base *Graph, binding composition.Key, premise Expr, transport DirectActivationTransport) (SelectedStructuralFactorEdge, bool) {
	if topology == nil || base == nil || !premise.Available() || !binding.Available() || !transport.Factor.Available() || transport.Source == 0 || transport.Target == 0 {
		return SelectedStructuralFactorEdge{}, false
	}
	if _, known := topology.source.FactorIndex(transport.Factor); !known {
		return SelectedStructuralFactorEdge{}, false
	}
	source, sourceOK := topology.directCandidatePoint(base, transport.Source)
	target, targetOK := topology.directCandidatePoint(base, transport.Target)
	if !sourceOK || !targetOK {
		return SelectedStructuralFactorEdge{}, false
	}
	maps := make([]DecisionMap, len(source.Scope().row.decisions))
	for index, decision := range source.Scope().row.decisions {
		if target.Scope().contains(decision) {
			maps[index] = Identity(decision)
		} else {
			maps[index] = Forget(decision)
		}
	}
	reindex, reindexed := NewReindex(source.Scope(), target.Scope(), maps)
	if !reindexed {
		return SelectedStructuralFactorEdge{}, false
	}
	pre, post := TrueExpr(), TrueExpr()
	if validScopedExpr(premise, source.Scope()) {
		var attached bool
		pre, attached = AndExpr(premise, pre)
		if !attached {
			return SelectedStructuralFactorEdge{}, false
		}
	} else if validScopedExpr(premise, target.Scope()) {
		var attached bool
		post, attached = AndExpr(premise, post)
		if !attached {
			return SelectedStructuralFactorEdge{}, false
		}
	} else {
		return SelectedStructuralFactorEdge{}, false
	}
	input := BoundaryInput(source.Site(), target.Site(), binding, pre, reindex, post)
	if !input.Available() {
		return SelectedStructuralFactorEdge{}, false
	}
	input.point = source
	key, keyed := identityKey("analysis/engine/equation/factor-edge", func(writer *canonical.DigestWriter) bool {
		return writeKey(writer, input.Key()) && writePoint(writer, target) && writeKey(writer, transport.Factor)
	})
	if !keyed {
		return SelectedStructuralFactorEdge{}, false
	}
	return SelectedStructuralFactorEdge{key: key, source: source, target: target, input: input, factor: transport.Factor}, true
}

func (topology *Topology) directCandidatePoint(base *Graph, ref PointRef) (Point, bool) {
	index := int(uint64(ref)) - 1
	if topology == nil || base == nil || index < 0 || index >= len(topology.rows.points) {
		return Point{}, false
	}
	key := topology.rows.points[index]
	node, found := base.pointAt[key]
	if !found || node < 0 || int(node) >= len(base.points) {
		return Point{}, false
	}
	point := base.points[node]
	return point, base.OwnsPoint(point) && point.key == key
}
