// runtime_assembly.go declares the solver runtime vocabulary and runs the assembly pass.

package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/identity"
)

// runtimeProducer is static Group metadata. Its candidate is explicitly
// epoch-local in executorEpoch, so no Group becomes a persistent executor.
type runtimeProducer struct {
	index        int
	group        equation.GroupNode
	plan         carrier.ContributionPlan
	members      []runtimeMember
	inputs       []runtimeInput
	environment  *runtimeInput
	outputScope  carrier.Scope
	premise      support.Mask
	reads        []demand.Observation
	dynamicReads []demand.DynamicRead
	carries      []demand.Carry
	footprint    []recurrenceFootprint
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
	faces               []int
	external            []int
	back                []int
	environmentExternal []int
	environmentBack     []int
	factorExternal      []int
	factorBack          []int
	widen               carrier.MergeScope
	narrow              carrier.MergeScope
}

type solverRuntime struct {
	// receiptState/receiptAuthority are the SchemaBinding-native runtime
	// authority. Receipt compilation never fabricates a cold declaration owner.
	receiptState     *schemaBindingState
	receiptAuthority *schemaBindingAuthority
	topology         *equation.Topology
	carrier          *carrier.Composition
	graph            *equation.Graph
	// execution overrides the graph-owned demanded event stream only after a
	// selected overlay introduces feedback over the same already-demanded Point
	// set. It contains no semantic facts; the live carrier remains unchanged.
	execution       *schedule.Schedule
	executionDemand *equation.Demand
	factors         []runtimeFactor // concrete bound operations; cold surface catalogs are released
	points          *equation.Demand
	producers       []runtimeProducer
	environments    []runtimeEnvironment
	factorEdges     []runtimeFactorEdge
	// Incoming structural rows are canonical dense edge indices, assembled
	// once from the sealed Graph. Hot Point folding never resolves an edge by
	// semantic key or scans the global edge table.
	environmentIncoming [][]int
	factorIncoming      [][]int
	overlay             runtimeStructuralOverlay
	demand              *demand.Plan
	queries             []runtimeQuery
	// observations are optional solve-local read-only projections. They are
	// attached after the reusable topology is committed, never participate in
	// demand or rule execution, and are empty on the ordinary solve path.
	observations   []runtimeObservation
	pointScopes    []carrier.Scope
	pointInitials  []support.Mask
	regions        []runtimeRegion
	regionChildren [][]int // operational traversal cache derived from immutable Region.Parent
	pointRegion    []int
	activePoints   []bool
	activeRegions  []bool

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
}

// runtimeStructuralOverlay owns only executable selected-edge indexing.  It
// is not an activation relation: Solver.accepted remains the sole authority
// for accepted Members and their premise union.  `reindexes` is likewise a
// derived carrier binding cache over already-issued scopes and atoms, never a
// second equation or carrier authoring surface.
type runtimeStructuralOverlay struct {
	factorByKey   map[composition.Key]runtimeFactor
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

func runtimeFactorEdgeOrigin(source, target int, factor, provenance, reindex composition.Key, post support.Mask) (runtimeFactorOrigin, bool) {
	postIdentity, postOK := post.Identity()
	origin := runtimeFactorOrigin{source: source, target: target, factor: factor, provenance: provenance, reindex: reindex, post: postIdentity}
	return origin, source >= 0 && target >= 0 && factor.Available() && provenance.Available() && reindex.Available() && postOK && origin.available()
}

func (origin runtimeFactorOrigin) available() bool {
	return origin.source >= 0 && origin.target >= 0 && origin.factor.Available() && origin.provenance.Available() && origin.reindex.Available() && origin.post.Available()
}

// assembleReceiptRuntime enters the common executable assembly with the
// exact sealed SchemaBinding authority already pinned by receipt compilation.
// It is deliberately disjoint from cold declaration capabilities.
func assembleReceiptRuntime(state *schemaBindingState, authority *schemaBindingAuthority, graph *equation.Graph, runtime *carrier.Composition, factors map[composition.Key]runtimeFactor, rows []runtimeMember, queries []runtimeQuery, observations []runtimeObservation) (*solverRuntime, bool) {
	return assembleRuntimeOwned(state, authority, graph, runtime, factors, rows, queries, observations)
}

func assembleRuntimeOwned(receiptState *schemaBindingState, receiptAuthority *schemaBindingAuthority, graph *equation.Graph, runtime *carrier.Composition, factors map[composition.Key]runtimeFactor, rows []runtimeMember, queries []runtimeQuery, observations []runtimeObservation) (*solverRuntime, bool) {
	if receiptState == nil || receiptAuthority == nil || receiptState.phase != schemaBindingSealed || receiptState.authority != receiptAuthority || receiptState.schema == nil || !receiptState.schema.Available() || graph == nil || runtime == nil || runtime.Guards() == nil || factors == nil {
		return nil, false
	}
	schema := receiptState.schema
	if schema == nil || graph.CompositionID() != schema.coldID() {
		return nil, false
	}
	observationPoints := make([]equation.Point, len(observations))
	for index, observation := range observations {
		if observation == nil {
			return nil, false
		}
		point := observation.observationPoint()
		if !graph.OwnsPoint(point) {
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
	// A graph Group becomes a hot runtime producer only when its exact output
	// Point is in this solve's demand closure. The immutable graph keeps every
	// reusable Program row for future observation/activation revisions, while
	// disconnected interiors retain identity only and allocate no contribution
	// plan, read set, or recurrence footprint in this runtime generation.
	selectedGroups := make([]bool, graph.GroupCount())
	for index := range selectedGroups {
		group, groupOK := graph.HyperedgeAt(index)
		outputIndex, outputOK := graph.PointIndex(group.Output())
		if !groupOK || !outputOK || outputIndex < 0 || outputIndex >= len(activePoints) {
			return nil, false
		}
		selectedGroups[index] = activePoints[outputIndex]
	}
	byMember := make(map[composition.Key]runtimeMember, len(rows))
	for _, row := range rows {
		if row == nil {
			return nil, false
		}
		member := row.member()
		if !member.Key().Available() || byMember[member.Key()] != nil {
			return nil, false
		}
		byMember[member.Key()] = row
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
		factor, factorKnown := factors[edge.Factor()]
		slot, slotOK := shape.Slot(0), false
		if factorKnown && factor != nil {
			slot, slotOK = factor.runtimeSlot()
		}
		if !edgeOK || !edge.Key().Available() || !sourceIndexed || !targetIndexed || !factorKnown || factor == nil || compositionKeyOf(factor.semantic()) != edge.Factor() || !slotOK || slot < 0 || int(slot) >= runtime.Count() {
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
		origin, originOK := runtimeFactorEdgeOrigin(sourcePoint, targetPoint, edge.Factor(), edge.Input().Provenance(), edge.Input().Reindex().Key(), bound.post)
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
	factorByKey := make(map[composition.Key]runtimeFactor, len(factors))
	for key, factor := range factors {
		if !key.Available() || factor == nil || compositionKeyOf(factor.semantic()) != key {
			return nil, false
		}
		factorByKey[key] = factor
	}
	for index := 0; index < graph.GroupCount(); index++ {
		group, groupOK := graph.HyperedgeAt(index)
		groupIndex, indexed := graph.GroupIndex(group)
		if !groupOK || !indexed || groupIndex != index || !graph.OwnsGroup(group) || !group.Key().Available() {
			return nil, false
		}
		if !selectedGroups[index] {
			// Consume and authenticate every supplied member identity so callers
			// cannot hide an extra or foreign row in an undemanded fragment. The
			// typed hot member itself is deliberately not copied into the runtime
			// producer.
			for memberIndex := 0; memberIndex < group.MemberCount(); memberIndex++ {
				member, memberOK := group.MemberAt(memberIndex)
				row := byMember[member.Key()]
				if !memberOK || !member.Key().Available() || row == nil || row.member().Key() != member.Key() || !row.member().Rule().Available() {
					return nil, false
				}
				delete(byMember, member.Key())
			}
			producers[index] = runtimeProducer{index: index, group: group}
			continue
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
		members := make([]runtimeMember, group.MemberCount())
		writes, carries := make([]shape.Slot, 0, group.MemberCount()), make([]carrier.ContributionSource, 0)
		initialReads, dynamicReads, demandCarries := make([]demand.Observation, 0), make([]demand.DynamicRead, 0), make([]demand.Carry, 0)
		footprint := make([]recurrenceFootprint, 0, group.MemberCount())
		// These sets are assembly-local deduplication scratch. The published
		// recurrenceFootprint retains authored occurrence targets only; the
		// Factor-owned O(R) route universe remains an owner identity until the
		// active Region binds its one recurrence scope.
		footprintIndexByFactor := make(map[composition.Key]int, group.MemberCount())
		footprintTargets := make(map[composition.Key]map[carrier.Target]struct{}, group.MemberCount())
		footprintNarrowTargets := make(map[composition.Key]map[carrier.Target]struct{}, group.MemberCount())
		appendFootprintTarget := func(index int, target carrier.Target, narrow bool) bool {
			if index < 0 || index >= len(footprint) || !footprint[index].key.Available() {
				return false
			}
			key := footprint[index].key
			seenByFactor := footprintTargets
			if narrow {
				seenByFactor = footprintNarrowTargets
			}
			seen := seenByFactor[key]
			if seen == nil {
				seen = make(map[carrier.Target]struct{})
				seenByFactor[key] = seen
			}
			if _, duplicate := seen[target]; duplicate {
				return true
			}
			seen[target] = struct{}{}
			if narrow {
				footprint[index].narrowTargets = append(footprint[index].narrowTargets, target)
			} else {
				footprint[index].targets = append(footprint[index].targets, target)
			}
			return true
		}
		supportPrune := false
		for memberIndex := range members {
			member, memberOK := group.MemberAt(memberIndex)
			if !memberOK || !member.Key().Available() {
				return nil, false
			}
			row := byMember[member.Key()]
			if row == nil || row.member().Key() != member.Key() || !row.member().Rule().Available() {
				return nil, false
			}
			// A Rule instance belongs to exactly one compiled Group. Consuming the
			// lookup here proves that every supplied runtime member was attached
			// without a later member×group verification sweep.
			delete(byMember, member.Key())
			members[memberIndex] = row
			slot, hasSlot := row.outputSlot()
			supportPrune = supportPrune || !hasSlot
			initialReads = append(initialReads, row.initialReads()...)
			dynamicReads = append(dynamicReads, row.dynamicReads()...)
			if !hasSlot {
				continue
			}
			factor, factorOK := row.factorKey()
			if !factorOK {
				return nil, false
			}
			memberCarries := row.carries()
			occurrenceTargets := row.targets()
			if len(memberCarries) != 0 {
				occurrenceTargets = row.carryTargets()
			}
			narrowTargets := row.narrowTargets()
			for _, target := range narrowTargets {
				if !runtimeContainsTarget(occurrenceTargets, target) {
					return nil, false
				}
			}
			footprintIndex, present := footprintIndexByFactor[factor]
			if !present {
				footprint = append(footprint, recurrenceFootprint{key: factor})
				footprintIndex = len(footprint) - 1
				footprintIndexByFactor[factor] = footprintIndex
			}
			// A route scope is an owner identity, not a target vector. Retaining
			// the Factor's O(R) universe in every route member made the static
			// recurrence footprint grow as Group×R. The active Region expands the
			// identity once below, after all authored occurrence targets have been
			// collected.
			if routeFactor := row.routeScope(); routeFactor != nil {
				if compositionKeyOf(routeFactor.semantic()) != factor {
					return nil, false
				}
				if footprint[footprintIndex].routeFactor != nil && footprint[footprintIndex].routeFactor != routeFactor {
					return nil, false
				}
				footprint[footprintIndex].routeFactor = routeFactor
				footprint[footprintIndex].route = true
				footprint[footprintIndex].narrowRoute = footprint[footprintIndex].narrowRoute || row.routeNarrow()
			}
			for _, target := range occurrenceTargets {
				if !appendFootprintTarget(footprintIndex, target, false) {
					return nil, false
				}
			}
			for _, target := range narrowTargets {
				if !appendFootprintTarget(footprintIndex, target, true) {
					return nil, false
				}
			}
			if row.writesOutput() {
				writes = append(writes, slot)
			}
			for _, input := range memberCarries {
				if input < 0 || input >= len(inputTransports) {
					return nil, false
				}
				carries = append(carries, carrier.ContributionSource{Slot: slot, Input: input})
				demandCarries = append(demandCarries, demand.Carry{Input: uint64(input), Slot: slot})
			}
		}
		plan, planOK := runtime.SealContribution(group.InputCount(), writes, carries, supportPrune, environment != nil)
		if !planOK {
			return nil, false
		}
		sort.Slice(members, func(left, right int) bool {
			return lessRuntimeKey(members[left].member().Key(), members[right].member().Key())
		})
		sort.Slice(footprint, func(left, right int) bool { return lessRuntimeKey(footprint[left].key, footprint[right].key) })
		producers[index] = runtimeProducer{index: index, group: group, plan: plan, members: members, inputs: inputTransports, environment: environment, outputScope: outputScope, premise: premise, reads: initialReads, dynamicReads: dynamicReads, carries: demandCarries, footprint: footprint}
	}
	if len(byMember) != 0 {
		return nil, false
	}
	regions, regionChildren, recurrenceOK := bindRuntimeRegions(graph, activeRegions, runtime, producers)
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
			if !producerOK || !indexed || groupIndex < 0 || groupIndex >= len(selectedGroups) {
				return nil, false
			}
			if !selectedGroups[groupIndex] {
				return nil, false
			}
		}
	}
	for groupIndex, selectedGroup := range selectedGroups {
		if !selectedGroup {
			continue
		}
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
	validQueries := validateRuntimeQueries(receiptState, receiptAuthority, graph, queries)
	if !sealedDemand || !validQueries {
		return nil, false
	}
	dependencyEdges, dependencyAt, dependencyOK := runtimeStaticDependencyEdges(graph)
	if !dependencyOK {
		return nil, false
	}
	assembled := &solverRuntime{receiptState: receiptState, receiptAuthority: receiptAuthority, carrier: runtime, graph: graph, factors: nil, points: points, producers: producers, environments: environments, factorEdges: factorEdges, environmentIncoming: environmentIncoming, factorIncoming: factorIncoming, overlay: runtimeStructuralOverlay{factorByKey: factorByKey, staticOrigins: staticOrigins, originAt: make(map[runtimeFactorOrigin]int), directAt: make(map[int]equation.SelectedStructuralFactorEdge), factorOutgoing: factorOutgoing, dependencyEdges: dependencyEdges, dependencyAt: dependencyAt, reindexes: plans, latePlans: make(map[composition.Key]carrier.ReindexPlan), generation: 1}, demand: demandPlan, queries: append([]runtimeQuery(nil), queries...), observations: append([]runtimeObservation(nil), observations...), pointScopes: pointScopes, pointInitials: pointInitials, regions: regions, regionChildren: regionChildren, pointRegion: pointRegion, activePoints: activePoints, activeRegions: activeRegions}
	return assembled, true
}

func validateRuntimeQueries(receiptState *schemaBindingState, receiptAuthority *schemaBindingAuthority, graph *equation.Graph, rows []runtimeQuery) bool {
	if receiptState == nil || receiptAuthority == nil || receiptState.phase != schemaBindingSealed || receiptState.authority != receiptAuthority || receiptState.schema == nil || !receiptState.schema.Available() || graph == nil || len(rows) != graph.QueryCount() {
		return false
	}
	schema := receiptState.schema
	if schema == nil || graph.CompositionID() != schema.coldID() {
		return false
	}
	ownerRuntime := &solverRuntime{receiptState: receiptState, receiptAuthority: receiptAuthority, graph: graph}
	for index, row := range rows {
		identity, identityOK := graph.QueryAt(index)
		if !identityOK || row == nil || !graph.OwnsQuery(row.query()) || !row.query().Key().Available() || row.query().Key() != identity.Key() || row.query().Family() != identity.Family() {
			return false
		}
		owner := row.queryOwner()
		if owner == nil || !owner.validQueryOwner(ownerRuntime, identity) {
			return false
		}
	}
	return true
}
