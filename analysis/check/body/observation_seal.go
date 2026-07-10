package body

import (
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ObservationPlan is the prepared-body query surface compiled before a result
// is published. It deliberately describes consumers, not an index of every
// solver point: boundary outputs are retained only for facts whose consumers
// need same-node effects, while edge observations are reduced to booleans.
//
// The current implementation uses the deterministic post-fixpoint projection
// path. This is the sound fallback for a by-value State solver: a final
// narrowing pass can replace a point state without another node-transfer
// evaluation, so a last-worklist capture cannot be trusted without state and
// dynamic-summary version identities.
type ObservationPlan struct {
	boundaryPoints   []cfg.Point
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

func compileObservationPlan(r *Result) ObservationPlan {
	if r == nil || r.cfg == nil || r.cfg.Graph == nil {
		return ObservationPlan{}
	}
	graph := r.cfg.Graph
	plan := ObservationPlan{
		boundaryPoints:   make([]cfg.Point, 0, graph.Size()),
		edgeReachability: make([]observationEdge, 0, graph.Size()),
	}
	// RPO and SuccessorsReadOnly are both canonical. Keeping this order makes
	// projection independent of map iteration and matches transfer.Run.
	for _, point := range graph.RPO() {
		if r.needsBoundaryNodeOutput(point) {
			plan.boundaryPoints = append(plan.boundaryPoints, point)
		}
		// FactsEdgeTransfer is identity on non-branch edges. Their normality is
		// therefore represented by the input reachability (or a planned same-node
		// no-normal-return output), not a separately retained edge record.
		if graph.IsBranch(point) {
			for _, successor := range cfg.SuccessorsReadOnly(graph, point) {
				plan.edgeReachability = append(plan.edgeReachability, observationEdge{from: point, to: successor})
			}
		}
	}
	return plan
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
	plan := compileObservationPlan(r)
	stats := ObservationStats{
		PlannedBoundaryOutputs:  len(plan.boundaryPoints),
		PlannedEdgeReachability: len(plan.edgeReachability),
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
	outputs := make(map[cfg.Point]state.State, len(plan.boundaryPoints))
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
