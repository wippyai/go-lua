// runtime_assembly.go declares the solver runtime vocabulary and runs the assembly pass.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/internal/linkexecutionplan"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// runtimeProducer is static Group metadata. Its candidate is explicitly
// epoch-local in executorEpoch, so no Group becomes a persistent executor.
type runtimeProducer struct {
	index int
	group equation.GroupNode
	plan  carrier.ContributionPlan
	// span addresses this Group's rows in the sealed program's member table. A
	// producer holds no member of its own; the span is retained even when its
	// output is outside the initial active mask.
	span            memberSpan
	inputProjection []runtimeInputProjection
	environment     *runtimeInput
	outputScope     carrier.Scope
	premise         support.Mask
	reads           []demand.Observation
	dynamicReads    []demand.DynamicRead
	carries         []demand.Carry
	footprint       []recurrenceFootprint
}

// runtimeInput is the one fully bound carrier transport for one immutable
// equation Input.  Provenance stays beside the plan so reverse demand and a
// later invalidation cut cannot mistake equal reindexes with different
// structural boundaries for one edge.
type runtimeInput struct {
	key        composition.Key
	provenance composition.Key
	pre        support.Mask
	plan       carrier.ReindexPlan
	post       support.Mask
}

func (input runtimeInput) valid() bool {
	return input.key.Available() && input.provenance.Available() && input.pre.Valid() && input.plan.Valid() && input.post.Valid()
}

// recurrenceFootprint is one occurrence-local recurrence membership. It is
// derived from a Group's attached members, never from a Factor-global target
// catalog, so distinct regions cannot widen one another's keys.
type recurrenceFootprint struct {
	key           composition.Key
	routeFactor   runtimeFactor
	route         bool
	narrowRoute   bool
	targets       []carrier.Target
	narrowTargets []carrier.Target
}

// runtimeRegion is the dense recurrence classification used only at its WTO
// head. external and back partition head producers; a mixed producer is back
// whenever any Group input is internal to the Region.
type runtimeRegion struct {
	active              bool
	head                int
	parent              int
	points              []int
	external            []int
	back                []int
	environmentExternal []int
	environmentBack     []int
	contextExternal     []int
	contextBack         []int
	factorExternal      []int
	factorBack          []int
	widen               carrier.MergeScope
	narrow              carrier.MergeScope
	// discharge is the Region's support-axis widening: the same recurrence
	// publication that widens values along region.widen coarsens the head's
	// guard partition along this relation.
	discharge regionDischarge
	// newton is the Region's closure basis, sealed only where the head's
	// recurrence is transport alone and its publication widens no Factor. It
	// is unavailable for every other Region, which keeps the exact solver path
	// it already has.
	newton regionNewton
}

type solverRuntime struct {
	topology *equation.Topology
	carrier  *carrier.Composition
	graph    *equation.Graph
	// contexts, contextIndex, and contextLayout are the Link-owned execution
	// address plane retained from the committed Program. The equation Graph and
	// its reusable point metadata remain singular; mutable semantic state is
	// addressed through contextLayout.StateOrdinal by the contextual executor.
	// A non-artifact engine program has none of these rows and is a distinct
	// engine construction, never a default context for mounted execution.
	contexts       executioncontext.Directory
	contextIndex   contextfiber.Index
	contextLayout  contextfiber.Layout
	pointOwners    []contextfiber.PointOwner
	artifactBacked bool
	// executionPlan is the immutable Link-owned projection from singular graph
	// points onto compact contextual state rows. It is derived once from the
	// committed point-transition witnesses; later executor migration consumes
	// this authority rather than reconstructing actor/library placement.
	executionPlan *linkexecutionplan.LinkExecutionPlan
	// program is the sealed row model: the member rows the executor folds, the
	// Factor records the overlay resolves, and the query and observation tables
	// the result readers project.
	program *runtimeProgram
	// execution overrides the graph-owned demanded event stream only after a
	// selected overlay introduces feedback over the same already-demanded Point
	// set. It contains no semantic facts; the live carrier remains unchanged.
	execution       *schedule.Schedule
	executionDemand *equation.Demand
	// stateExecution is the contextual schedule for an installed dynamic
	// overlay.  The immutable base schedule remains executionPlan.Schedule;
	// this view is built only by lifting an accepted overlay through that plan
	// and is never a graph-point schedule in artifact mode.
	stateExecution *schedule.Schedule
	// stateExecutionEvents is the demanded event view over stateExecution. The
	// immutable schedule retains every admitted state; this compact view keeps
	// inactive contextual regions out of the executor bracket walk.
	stateExecutionEvents []schedule.Event
	points               *equation.Demand
	producers            []runtimeProducer
	// producerRows is the compact admitted (StateOrdinal, GroupOrdinal)
	// mapping used by artifact epochs. The static producers slice remains one
	// row per singular graph Group and is never multiplied by state count.
	producerRows stateGroupIndex
	environments []runtimeEnvironment
	factorEdges  []runtimeFactorEdge
	// Incoming structural rows are canonical dense edge indices, assembled
	// once from the sealed Graph. Hot Point folding never resolves an edge by
	// semantic key or scans the global edge table.
	environmentIncoming [][]int
	factorIncoming      [][]int
	// stateFactorRows is the compact contextual transpose of factor edges.
	// Each row retains the singular graph edge metadata plus its exact source
	// and target StateOrdinal; artifact folds and wakes consult these rows,
	// while the graph CSR above remains metadata for the non-artifact engine
	// construction and detached compatibility inspection.
	stateFactorRows     []runtimeStateFactorRow
	stateFactorIncoming [][]int
	stateFactorOutgoing [][]int
	statePointRows      [][]int
	// contextTransports is the runtime lowering of the immutable Link plan's
	// authenticated cross-context transports. Incoming/outgoing rows are keyed
	// by exact StateOrdinal; no graph-point or module fallback is permitted.
	contextTransports        []runtimeContextTransport
	contextTransportIncoming [][]int
	contextTransportOutgoing [][]int
	// contextTransportSource is the sealed inverse at the source-selection
	// cut: (target StateOrdinal, source graph Point) names exactly one
	// authenticated source StateOrdinal. It is not a graph/global fallback.
	contextTransportSource map[[2]int]int
	overlay                runtimeStructuralOverlay
	demand                 *demand.Plan
	pointScopes            []carrier.Scope
	pointInitials          []support.Mask
	regions                []runtimeRegion
	// operands is the sealed transpose of the recurrence and Group-input
	// operand rows. It is re-derived wherever those rows are, and it is the
	// sole authority for which reader a published value makes stale.
	operands       *operandPlane
	regionChildren [][]int // operational traversal cache derived from immutable Region.Parent
	pointRegion    []int
	activePoints   []bool
	activeStates   []bool
	activeRegions  []bool
	// stateSelected is the artifact demand closure. It is a compact list of
	// admitted StateOrdinal rows, not a Context×Point or State×Group product.
	stateSelected []int
	// publication is the sealed Snapshot authority for this runtime generation:
	// its column writes, result key universes, and point denominator/index are
	// all constructed once at assembly and borrowed by every solve epoch.
	publication *solvedPublicationPlan

	retained  *carrier.RetainedWork
	completed *State
}

type runtimeEnvironment struct {
	index  int
	source int
	target int
	input  runtimeInput
}

type runtimeFactorEdge struct {
	index  int
	key    composition.Key
	factor composition.Key
	source int
	target int
	input  runtimeInput
	slot   shape.Slot
	// activation is the sealed candidate branch this edge instantiates. An
	// edge that carries one was authenticated exactly once, against the Link
	// directory, the point layout, and the execution plan together; every
	// consumer below reads its endpoint Contexts and States off the row rather
	// than deriving them again from the transition tuple. A static transport
	// edge carries no branch, which is what says it is not an activation.
	activation execution.ActivationRow
}

// runtimeStateFactorRow is one admitted contextual occurrence of a singular
// runtime factor edge.  The edge index points back to immutable graph-edge
// metadata; source/target are the mutable executor coordinates.
type runtimeStateFactorRow struct {
	edge   int
	source int
	target int
}

// runtimeContextTransport is one exact Link-bound semantic transport after the
// equation relation has crossed the single equation-to-carrier cut. The source
// and target StateOrdinal rows are immutable addresses; the carrier plan is
// lowered from the authenticated ContextTransport reindex and carries no
// caller-selected context or graph-point fallback.
type runtimeContextTransport struct {
	from, to                     int
	sourcePoint, targetPoint     int
	sourceContext, targetContext contextfiber.ContextOrdinal
	plan                         carrier.ReindexPlan
	pre, post                    support.Mask
}

func (transport runtimeContextTransport) validFor(runtime *carrier.Composition, stateCount, pointCount int) bool {
	return runtime != nil && transport.from >= 0 && transport.to >= 0 && transport.from < stateCount && transport.to < stateCount &&
		transport.sourcePoint >= 0 && transport.targetPoint >= 0 && transport.sourcePoint < pointCount && transport.targetPoint < pointCount &&
		transport.plan.Valid() && transport.pre.Valid() && transport.post.Valid() && transport.pre.Manager() == runtime.Guards() && transport.post.Manager() == runtime.Guards()
}

// runtimeFactorOrigin is the stable structural identity of one selected
// transport. Its input precondition is deliberately absent: activation
// evidence can widen that condition monotonically while preserving every
// other transport coordinate.
type runtimeFactorOrigin struct {
	source, target     int
	factor, provenance composition.Key
	reindex            composition.Key
	post               guard.FormulaID
	context            equation.ActivationContext
}

// runtimeStructuralOverlay owns only executable selected-edge indexing.  It
// is not an activation relation: Solver.accepted remains the sole authority
// for accepted Members and their premise union.  `reindexes` is likewise a
// derived carrier binding cache over already-issued scopes and atoms, never a
// second equation or carrier authoring surface.
type runtimeStructuralOverlay struct {
	staticOrigins map[runtimeFactorOrigin]struct{}
	originAt      map[runtimeFactorOrigin]int
	// directAt retains only the exact equation descriptors for installed
	// selected direct edges. It is the compact bridge needed when a later
	// frontier first creates feedback; runtime.factorEdges alone has already
	// lowered those descriptors to carrier rows.
	directAt        map[int]equation.SelectedStructuralFactorEdge
	factorOutgoing  [][]int // all static and selected factor transports
	dependencyEdges []schedule.Edge
	dependencyAt    map[[2]int]struct{}
	reindexes       runtimeReindexes
	latePlans       map[composition.Key]carrier.ReindexPlan
	// generation stamps installed selected-edge state. It fences prepared
	// overlays against a runtime that has moved on; it is not the activation
	// relation stamp, which orders a different lifetime.
	generation identity.Generation
}

func runtimeFactorEdgeOrigin(source, target int, factor, provenance, reindex composition.Key, post support.Mask, context equation.ActivationContext) (runtimeFactorOrigin, bool) {
	postIdentity, postOK := post.Identity()
	origin := runtimeFactorOrigin{source: source, target: target, factor: factor, provenance: provenance, reindex: reindex, post: postIdentity, context: context}
	return origin, source >= 0 && target >= 0 && factor.Available() && provenance.Available() && reindex.Available() && postOK && context.WellFormed() && origin.available()
}

func (origin runtimeFactorOrigin) available() bool {
	return origin.source >= 0 && origin.target >= 0 && origin.factor.Available() && origin.provenance.Available() && origin.reindex.Available() && origin.post.Available() && origin.context.WellFormed()
}

// assembleRuntimeOwned lowers one sealed program into the executable runtime.
// Every member answer it needs has already been taken by the binder: the folds
// carry the Group aggregates, the program carries the rows, and no draft
// reaches this pass.
func assembleRuntimeOwned(graph *equation.Graph, runtime *carrier.Composition, program *runtimeProgram, folds []memberFold, contexts executioncontext.Directory, contextIndex contextfiber.Index, contextLayout contextfiber.Layout, pointOwners []contextfiber.PointOwner, pointTransitions []ProgramPointTransition, artifactBacked bool) (*solverRuntime, bool) {
	if graph == nil || runtime == nil || runtime.Guards() == nil || !program.valid() || len(folds) != graph.GroupCount() || program.groupCount() != graph.GroupCount() {
		return nil, false
	}
	var executionPlan *linkexecutionplan.LinkExecutionPlan
	if artifactBacked {
		generation := contextLayout.Generation()
		if !contexts.Available() || len(pointOwners) != graph.PointCount() || !generation.Available() ||
			!contextIndex.OwnedBy(contexts, graph.PointCount(), generation) ||
			!contextLayout.OwnedBy(contextIndex, contexts, pointOwners, generation) {
			return nil, false
		}
		boundEdges := make([]linkexecutionplan.BoundEdge, len(pointTransitions))
		for index, transition := range pointTransitions {
			if !transition.available {
				return nil, false
			}
			activation, activationOK := contexts.ActivationEdge(transition.FromContextID(), transition.ToContextID())
			if !activationOK {
				return nil, false
			}
			edge, bound := linkexecutionplan.NewBoundEdge(graph, contextLayout, contexts, transition.SourcePoint(), transition.TargetPoint(), linkexecutionplan.BoundEdgeSpec{
				TransitionID: activation.ID(), GenerationID: transition.GenerationID(),
				FromContextID: transition.FromContextID(), ToContextID: transition.ToContextID(),
			})
			if !bound {
				return nil, false
			}
			boundEdges[index] = edge
		}
		var planned bool
		executionPlan, planned = linkexecutionplan.New(graph, contextLayout, contexts, boundEdges)
		if !planned {
			return nil, false
		}
	} else if contexts.Available() || contextIndex.Available() || contextLayout.Available() || len(pointOwners) != 0 || len(pointTransitions) != 0 {
		return nil, false
	}
	observationPoints := make([]equation.Point, program.observationCount())
	for index := range observationPoints {
		observation, observed := program.observationAt(index)
		if !observed {
			return nil, false
		}
		point, pointOK := graph.PointAt(schedule.Node(observation.point))
		if !pointOK || !graph.OwnsPoint(point) {
			return nil, false
		}
		observationPoints[index] = point
	}
	points, ok := graph.DemandWithPoints(observationPoints)
	if !ok || points == nil || points.PointCount() == 0 || points.EventCount() == 0 {
		return nil, false
	}
	activePoints, activeRegions, activeOK := runtimeDemandMembership(graph, points)
	if !activeOK {
		return nil, false
	}
	plans, ok := bindRuntimeReindexes(graph, runtime)
	if !ok {
		return nil, false
	}
	pointScopes := make([]carrier.Scope, graph.PointCount())
	pointInitials := make([]support.Mask, graph.PointCount())
	for index := range pointScopes {
		point, pointOK := graph.PointAt(schedule.Node(index))
		scope, scoped := plans.scope(point.Scope())
		init, disposition, initialized := point.Init()
		mask, masked := runtimeFormula(runtime, scope, init, plans.decisions)
		if !pointOK || !scoped || !initialized || !masked || disposition != equation.InitAbsent && disposition != equation.InitPresent {
			return nil, false
		}
		pointScopes[index], pointInitials[index] = scope, mask
	}
	var contextTransports []runtimeContextTransport
	var contextTransportIncoming, contextTransportOutgoing [][]int
	var contextTransportSource map[[2]int]int
	if artifactBacked {
		var transportsOK bool
		contextTransports, contextTransportIncoming, contextTransportOutgoing, contextTransportSource, transportsOK = bindRuntimeContextTransports(graph, executionPlan, runtime, plans)
		if !transportsOK {
			return nil, false
		}
	}
	producers := make([]runtimeProducer, graph.GroupCount())
	environments := make([]runtimeEnvironment, graph.EnvironmentEdgeTotal())
	for edgeIndex := 0; edgeIndex < graph.EnvironmentEdgeTotal(); edgeIndex++ {
		edge, edgeOK := graph.EnvironmentEdgeAtIndex(edgeIndex)
		sourcePoint, sourceIndexed := graph.PointIndex(edge.Input().Point())
		targetPoint, targetIndexed := graph.PointIndex(edge.Target())
		if !edgeOK || !sourceIndexed || !targetIndexed {
			return nil, false
		}
		plan, planOK := plans.plan(edge.Input().Reindex())
		sourceScope, sourceScoped := plans.scope(edge.Input().Source().Scope())
		targetScope, targetScoped := plans.scope(edge.Input().Target().Scope())
		pre, preOK := runtimeFormula(runtime, sourceScope, edge.Input().Pre(), plans.decisions)
		post, postOK := runtimeFormula(runtime, targetScope, edge.Input().Post(), plans.decisions)
		if !planOK || !sourceScoped || !targetScoped || !preOK || !postOK || !edge.Input().Key().Available() || !edge.Input().Provenance().Available() {
			return nil, false
		}
		bound := runtimeInput{key: edge.Input().Key(), provenance: edge.Input().Provenance(), pre: pre, plan: plan, post: post}
		if !bound.valid() {
			return nil, false
		}
		environments[edgeIndex] = runtimeEnvironment{index: edgeIndex, source: sourcePoint, target: targetPoint, input: bound}
	}
	factorEdges := make([]runtimeFactorEdge, graph.FactorEdgeTotal())
	// Selected-origin identity is cold overlay indexing, not part of the hot
	// fold/wake edge header.  Keep static origins here only to reject a later
	// selected edge that would alias an already graph-owned transport.
	staticOrigins := make(map[runtimeFactorOrigin]struct{}, len(factorEdges))
	for edgeIndex := 0; edgeIndex < graph.FactorEdgeTotal(); edgeIndex++ {
		edge, edgeOK := graph.FactorEdgeAtIndex(edgeIndex)
		sourcePoint, sourceIndexed := graph.PointIndex(edge.Input().Point())
		targetPoint, targetIndexed := graph.PointIndex(edge.Target())
		record, factorKnown := program.factorRecordByKey(edge.Factor())
		if !edgeOK || !edge.Key().Available() || !sourceIndexed || !targetIndexed || !factorKnown || record.slot < 0 || int(record.slot) >= runtime.Count() {
			return nil, false
		}
		slot := record.slot
		plan, planOK := plans.plan(edge.Input().Reindex())
		sourceScope, sourceScoped := plans.scope(edge.Input().Source().Scope())
		targetScope, targetScoped := plans.scope(edge.Input().Target().Scope())
		pre, preOK := runtimeFormula(runtime, sourceScope, edge.Input().Pre(), plans.decisions)
		post, postOK := runtimeFormula(runtime, targetScope, edge.Input().Post(), plans.decisions)
		if !planOK || !sourceScoped || !targetScoped || !preOK || !postOK || !edge.Input().Key().Available() || !edge.Input().Provenance().Available() {
			return nil, false
		}
		bound := runtimeInput{key: edge.Input().Key(), provenance: edge.Input().Provenance(), pre: pre, plan: plan, post: post}
		if !bound.valid() {
			return nil, false
		}
		origin, originOK := runtimeFactorEdgeOrigin(sourcePoint, targetPoint, edge.Factor(), edge.Input().Provenance(), edge.Input().Reindex().Key(), bound.post, equation.ActivationContext{})
		if !originOK {
			return nil, false
		}
		staticOrigins[origin] = struct{}{}
		factorEdges[edgeIndex] = runtimeFactorEdge{index: edgeIndex, key: edge.Key(), factor: edge.Factor(), source: sourcePoint, target: targetPoint, input: bound, slot: slot}
	}
	environmentIncoming := make([][]int, graph.PointCount())
	for edgeIndex, edge := range environments {
		if edge.source < 0 || edge.target < 0 || edge.target >= len(environmentIncoming) {
			return nil, false
		}
		environmentIncoming[edge.target] = append(environmentIncoming[edge.target], edgeIndex)
	}
	factorIncoming := make([][]int, graph.PointCount())
	factorOutgoing := make([][]int, graph.PointCount())
	for edgeIndex, edge := range factorEdges {
		if !edge.key.Available() || !edge.factor.Available() || edge.source < 0 || edge.source >= len(factorOutgoing) || edge.target < 0 || edge.target >= len(factorIncoming) {
			return nil, false
		}
		factorIncoming[edge.target] = append(factorIncoming[edge.target], edgeIndex)
		factorOutgoing[edge.source] = append(factorOutgoing[edge.source], edgeIndex)
	}
	for index := 0; index < graph.GroupCount(); index++ {
		group, groupOK := graph.HyperedgeAt(index)
		groupIndex, indexed := graph.GroupIndex(group)
		outputIndex, outputIndexed := graph.PointIndex(group.Output())
		if !groupOK || !indexed || groupIndex != index || !graph.OwnsGroup(group) || !group.Key().Available() || !outputIndexed || outputIndex < 0 || outputIndex >= len(activePoints) {
			return nil, false
		}
		span, spanOK := program.groupSpanAt(index)
		if !spanOK || span.count() != group.MemberCount() {
			return nil, false
		}
		outputScope, scoped := plans.scope(group.Output().Scope())
		premise, premised := runtimeFormula(runtime, outputScope, group.Premise(), plans.decisions)
		if !scoped || !premised {
			return nil, false
		}
		inputTransports := make([]runtimeInput, group.InputCount())
		for inputIndex := range inputTransports {
			input, inputOK := group.InputAt(inputIndex)
			if !inputOK || !input.Point().Available() {
				return nil, false
			}
			plan, planOK := plans.plan(input.Reindex())
			sourceScope, sourceScoped := plans.scope(input.Source().Scope())
			pre, preOK := runtimeFormula(runtime, sourceScope, input.Pre(), plans.decisions)
			post, postOK := runtimeFormula(runtime, outputScope, input.Post(), plans.decisions)
			if !planOK || !sourceScoped || !preOK || !postOK || !input.Key().Available() || !input.Provenance().Available() {
				return nil, false
			}
			inputTransports[inputIndex] = runtimeInput{key: input.Key(), provenance: input.Provenance(), pre: pre, plan: plan, post: post}
			if !inputTransports[inputIndex].valid() {
				return nil, false
			}
		}
		if !validateRuntimeInputReads(program, span, group.InputCount()) {
			return nil, false
		}
		inputProjection, projectionOK := sealRuntimeInputProjection(graph, executionPlan, group, inputTransports)
		if !projectionOK {
			return nil, false
		}
		var environment *runtimeInput
		if environmentInput, environmentOK := group.EnvironmentInput(); environmentOK {
			plan, planOK := plans.plan(environmentInput.Reindex())
			sourceScope, sourceScoped := plans.scope(environmentInput.Source().Scope())
			pre, preOK := runtimeFormula(runtime, sourceScope, environmentInput.Pre(), plans.decisions)
			post, postOK := runtimeFormula(runtime, outputScope, environmentInput.Post(), plans.decisions)
			if !planOK || !sourceScoped || !preOK || !postOK || !environmentInput.Key().Available() || !environmentInput.Provenance().Available() {
				return nil, false
			}
			bound := runtimeInput{key: environmentInput.Key(), provenance: environmentInput.Provenance(), pre: pre, plan: plan, post: post}
			if !bound.valid() {
				return nil, false
			}
			environment = &bound
		}
		fold := folds[index]
		plan, planOK := runtime.SealContribution(group.InputCount(), fold.writes, fold.sources, environment != nil)
		if planOK {
			plan, planOK = plan.SealCarryExclusions(fold.carryExclusions)
		}
		if !planOK {
			return nil, false
		}
		producers[index] = runtimeProducer{index: index, group: group, plan: plan, span: span, inputProjection: inputProjection, environment: environment, outputScope: outputScope, premise: premise, reads: fold.initialReads, dynamicReads: fold.dynamicReads, carries: fold.carries, footprint: fold.footprint}
	}
	stateFactorIncoming, stateFactorOutgoing, stateFactorRows, statePointRows, stateFactorOK := buildStateFactorIndex(graph, executionPlan, factorEdges, artifactBacked)
	if !stateFactorOK {
		return nil, false
	}
	regions, regionChildren, recurrenceOK := bindRuntimeRegions(graph, activeRegions, runtime, producers, plans)
	if !recurrenceOK {
		return nil, false
	}
	pointRegion := make([]int, graph.PointCount())
	for index := range pointRegion {
		point, pointOK := graph.PointAt(schedule.Node(index))
		region, regionOK := graph.PointRegion(point)
		if !pointOK || !regionOK {
			return nil, false
		}
		pointRegion[index] = region
	}
	for index := range pointRegion {
		if pointRegion[index] == schedule.NoRegion {
			continue
		}
		if pointRegion[index] < 0 || pointRegion[index] >= len(regions) {
			return nil, false
		}
	}
	demandPlan := demand.NewPlan(graph, runtime)
	if demandPlan == nil {
		return nil, false
	}
	selected := make([]int, points.PointCount())
	for index := range selected {
		point, pointOK := points.PointAt(index)
		pointIndex, indexed := graph.PointIndex(point)
		if !pointOK || !indexed {
			return nil, false
		}
		selected[index] = pointIndex
		for producerIndex := 0; producerIndex < graph.ProducerCount(point); producerIndex++ {
			producer, producerOK := graph.ProducerAt(point, producerIndex)
			groupIndex, indexed := graph.GroupIndex(producer)
			if !producerOK || !indexed || groupIndex < 0 || groupIndex >= len(producers) {
				return nil, false
			}
		}
	}
	// Declare every dense Group family. Seal still projects the initial active
	// subset, but no later demand/activation revision needs to reconstruct the
	// family metadata or rebind its carrier transports.
	for groupIndex := range producers {
		producer := producers[groupIndex]
		inputs := make([]equation.Point, producer.group.InputCount())
		inputIndexes := make([]int, len(inputs))
		for inputIndex := range inputs {
			input, ok := producer.group.InputAt(inputIndex)
			pointIndex, indexed := graph.PointIndex(input.Point())
			if !ok || !indexed {
				return nil, false
			}
			inputs[inputIndex] = input.Point()
			inputIndexes[inputIndex] = pointIndex
		}
		if !demandPlan.Declare(demand.Family{Group: groupIndex, Inputs: inputIndexes, InitialReads: producer.reads, DynamicReads: producer.dynamicReads, Carries: producer.carries}) {
			return nil, false
		}
	}
	sealedDemand := demandPlan.Seal(selected)
	if !sealedDemand {
		return nil, false
	}
	dependencyEdges, dependencyAt, dependencyOK := runtimeStaticDependencyEdges(graph)
	if !dependencyOK {
		return nil, false
	}
	producerRows, activeStates, producerRowsOK := buildStateGroupIndex(graph, executionPlan, artifactBacked, activePoints)
	if !producerRowsOK {
		return nil, false
	}
	stateSelected := []int(nil)
	if artifactBacked {
		var demandOK bool
		activeStates, activePoints, stateSelected, demandOK = buildArtifactStateDemand(graph, program, executionPlan, activePoints)
		if !demandOK {
			return nil, false
		}
	}
	assembled := &solverRuntime{carrier: runtime, graph: graph, program: program, contexts: contexts, contextIndex: contextIndex, contextLayout: contextLayout, pointOwners: append([]contextfiber.PointOwner(nil), pointOwners...), artifactBacked: artifactBacked, executionPlan: executionPlan, points: points, producers: producers, environments: environments, factorEdges: factorEdges, environmentIncoming: environmentIncoming, factorIncoming: factorIncoming, stateFactorRows: stateFactorRows, stateFactorIncoming: stateFactorIncoming, stateFactorOutgoing: stateFactorOutgoing, statePointRows: statePointRows, contextTransports: contextTransports, contextTransportIncoming: contextTransportIncoming, contextTransportOutgoing: contextTransportOutgoing, contextTransportSource: contextTransportSource, overlay: runtimeStructuralOverlay{staticOrigins: staticOrigins, originAt: make(map[runtimeFactorOrigin]int), directAt: make(map[int]equation.SelectedStructuralFactorEdge), factorOutgoing: factorOutgoing, dependencyEdges: dependencyEdges, dependencyAt: dependencyAt, reindexes: plans, latePlans: make(map[composition.Key]carrier.ReindexPlan), generation: 1}, demand: demandPlan, pointScopes: pointScopes, pointInitials: pointInitials, regions: regions, regionChildren: regionChildren, pointRegion: pointRegion, activePoints: activePoints, activeRegions: activeRegions}
	assembled.producerRows = producerRows
	assembled.activeStates = activeStates
	assembled.stateSelected = stateSelected
	if artifactBacked {
		stateRegions, stateChildren, statePointRegion, stateActiveRegions, stateEvents, lifted := liftStateRegions(graph, executionPlan.Schedule(), activeStates, assembled, stateFactorRows, false)
		if !lifted {
			return nil, false
		}
		assembled.regions = stateRegions
		assembled.regionChildren = stateChildren
		assembled.pointRegion = statePointRegion
		assembled.activeRegions = stateActiveRegions
		assembled.stateExecutionEvents = stateEvents
	}
	operandRegions := regions
	if artifactBacked {
		operandRegions = assembled.regions
	}
	var operands *operandPlane
	var planed bool
	if artifactBacked {
		operands, planed = buildStateOperandPlane(assembled, stateFactorSources(stateFactorRows), operandRegions)
	} else {
		operands, planed = buildOperandPlane(graph, producers, environments, installedFactorSources(factorEdges), operandRegions)
	}
	if !planed {
		return nil, false
	}
	assembled.operands = operands
	plan, sealed := sealSolvedPublicationPlan(assembled)
	if !sealed {
		return nil, false
	}
	assembled.publication = plan
	return assembled, true
}
