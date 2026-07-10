package body

import (
	"github.com/wippyai/go-lua/analysis/engine/callproducer"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
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

// PublishedFacts is the immutable, consumer-specific output of an
// ObservationPlan. It is intentionally not a generic point-state index.
// Node states exist only for planned boundary facts and only while a body
// result is being projected into summaries, diagnostics, placement, and the
// service semantic snapshot.
type PublishedFacts struct {
	nodeOutputs map[cfg.Point]state.State
	edgeNormal  map[observationEdge]bool
}

func compileObservationPlan(graph cfg.Graph, facts factflow.Facts, callOutcomeEnabled bool) ObservationPlan {
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
	// RPO and SuccessorsReadOnly are both canonical. Keeping this order makes
	// projection independent of map iteration and matches transfer.Run.
	for _, point := range graph.RPO() {
		if plannedBoundaryNodeOutput(facts, callOutcomeEnabled, point) {
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
				plan.edgeReachability = append(plan.edgeReachability, observationEdge{from: point, to: successor})
			}
		}
	}
	return plan
}

func (p ObservationPlan) observesBoundary(point cfg.Point) bool {
	_, ok := p.boundarySet[point]
	return ok
}

func (p ObservationPlan) observesNode(point cfg.Point) bool {
	_, ok := p.nodeSet[point]
	return ok
}

func plannedBoundaryNodeOutput(facts factflow.Facts, callOutcomeEnabled bool, point cfg.Point) bool {
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
	if callOutcomeEnabled {
		if _, ok := facts.CallSiteView(point); ok {
			return true
		}
	}
	if facts.NoNormalReturn(point) || len(facts.CallResultValues(point)) != 0 || facts.HasChannelSelects(point) || len(facts.CovariantExposures(point)) != 0 {
		return true
	}
	return len(facts.PostconditionRefinements(point)) != 0 || len(facts.PostconditionPathRelations(point)) != 0
}

type observationCapture struct {
	plan    ObservationPlan
	records map[cfg.Point]transfer.NodeObservation
	valid   map[cfg.Point]state.State
	stats   ObservationStats
}

func newObservationCapture(plan ObservationPlan) *observationCapture {
	return &observationCapture{
		plan:    plan,
		records: make(map[cfg.Point]transfer.NodeObservation, len(plan.nodePoints)),
		stats: ObservationStats{
			PlannedNodeOutputs:      len(plan.nodePoints),
			PlannedBoundaryOutputs:  len(plan.boundaryPoints),
			PlannedEdgeReachability: len(plan.edgeReachability),
		},
	}
}

func (c *observationCapture) record(record transfer.NodeObservation) {
	if c == nil || !c.plan.observesNode(record.Point) {
		return
	}
	c.records[record.Point] = record
}

func (c *observationCapture) finalize(finalVersion func(cfg.Point) uint64) {
	if c == nil || finalVersion == nil {
		return
	}
	for _, point := range c.plan.nodePoints {
		record, ok := c.records[point]
		if !ok {
			continue
		}
		c.stats.CapturedNodeOutputs++
		if finalVersion(point) != record.InputVersion {
			continue
		}
		valid := true
		for _, read := range record.Reads {
			if finalVersion(read.Point) != read.Version {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		if c.valid == nil {
			c.valid = make(map[cfg.Point]state.State, len(c.plan.boundaryPoints))
		}
		c.valid[point] = record.Output
		c.stats.ValidatedNodeOutputs++
		if c.plan.observesBoundary(point) {
			c.stats.CapturedBoundaryOutputs++
			c.stats.ValidatedBoundaryOutputs++
		}
	}
	// Records include working outputs and dynamic dependency versions. Neither
	// belongs to a published Result after validity is decided.
	c.records = nil
}

// sealObservations projects the final fixed point exactly once in plan order.
// It runs strictly after Solve/TryRun has completed both main worklist and
// narrowing. Consequently every transfer read observes the final state, even
// when the final bounded narrowing iteration changed a candidate after its
// last transfer evaluation.
func (r *Result) sealObservations() {
	if r == nil {
		return
	}
	plan := r.observationPlan
	if len(plan.nodePoints) == 0 && len(plan.edgeReachability) == 0 {
		if r.cfg != nil {
			plan = compileObservationPlan(r.cfg.Graph, r.facts, r.callOutcome != nil)
		}
	}
	stats := ObservationStats{
		PlannedNodeOutputs:      len(plan.nodePoints),
		PlannedBoundaryOutputs:  len(plan.boundaryPoints),
		PlannedEdgeReachability: len(plan.edgeReachability),
	}
	if r.observation.CapturedNodeOutputs != 0 || r.observation.ValidatedNodeOutputs != 0 {
		stats.CapturedNodeOutputs = r.observation.CapturedNodeOutputs
		stats.ValidatedNodeOutputs = r.observation.ValidatedNodeOutputs
		stats.CapturedBoundaryOutputs = r.observation.CapturedBoundaryOutputs
		stats.ValidatedBoundaryOutputs = r.observation.ValidatedBoundaryOutputs
	}
	published := PublishedFacts{}
	if len(plan.boundaryPoints) != 0 {
		published.nodeOutputs = make(map[cfg.Point]state.State, len(plan.boundaryPoints))
	}
	if len(plan.edgeReachability) != 0 {
		published.edgeNormal = make(map[observationEdge]bool, len(plan.edgeReachability))
	}
	if r.registry == nil || r.cfg == nil || r.cfg.Graph == nil || r.boundaryXfer == nil {
		r.published = published
		r.observation = stats
		return
	}

	graph := r.cfg.Graph
	domain, err := state.TryDomainWithOptionalLanes(r.registry, r.stateLanes)
	if err != nil {
		domain = state.Domain(r.registry)
	}
	outputs := r.capturedNodeOutputs
	if outputs == nil {
		outputs = make(map[cfg.Point]state.State, len(plan.boundaryPoints))
	}
	outputAt := func(point cfg.Point) (state.State, bool) {
		if out, ok := outputs[point]; ok {
			return out, true
		}
		in, ok := r.solvedStateAt(point)
		if !ok {
			return state.State{}, false
		}
		out := r.boundaryXfer(transfer.NodeContext{
			Graph:    graph,
			Registry: r.registry,
			Point:    point,
			Node:     graph.Node(point),
			Read:     r.stateRead,
		}, in)
		outputs[point] = out
		stats.RecomputedNodeOutputs++
		if plan.observesBoundary(point) {
			stats.RecomputedBoundaryOutputs++
		}
		return out, true
	}

	for _, point := range plan.boundaryPoints {
		if out, ok := outputAt(point); ok {
			published.nodeOutputs[point] = out
			stats.ProjectedBoundaryOutputs++
		}
	}
	if r.edgeXfer != nil {
		for _, edge := range plan.edgeReachability {
			stats.ProjectedEdgeReachability++
			in, ok := r.solvedStateAt(edge.from)
			if !ok || domain.Equal(state.NormalizeForDomain(domain, in), domain.Bottom()) {
				published.edgeNormal[edge] = false
				continue
			}
			out, ok := outputAt(edge.from)
			if !ok {
				published.edgeNormal[edge] = false
				continue
			}
			cond, hasCond := graph.EdgeCond(edge.from, edge.to)
			hasCond = hasCond && graph.IsBranch(edge.from)
			out = r.edgeXfer(transfer.EdgeContext{
				Graph:    graph,
				Registry: r.registry,
				Edge:     cfg.Edge{From: edge.from, To: edge.to, Cond: cond},
				HasCond:  hasCond,
			}, out)
			published.edgeNormal[edge] = !domain.Equal(state.NormalizeForDomain(domain, out), domain.Bottom())
		}
	}
	r.published = published
	r.capturedNodeOutputs = nil
	r.observation = stats
}

// ObservationStats returns a copy of this body's seal counters.
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

func (r *Result) publishedEdgeNormal(from, to cfg.Point) (bool, bool) {
	if r == nil || r.published.edgeNormal == nil {
		return false, false
	}
	normal, ok := r.published.edgeNormal[observationEdge{from: from, to: to}]
	return normal, ok
}
