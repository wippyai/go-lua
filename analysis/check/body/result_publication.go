package body

import (
	"context"
	"fmt"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ResultEdge identifies one directed CFG edge in a stabilized result. Edge
// condition syntax remains owned by the prepared graph and is deliberately not
// repeated in the publication payload.
type ResultEdge struct {
	From cfg.Point
	To   cfg.Point
}

// StabilizedResultCoordinates is the scalar, correlation-forgotten boundary
// published by a completed body fixpoint. A guarded solver obtains each State
// by existentially joining that coordinate independently before constructing
// this value; it never enumerates a cross-product of guarded leaves.
//
// Every map is copied by publication. PointInputs contains the ordinary body
// Result surface. PlannedNodeOutputs covers ObservationPlan's node points,
// including branch-only outputs used to prove EdgeNormal; only boundary-consumed
// outputs are retained by Result. Reachability and edge facts are authoritative:
// publication never invokes a node or edge transfer to fill a missing value.
type StabilizedResultCoordinates struct {
	PointInputs         transfer.Result
	PlannedNodeOutputs  map[cfg.Point]state.State
	PointReachable      map[cfg.Point]bool
	NodeOutputReachable map[cfg.Point]bool
	EdgeNormal          map[ResultEdge]bool
	// CallOutcomes contains the exact already-specialized lexical outcome at
	// every call point. Empty outcomes are represented by an explicit map entry.
	// PublishResult validates completeness and never consults a provider to fill
	// a missing point.
	CallOutcomes map[cfg.Point]callpayload.CallOutcome
	// ReturnSlots is the explicit N5 normal-exit observation. The formal
	// publisher owns its projection; Result publication only installs it at the
	// public exit coordinate and never reconstructs it from terminal States.
	ReturnSlots map[int]product.Value
	// DiagnosticOutput is the body-level diagnostic component projected from
	// the exact same stabilized tuple as PointInputs and CallOutcomes. It is
	// detached during publication; Result never recovers it from Lua syntax.
	DiagnosticOutput callpayload.DiagnosticOutput
}

// ResultPublicationConfig binds already-stabilized coordinates to their
// prepared execution authority. Solve is consulted only for result
// lineage/statistics; publication never constructs or invokes a point
// transfer.
type ResultPublicationConfig struct {
	Coordinates             StabilizedResultCoordinates
	FormalPathValue         FormalPathValueObservation
	Solve                   SolveConfig
	SeededEntry             state.State
	Initial                 transfer.InitialState
	ApplicationDependencies []ApplicationDependency
}

// PublishResult constructs the canonical body Result directly from stabilized
// coordinates. It performs projection/publication only and therefore does not
// construct either generic point transfer.
func (f *ExecutionFactory) PublishResult(config ResultPublicationConfig) (*Result, error) {
	if f == nil || f.prepared == nil {
		return nil, fmt.Errorf("body: result publication requires a prepared execution factory")
	}
	plan := compileObservationPlan(f.Graph(), f.prepared.facts)
	return f.publishResult(config, plan, directPublicationObservationStats(plan))
}

func (f *ExecutionFactory) publishResult(
	config ResultPublicationConfig,
	plan ObservationPlan,
	stats ObservationStats,
) (*Result, error) {
	// Publication is transactional. A canceled replacement solve must not even
	// construct an observable Result carrying provider-backed fallback state.
	if err := observationContextErr(config.Solve.Context); err != nil {
		return nil, err
	}
	if _, ok := config.Coordinates.PointInputs[f.Graph().Entry()]; !ok {
		return nil, fmt.Errorf("body: stabilized result has no entry input")
	}
	result := f.newResult(config.Coordinates.PointInputs, plan)
	result.formalPathValue = config.FormalPathValue
	return f.completeResultPublication(result, config.Solve, config.SeededEntry, config.Initial, config.Coordinates, config.ApplicationDependencies, stats)
}

func (f *ExecutionFactory) completeResultPublication(
	result *Result,
	config SolveConfig,
	entry state.State,
	initial transfer.InitialState,
	coordinates StabilizedResultCoordinates,
	applicationDependencies []ApplicationDependency,
	observation ObservationStats,
) (*Result, error) {
	if result == nil || f == nil || f.prepared == nil {
		return nil, fmt.Errorf("body: result publication is incomplete")
	}
	if err := result.publishStabilizedCoordinates(coordinates); err != nil {
		return nil, err
	}
	return f.finishResultPublication(result, config, entry, initial, applicationDependencies, observation)
}

func (f *ExecutionFactory) finishResultPublication(
	result *Result,
	config SolveConfig,
	entry state.State,
	initial transfer.InitialState,
	applicationDependencies []ApplicationDependency,
	observation ObservationStats,
) (*Result, error) {
	if result == nil || f == nil || f.prepared == nil {
		return nil, fmt.Errorf("body: result publication is incomplete")
	}
	result.observation = observation
	if config.Stats != nil {
		addObservationStats(&config.Stats.Observation, observation)
	}
	lineage, err := computeResultVersionLineageWithApplications(f.prepared, config, entry, initial, applicationDependencies)
	if err != nil {
		return nil, err
	}
	result.resultVersion = lineage.ResultVersion()
	result.resultLineage = lineage
	return result, nil
}

func (r *Result) publishStabilizedCoordinates(coordinates StabilizedResultCoordinates) error {
	if r == nil || r.cfg == nil || r.cfg.Graph == nil {
		return fmt.Errorf("body: stabilized result has no prepared graph")
	}
	graph := r.cfg.Graph
	plan := r.observationPlan
	flow := make(transfer.Result, len(coordinates.PointInputs))
	pointReachable := make(map[cfg.Point]bool, graph.Size())
	for _, point := range cfg.RPOReadOnly(graph) {
		reachable, ok := coordinates.PointReachable[point]
		if !ok {
			return fmt.Errorf("body: stabilized result has no reachability fact for point %d", point)
		}
		pointReachable[point] = reachable
		if input, ok := coordinates.PointInputs[point]; ok {
			flow[point] = input
		}
	}
	// Manually assembled diagnostic Results historically permit sparse flow
	// points outside Graph.RPO. Preserve that readmodel contract; the exported
	// factory publisher separately requires the prepared entry coordinate.
	for point, input := range coordinates.PointInputs {
		if _, ok := flow[point]; ok {
			continue
		}
		if _, ok := coordinates.PointReachable[point]; !ok {
			return fmt.Errorf("body: stabilized result has no reachability fact for point %d", point)
		}
		flow[point] = input
		pointReachable[point] = coordinates.PointReachable[point]
	}

	nodeOutputs := make(map[cfg.Point]state.State, len(plan.boundaryPoints))
	nodeOutputReachable := make(map[cfg.Point]bool, len(plan.boundaryPoints))
	for _, point := range plan.nodePoints {
		output, ok := coordinates.PlannedNodeOutputs[point]
		if !ok {
			return fmt.Errorf("body: stabilized result has no planned output for point %d", point)
		}
		reachable, ok := coordinates.NodeOutputReachable[point]
		if !ok {
			return fmt.Errorf("body: stabilized result has no output reachability fact for point %d", point)
		}
		if plan.observesBoundary(point) {
			nodeOutputs[point] = output
			nodeOutputReachable[point] = reachable
		}
	}

	edgeNormal := make(map[observationEdge]bool, len(plan.edgeReachability))
	for _, edge := range plan.edgeReachability {
		normal, ok := coordinates.EdgeNormal[ResultEdge{From: edge.from, To: edge.to}]
		if !ok {
			return fmt.Errorf("body: stabilized result has no normality fact for edge %d -> %d", edge.from, edge.to)
		}
		edgeNormal[edge] = normal
	}
	callOutcomes := make(map[cfg.Point]callpayload.CallOutcome, r.facts.CallSiteCount())
	callSiteCount := 0
	// Points are dense CFG indices. Iterating the complete graph inventory,
	// rather than only reachable RPO points, makes the publication exact for
	// every lexical call site, including a statically unreachable one.
	for ordinal := 0; ordinal < graph.Size(); ordinal++ {
		point := cfg.Point(ordinal)
		if _, call := r.facts.CallSiteView(point); !call {
			continue
		}
		callSiteCount++
		outcome, ok := coordinates.CallOutcomes[point]
		if !ok {
			return fmt.Errorf("body: stabilized result has no exact call outcome for point %d", point)
		}
		callOutcomes[point] = outcome.Clone()
	}
	if callSiteCount != r.facts.CallSiteCount() {
		return fmt.Errorf("body: %d call-site facts lie outside the prepared graph", r.facts.CallSiteCount()-callSiteCount)
	}
	diagnostics := coordinates.DiagnosticOutput.Normalize(r.registry)
	if !diagnostics.Valid(r.registry) {
		return fmt.Errorf("body: stabilized result has malformed diagnostic output")
	}
	if err := r.publishFormalReturnSlots(flow, coordinates.ReturnSlots); err != nil {
		return err
	}
	r.flow = flow
	r.diagnosticOutput = diagnostics.Clone()
	r.published = PublishedFacts{
		nodeOutputs:         nodeOutputs,
		pointReachable:      pointReachable,
		nodeOutputReachable: nodeOutputReachable,
		edgeNormal:          edgeNormal,
		callOutcomes:        callOutcomes,
	}
	return nil
}

func observationContextErr(ctx context.Context) error {
	return cancellation.FromContext(ctx).Token().Err()
}

// publishFormalReturnSlots installs the already-projected N5 return
// observation at the public normal-exit coordinate. It deliberately receives
// no point outputs: scanning terminal States here would recreate the retired
// N5 reconstruction bridge.
func (r *Result) publishFormalReturnSlots(flow transfer.Result, slots map[int]product.Value) error {
	if r == nil || r.registry == nil || r.cfg == nil || r.cfg.Graph == nil {
		return fmt.Errorf("body: formal return-slot publication is unowned")
	}
	exit := r.cfg.Graph.Exit()
	exitState, normalExit := flow[exit]
	if !normalExit {
		return nil
	}
	exitValues := exitState.ValuesSnapshot()
	if exitValues.Top {
		return fmt.Errorf("body: normal exit has a non-finite value lane")
	}
	edit := exitState.EditValues(r.registry)
	for slot := range exitValues.Values {
		if index, returnSlot := statekey.ParseReturnSlot(slot); returnSlot {
			edit.WriteReturnSlot(index, product.Bottom(r.registry))
		}
	}
	for index, value := range slots {
		edit.WriteReturnSlot(index, value)
	}
	flow[exit] = edit.DoneOn(exitState)
	return nil
}

// DiagnosticOutput returns detached body-level diagnostic evidence published
// from the stabilized tuple. It owns no solver, provider, or lexical syntax.
func (r *Result) DiagnosticOutput() callpayload.DiagnosticOutput {
	if r == nil {
		return callpayload.DiagnosticOutput{}
	}
	return r.diagnosticOutput.Clone()
}

func directPublicationObservationStats(plan ObservationPlan) ObservationStats {
	return ObservationStats{
		PlannedNodeOutputs:        len(plan.nodePoints),
		PlannedBoundaryOutputs:    len(plan.boundaryPoints),
		PlannedEdgeReachability:   len(plan.edgeReachability),
		ProjectedBoundaryOutputs:  len(plan.boundaryPoints),
		ProjectedEdgeReachability: len(plan.edgeReachability),
	}
}
