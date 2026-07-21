package body

import (
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/callproducer"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ObservationPlan is the prepared-body query surface compiled before a result
// is published. It deliberately describes consumers, not an index of every
// solver point: boundary outputs are retained only for facts whose consumers
// need same-node effects, while edge observations are reduced to booleans.
//
// A plan is compiled from immutable prepared facts and the enabled
// call-boundary consumer. It marks only outputs consumed by summary, proof,
// diagnostic, placement, and semantic projections; it is not a point-state
// index.
type ObservationPlan struct {
	boundaryPoints   []cfg.Point
	boundarySet      map[cfg.Point]struct{}
	nodePoints       []cfg.Point
	nodeSet          map[cfg.Point]struct{}
	edgeReachability []observationEdge
}

type observationEdge struct {
	from cfg.Point
	to   cfg.Point
}

// NodePoints returns the exact prepared node-output inventory required to
// publish a Result. The returned slice is detached from the plan.
func (p ObservationPlan) NodePoints() []cfg.Point {
	return append([]cfg.Point(nil), p.nodePoints...)
}

// Edges returns the exact prepared edge-normality inventory required to
// publish a Result. Conditions remain owned by the prepared graph.
func (p ObservationPlan) Edges() []ResultEdge {
	out := make([]ResultEdge, len(p.edgeReachability))
	for index, edge := range p.edgeReachability {
		out[index] = ResultEdge{From: edge.from, To: edge.to}
	}
	return out
}

// PublishedFacts is the immutable projection of stabilized relation output.
// It retains a compact reachability boolean for every solved point, while full
// node states exist only for planned boundary consumers. This keeps ordinary
// reachability queries O(1) without turning the result into a generic point-
// state index.
type PublishedFacts struct {
	nodeOutputs         map[cfg.Point]state.State
	pointReachable      map[cfg.Point]bool
	nodeOutputReachable map[cfg.Point]bool
	edgeNormal          map[observationEdge]bool
	callOutcomes        map[cfg.Point]callpayload.CallOutcome
}

func compileObservationPlan(graph cfg.Graph, facts factflow.Facts) ObservationPlan {
	if graph == nil {
		return ObservationPlan{}
	}
	plan := ObservationPlan{
		boundaryPoints:   make([]cfg.Point, 0, graph.Size()),
		boundarySet:      make(map[cfg.Point]struct{}),
		nodePoints:       make([]cfg.Point, 0, graph.Size()),
		nodeSet:          make(map[cfg.Point]struct{}),
		edgeReachability: make([]observationEdge, 0, graph.Size()),
	}
	plannedEdges := make(map[observationEdge]struct{})
	// RPO and SuccessorsReadOnly are both canonical. Keeping this order makes
	// projection independent of map iteration and matches canonical publication.
	for _, point := range graph.RPO() {
		if plannedBoundaryNodeOutput(facts, point) {
			plan.boundaryPoints = append(plan.boundaryPoints, point)
			plan.boundarySet[point] = struct{}{}
			plan.nodePoints = append(plan.nodePoints, point)
			plan.nodeSet[point] = struct{}{}
		}
		// FactsEdgeTransfer is identity on non-branch edges. Their normality is
		// therefore represented by the input reachability (or a planned same-node
		// no-normal-return output), not a separately retained edge record.
		if graph.IsBranch(point) {
			if _, alreadyPlanned := plan.nodeSet[point]; !alreadyPlanned {
				plan.nodePoints = append(plan.nodePoints, point)
				plan.nodeSet[point] = struct{}{}
			}
			for _, successor := range cfg.SuccessorsReadOnly(graph, point) {
				edge := observationEdge{from: point, to: successor}
				if _, exists := plannedEdges[edge]; exists {
					continue
				}
				plannedEdges[edge] = struct{}{}
				plan.edgeReachability = append(plan.edgeReachability, edge)
			}
		}
	}
	return plan
}

func (p ObservationPlan) observesBoundary(point cfg.Point) bool {
	_, ok := p.boundarySet[point]
	return ok
}

func plannedBoundaryNodeOutput(facts factflow.Facts, point cfg.Point) bool {
	if _, ok := facts.RootAssignment(point); ok {
		return true
	}
	if _, ok := facts.PathAssignment(point); ok {
		return true
	}
	if _, ok := facts.PathDescendantInvalidation(point); ok {
		return true
	}
	if _, ok := facts.DynamicIndexWrite(point); ok {
		return true
	}
	if _, ok := facts.PathStaticMemberWrite(point); ok {
		return true
	}
	if _, ok := facts.Return(point); ok || callproducer.Has(facts, point) {
		return true
	}
	if _, ok := facts.CallSiteView(point); ok {
		return true
	}
	if facts.NoNormalReturn(point) || len(facts.CallResultValues(point)) != 0 || facts.HasChannelSelects(point) || len(facts.CovariantExposures(point)) != 0 {
		return true
	}
	return len(facts.PostconditionRefinements(point)) != 0 || len(facts.PostconditionPathRelations(point)) != 0
}

// ObservationStats returns a copy of this body's publication counters.
func (r *Result) ObservationStats() ObservationStats {
	if r == nil {
		return ObservationStats{}
	}
	return r.observation
}

func (r *Result) publishedNodeOutput(point cfg.Point) (state.State, bool) {
	if r == nil || r.published.nodeOutputs == nil {
		return state.State{}, false
	}
	out, ok := r.published.nodeOutputs[point]
	return out, ok
}

func (r *Result) publishedPointReachable(point cfg.Point) (bool, bool) {
	if r == nil || r.published.pointReachable == nil {
		return false, false
	}
	reachable, ok := r.published.pointReachable[point]
	return reachable, ok
}

func (r *Result) publishedNodeOutputReachable(point cfg.Point) (bool, bool) {
	if r == nil || r.published.nodeOutputReachable == nil {
		return false, false
	}
	reachable, ok := r.published.nodeOutputReachable[point]
	return reachable, ok
}

func (r *Result) publishedEdgeNormal(from, to cfg.Point) (bool, bool) {
	if r == nil || r.published.edgeNormal == nil {
		return false, false
	}
	normal, ok := r.published.edgeNormal[observationEdge{from: from, to: to}]
	return normal, ok
}
