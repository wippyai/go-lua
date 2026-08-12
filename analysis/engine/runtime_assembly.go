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
)

// runtimeMember is one typed attachment for a graph-owned RuleMember. It has
// no independent schedule, candidate, or publication path.
type runtimeMember interface {
	member() equation.RuleMember
	outputSlot() (shape.Slot, bool)
	factorKey() (composition.Key, bool)
	carries() []int
	initialReads() []demand.Observation
	dynamicReads() []demand.DynamicRead
	targets() []carrier.Target
	carryTargets() []carrier.Target
	narrowTargets() []carrier.Target
	// routeScope is the Factor-owned authority for a route write/carry. The
	// member retains only this identity and the narrow capability bit; the
	// route target universe is expanded by bindRuntimeRegions once per
	// (active Region, Factor), never once per member or Group.
	routeScope() runtimeFactor
	routeNarrow() bool
	writesOutput() bool
	execute(*carrier.Work, carrier.RuleContributionBase, []carrier.State, support.Mask) memberResult
}

type memberResult struct {
	patch       carrier.Patch
	wrote       bool
	retained    support.Mask
	hasSupport  bool
	activations []equation.AcceptedMember
	reads       []demand.Observation
	phase       SolveFailurePhase
	valid       bool
}

type boundRuleMember[V, O any] struct {
	value          equation.RuleMember
	rule           *boundRule[V, O]
	slot           shape.Slot
	hasSlot        bool
	carry          []int
	outputTargets  []carrier.Target
	allTargets     []carrier.Target
	narrowEligible bool
	routeOwner     runtimeFactor
	outputWrite    bool
}

func (bound *boundRuleMember[V, O]) member() equation.RuleMember { return bound.value }
func (bound *boundRuleMember[V, O]) outputSlot() (shape.Slot, bool) {
	return bound.slot, bound != nil && bound.rule != nil && bound.hasSlot
}
func (bound *boundRuleMember[V, O]) factorKey() (composition.Key, bool) {
	if bound == nil || bound.rule == nil || bound.rule.rule == nil || bound.rule.rule.output == nil {
		return composition.Key{}, false
	}
	return bound.rule.rule.output.semantic.compositionKey(), true
}
func (bound *boundRuleMember[V, O]) carries() []int { return append([]int(nil), bound.carry...) }
func (bound *boundRuleMember[V, O]) initialReads() []demand.Observation {
	if bound == nil || bound.rule == nil {
		return nil
	}
	return bound.rule.initialReads()
}
func (bound *boundRuleMember[V, O]) dynamicReads() []demand.DynamicRead {
	if bound == nil || bound.rule == nil {
		return nil
	}
	return bound.rule.dynamicReads()
}
func (bound *boundRuleMember[V, O]) targets() []carrier.Target {
	if bound == nil {
		return nil
	}
	return append([]carrier.Target(nil), bound.outputTargets...)
}
func (bound *boundRuleMember[V, O]) carryTargets() []carrier.Target {
	if bound == nil {
		return nil
	}
	return append([]carrier.Target(nil), bound.allTargets...)
}
func (bound *boundRuleMember[V, O]) narrowTargets() []carrier.Target {
	if bound == nil || !bound.narrowEligible {
		return nil
	}
	if len(bound.carry) != 0 {
		return append([]carrier.Target(nil), bound.allTargets...)
	}
	return append([]carrier.Target(nil), bound.outputTargets...)
}
func (bound *boundRuleMember[V, O]) routeScope() runtimeFactor {
	if bound == nil {
		return nil
	}
	return bound.routeOwner
}
func (bound *boundRuleMember[V, O]) routeNarrow() bool {
	return bound != nil && bound.routeOwner != nil && bound.narrowEligible
}
func (bound *boundRuleMember[V, O]) writesOutput() bool { return bound != nil && bound.outputWrite }
func (bound *boundRuleMember[V, O]) execute(work *carrier.Work, base carrier.RuleContributionBase, inputs []carrier.State, within support.Mask) memberResult {
	if bound == nil || bound.rule == nil {
		return memberResult{phase: SolveFailurePhasePreflight}
	}
	patch, reads, wrote, ok, phase := bound.rule.execute(work, base, inputs, within)
	return memberResult{patch: patch, wrote: wrote, reads: reads, phase: phase, valid: ok}
}

// bindRuleMember attaches one typed Factor Rule to its private Group member.
// Group input arity is checked by assembly, where the Group is available.
func bindRuleMember[V, O any](member equation.RuleMember, rule *Rule[V, O], operand O, output runtimeFactor) (*boundRuleMember[V, O], bool) {
	if !member.Key().Available() || rule == nil || rule.schema == nil || member.Rule() != rule.schema.semantic.compositionKey() {
		return nil, false
	}
	bound, targets, ok := bindMemberRule(member, rule, operand, output)
	if !ok {
		return nil, false
	}
	carries := make([]int, len(rule.schema.carries))
	for index, carry := range rule.schema.carries {
		if carry.factor != rule.schema.output || carry.input < 0 || carry.input >= rule.schema.inputs {
			return nil, false
		}
		carries[index] = carry.input
	}
	slot, slotOK := output.runtimeSlot()
	if !slotOK {
		return nil, false
	}
	for _, target := range targets {
		owner, valid := target.Slot()
		if !valid || owner != slot {
			return nil, false
		}
	}
	if len(targets) == 0 && len(carries) == 0 && bound.routeScope == nil {
		return nil, false
	}
	var carryTargets []carrier.Target
	if len(carries) != 0 {
		var targetsOK bool
		carryTargets, targetsOK = output.carryTargetsFor(member)
		if !targetsOK {
			return nil, false
		}
		for _, target := range targets {
			carryTargets = appendUniqueTarget(carryTargets, target)
		}
		carryTargets = compactRuntimeTargets(carryTargets)
	}
	return &boundRuleMember[V, O]{value: member, rule: bound, slot: slot, hasSlot: true, carry: carries, outputTargets: append([]carrier.Target(nil), targets...), allTargets: carryTargets, narrowEligible: output.supports(carrier.Narrow), routeOwner: bound.routeScope, outputWrite: len(rule.schema.writes) != 0 || bound.carrySemantic.Available()}, true
}

type boundSupportMember struct {
	value equation.RuleMember
	rule  *compiledSupportRule
}

func (bound *boundSupportMember) member() equation.RuleMember        { return bound.value }
func (bound *boundSupportMember) outputSlot() (shape.Slot, bool)     { return 0, false }
func (bound *boundSupportMember) factorKey() (composition.Key, bool) { return composition.Key{}, false }
func (bound *boundSupportMember) carries() []int                     { return nil }
func (bound *boundSupportMember) initialReads() []demand.Observation {
	if bound == nil || bound.rule == nil {
		return nil
	}
	return bound.rule.initialReads()
}
func (bound *boundSupportMember) dynamicReads() []demand.DynamicRead {
	if bound == nil || bound.rule == nil {
		return nil
	}
	return bound.rule.dynamicReads()
}
func (bound *boundSupportMember) targets() []carrier.Target       { return nil }
func (bound *boundSupportMember) carryTargets() []carrier.Target  { return nil }
func (bound *boundSupportMember) narrowTargets() []carrier.Target { return nil }
func (bound *boundSupportMember) routeScope() runtimeFactor       { return nil }
func (bound *boundSupportMember) routeNarrow() bool               { return false }
func (bound *boundSupportMember) writesOutput() bool              { return false }
func (bound *boundSupportMember) execute(work *carrier.Work, base carrier.RuleContributionBase, inputs []carrier.State, within support.Mask) memberResult {
	if bound == nil || bound.rule == nil {
		return memberResult{}
	}
	retained, reads, ok := bound.rule.execute(work, base, inputs, within)
	return memberResult{retained: retained, hasSupport: true, reads: reads, valid: ok}
}

func bindSupportMember(member equation.RuleMember, rule *SupportRule) (*boundSupportMember, bool) {
	if !member.Key().Available() || rule == nil || rule.schema == nil || member.Rule() != rule.schema.semantic.compositionKey() {
		return nil, false
	}
	compiled, ok := compileSupportRule(rule)
	if !ok {
		return nil, false
	}
	anchor := semanticKeyFromComposition(member.Key())
	if !anchor.Available() {
		return nil, false
	}
	compiled.anchor = anchor
	return &boundSupportMember{value: member, rule: compiled}, true
}

// boundActivationMember is an output-free member in the same Group
// transaction as Factors and support pruning.  Its selection is returned to
// the executor; it has no second evaluation or publication route.
type boundActivationMember struct {
	value equation.RuleMember
	rule  *compiledActivationRule
}

func (bound *boundActivationMember) member() equation.RuleMember    { return bound.value }
func (bound *boundActivationMember) outputSlot() (shape.Slot, bool) { return 0, false }
func (bound *boundActivationMember) factorKey() (composition.Key, bool) {
	return composition.Key{}, false
}
func (bound *boundActivationMember) carries() []int { return nil }
func (bound *boundActivationMember) initialReads() []demand.Observation {
	if bound == nil || bound.rule == nil {
		return nil
	}
	return bound.rule.initialReads()
}
func (bound *boundActivationMember) dynamicReads() []demand.DynamicRead {
	if bound == nil || bound.rule == nil {
		return nil
	}
	return bound.rule.dynamicReads()
}
func (bound *boundActivationMember) targets() []carrier.Target       { return nil }
func (bound *boundActivationMember) carryTargets() []carrier.Target  { return nil }
func (bound *boundActivationMember) narrowTargets() []carrier.Target { return nil }
func (bound *boundActivationMember) routeScope() runtimeFactor       { return nil }
func (bound *boundActivationMember) routeNarrow() bool               { return false }
func (bound *boundActivationMember) writesOutput() bool              { return false }
func (bound *boundActivationMember) execute(work *carrier.Work, base carrier.RuleContributionBase, inputs []carrier.State, within support.Mask) memberResult {
	if bound == nil || bound.rule == nil {
		return memberResult{}
	}
	selected, reads, ok := bound.rule.execute(work, base, inputs, within)
	return memberResult{activations: selected, reads: reads, valid: ok}
}

func bindActivationMember(member equation.RuleMember, rule *ActivationRule, topology *equation.Topology, trigger composition.Key, graph *equation.Graph) (*boundActivationMember, bool) {
	if !member.Key().Available() || rule == nil || rule.schema == nil || member.Rule() != rule.schema.semantic.compositionKey() || topology == nil || graph == nil || !trigger.Available() || trigger != member.Key() {
		return nil, false
	}
	compiled, ok := compileActivationRule(rule, topology, trigger, graph)
	if !ok {
		return nil, false
	}
	anchor := semanticKeyFromComposition(member.Key())
	if !anchor.Available() {
		return nil, false
	}
	compiled.anchor = anchor
	return &boundActivationMember{value: member, rule: compiled}, true
}

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
	composition *Composition
	topology    *equation.Topology
	carrier     *carrier.Composition
	graph       *equation.Graph
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
	pointScopes         []carrier.Scope
	pointInitials       []support.Mask
	regions             []runtimeRegion
	regionChildren      [][]int // operational traversal cache derived from immutable Region.Parent
	pointRegion         []int
	activePoints        []bool
	activeRegions       []bool

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
	factorByKey     map[composition.Key]runtimeFactor
	staticOrigins   map[runtimeFactorOrigin]struct{}
	originAt        map[runtimeFactorOrigin]int
	factorOutgoing  [][]int // all static and selected factor transports
	dependencyEdges []schedule.Edge
	dependencyAt    map[[2]int]struct{}
	reindexes       runtimeReindexes
	latePlans       map[composition.Key]carrier.ReindexPlan
	generation      uint64
}

func runtimeFactorEdgeOrigin(source, target int, factor, provenance, reindex composition.Key, post support.Mask) (runtimeFactorOrigin, bool) {
	postIdentity, postOK := post.Identity()
	origin := runtimeFactorOrigin{source: source, target: target, factor: factor, provenance: provenance, reindex: reindex, post: postIdentity}
	return origin, source >= 0 && target >= 0 && factor.Available() && provenance.Available() && reindex.Available() && postOK && origin.available()
}

func (origin runtimeFactorOrigin) available() bool {
	return origin.source >= 0 && origin.target >= 0 && origin.factor.Available() && origin.provenance.Available() && origin.reindex.Available() && origin.post.Available()
}

// assembleSolver attaches graph-issued members to one sealed carrier, then
// seals the demand Point closure. Points are the only runtime state roots.
func assembleRuntime(cold *Composition, graph *equation.Graph, runtime *carrier.Composition, factors map[composition.Key]runtimeFactor, rows []runtimeMember, queries []runtimeQuery) (*solverRuntime, bool) {
	if cold == nil || !cold.Sealed() || graph == nil || runtime == nil || runtime.Guards() == nil || factors == nil {
		return nil, false
	}
	points, ok := graph.Demand()
	if !ok || points == nil || points.PointCount() == 0 || points.EventCount() == 0 {
		return nil, false
	}
	activePoints, activeRegions, activeOK := runtimeDemandMembership(graph, points)
	if !activeOK {
		return nil, false
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
		if !edgeOK || !edge.Key().Available() || !sourceIndexed || !targetIndexed || !factorKnown || factor == nil || factor.semantic().compositionKey() != edge.Factor() || !slotOK || slot < 0 || int(slot) >= runtime.Count() {
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
		if edge.target < 0 || edge.target >= len(environmentIncoming) {
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
		if !key.Available() || factor == nil || factor.semantic().compositionKey() != key {
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
			occurrenceTargets := row.targets()
			if len(row.carries()) != 0 {
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
				if routeFactor.semantic().compositionKey() != factor {
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
			for _, input := range row.carries() {
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
		producers[index] = runtimeProducer{index: index, group: group, plan: plan, members: members, inputs: append([]runtimeInput(nil), inputTransports...), environment: environment, outputScope: outputScope, premise: premise, reads: initialReads, dynamicReads: dynamicReads, carries: demandCarries, footprint: footprint}
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
	selectedGroups := make([]bool, graph.GroupCount())
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
			selectedGroups[groupIndex] = true
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
	validQueries := validateRuntimeQueries(cold, graph, queries)
	if !sealedDemand || !validQueries {
		return nil, false
	}
	dependencyEdges, dependencyAt, dependencyOK := runtimeStaticDependencyEdges(graph)
	if !dependencyOK {
		return nil, false
	}
	assembled := &solverRuntime{composition: cold, carrier: runtime, graph: graph, factors: nil, points: points, producers: producers, environments: environments, factorEdges: factorEdges, environmentIncoming: environmentIncoming, factorIncoming: factorIncoming, overlay: runtimeStructuralOverlay{factorByKey: factorByKey, staticOrigins: staticOrigins, originAt: make(map[runtimeFactorOrigin]int), factorOutgoing: factorOutgoing, dependencyEdges: dependencyEdges, dependencyAt: dependencyAt, reindexes: plans, latePlans: make(map[composition.Key]carrier.ReindexPlan), generation: 1}, demand: demandPlan, queries: append([]runtimeQuery(nil), queries...), pointScopes: pointScopes, pointInitials: pointInitials, regions: regions, regionChildren: regionChildren, pointRegion: pointRegion, activePoints: activePoints, activeRegions: activeRegions}
	return assembled, true
}

func validateRuntimeQueries(cold *Composition, graph *equation.Graph, rows []runtimeQuery) bool {
	if cold == nil || !cold.Sealed() || cold.coldComposition() == nil || graph == nil || graph.CompositionID() != cold.coldComposition().ID() || len(rows) != graph.QueryCount() {
		return false
	}
	for index, row := range rows {
		identity, identityOK := graph.QueryAt(index)
		if !identityOK || row == nil || !graph.OwnsQuery(row.query()) || !row.query().Key().Available() || row.query().Key() != identity.Key() || row.query().Family() != identity.Family() {
			return false
		}
		authority := row.queryAuthority()
		if !validQueryAuthority(authority) || authority.schema.composition != cold || authority.schema.semantic.compositionKey() != row.query().Family() || authority.schema.bindIndex != authority.index {
			return false
		}
		familyIndex, known := cold.coldComposition().QueryIndex(row.query().Family())
		if !known || familyIndex != authority.index {
			return false
		}
	}
	return true
}

// runtimeStaticDependencyEdges copies the already sealed Point influence
// relation once into dense form.  Selected structural edges are appended to
// this relation only while preparing an overlay; schedule.Prepare then gives
// one batched cycle proof for static plus previously selected plus new edges.
// Group input and designated environment input dependencies both target the
// Group output, while direct Environment/Factor rows target their own Point.
func runtimeStaticDependencyEdges(graph *equation.Graph) ([]schedule.Edge, map[[2]int]struct{}, bool) {
	if graph == nil || graph.Schedule() == nil || graph.PointCount() == 0 {
		return nil, nil, false
	}
	seen := make(map[[2]int]struct{})
	edges := make([]schedule.Edge, 0, graph.EnvironmentEdgeTotal()+graph.FactorEdgeTotal()+graph.GroupCount())
	appendEdge := func(source, target int) bool {
		if source < 0 || target < 0 || source >= graph.PointCount() || target >= graph.PointCount() {
			return false
		}
		key := [2]int{source, target}
		if _, duplicate := seen[key]; !duplicate {
			seen[key] = struct{}{}
			edges = append(edges, schedule.Edge{From: schedule.Node(source), To: schedule.Node(target)})
		}
		return true
	}
	for sourceIndex := 0; sourceIndex < graph.PointCount(); sourceIndex++ {
		source, sourceOK := graph.PointAt(schedule.Node(sourceIndex))
		if !sourceOK || !graph.OwnsPoint(source) {
			return nil, nil, false
		}
		for index := 0; index < graph.ConsumerCount(source); index++ {
			group, groupOK := graph.ConsumerAt(source, index)
			target, targetOK := group.Output(), graph.OwnsGroup(group)
			targetIndex, indexed := graph.PointIndex(target)
			if !groupOK || !targetOK || !indexed || !appendEdge(sourceIndex, targetIndex) {
				return nil, nil, false
			}
		}
		for index := 0; index < graph.EnvironmentGroupCount(source); index++ {
			group, groupOK := graph.EnvironmentGroupAt(source, index)
			target, targetOK := group.Output(), graph.OwnsGroup(group)
			targetIndex, indexed := graph.PointIndex(target)
			if !groupOK || !targetOK || !indexed || !appendEdge(sourceIndex, targetIndex) {
				return nil, nil, false
			}
		}
		for index := 0; index < graph.EnvironmentOutgoingCount(source); index++ {
			edge, edgeOK := graph.EnvironmentOutgoingAt(source, index)
			targetIndex, indexed := graph.PointIndex(edge.Target())
			if !edgeOK || !indexed || !appendEdge(sourceIndex, targetIndex) {
				return nil, nil, false
			}
		}
		for index := 0; index < graph.FactorOutgoingCount(source); index++ {
			edge, edgeOK := graph.FactorOutgoingAt(source, index)
			targetIndex, indexed := graph.PointIndex(edge.Target())
			if !edgeOK || !indexed || !appendEdge(sourceIndex, targetIndex) {
				return nil, nil, false
			}
		}
	}
	sort.Slice(edges, func(left, right int) bool {
		if edges[left].From != edges[right].From {
			return edges[left].From < edges[right].From
		}
		return edges[left].To < edges[right].To
	})
	return edges, seen, true
}

// bindRuntimeRegions is the recurrence admission cut for the one already
// sealed Demand.  A disconnected Region has no runtime meaning for this
// Solve: it receives neither scopes nor an episode, so an unranked unrelated
// cycle cannot poison a demanded query.  Every active Region still binds its
// complete static M_K footprint before Work opens.
type runtimeRegionEdgeResolver struct {
	environment func(equation.EnvironmentEdgeNode) (int, bool)
	factor      func(equation.FactorEdgeNode) (int, bool)
	runtime     bool
}

func bindRuntimeRegions(graph *equation.Graph, active []bool, runtime *carrier.Composition, producers []runtimeProducer) ([]runtimeRegion, [][]int, bool) {
	if graph == nil {
		return nil, nil, false
	}
	return bindRuntimeRegionsWithEdges(graph, active, runtime, producers, runtimeRegionEdgeResolver{
		environment: graph.EnvironmentEdgeIndex,
		factor:      graph.FactorEdgeIndex,
	})
}

func bindRuntimeRegionsWithEdges(graph *equation.Graph, active []bool, runtime *carrier.Composition, producers []runtimeProducer, edges runtimeRegionEdgeResolver) ([]runtimeRegion, [][]int, bool) {
	if graph == nil || runtime == nil || len(active) != graph.RegionCount() || len(producers) != graph.GroupCount() {
		return nil, nil, false
	}
	if edges.environment == nil || edges.factor == nil {
		return nil, nil, false
	}
	regions := make([]runtimeRegion, graph.RegionCount())
	for regionIndex := range regions {
		if !active[regionIndex] {
			continue
		}
		region, ok := graph.RegionAt(regionIndex)
		if !ok {
			return nil, nil, false
		}
		head, headOK := region.Head()
		headIndex, indexed := graph.PointIndex(head)
		if !headOK || !indexed {
			return nil, nil, false
		}
		bound := runtimeRegion{active: true, head: headIndex}
		parent, parentOK := region.Parent()
		if !parentOK || parent < schedule.NoRegion || parent >= graph.RegionCount() {
			return nil, nil, false
		}
		bound.parent = parent
		bound.points = make([]int, region.PointCount())
		for index := range bound.points {
			point, pointOK := region.PointAt(index)
			pointIndex, indexed := graph.PointIndex(point)
			if !pointOK || !indexed {
				return nil, nil, false
			}
			bound.points[index] = pointIndex
		}
		faces := make(map[int]struct{}, region.FaceCount())
		for index := 0; index < region.FaceCount(); index++ {
			face, faceOK := region.FaceAt(index)
			pointIndex, indexed := graph.PointIndex(face)
			if !faceOK || !indexed {
				return nil, nil, false
			}
			if _, duplicate := faces[pointIndex]; !duplicate {
				faces[pointIndex] = struct{}{}
				bound.faces = append(bound.faces, pointIndex)
			}
		}
		for index := 0; index < region.ExternalHeadProducerCount(); index++ {
			group, ok := region.ExternalHeadProducerAt(index)
			groupIndex, indexed := graph.GroupIndex(group)
			if !ok || !indexed || group.Output() != head {
				return nil, nil, false
			}
			bound.external = append(bound.external, groupIndex)
		}
		for index := 0; index < region.BackHeadProducerCount(); index++ {
			group, ok := region.BackHeadProducerAt(index)
			groupIndex, indexed := graph.GroupIndex(group)
			if !ok || !indexed || group.Output() != head {
				return nil, nil, false
			}
			bound.back = append(bound.back, groupIndex)
		}
		for index := 0; index < region.ExternalEnvironmentEdgeCount(); index++ {
			edge, edgeOK := region.ExternalEnvironmentEdgeAt(index)
			if !edgeOK {
				return nil, nil, false
			}
			edgeIndex, edgeIndexed := edges.environment(edge)
			if !edgeIndexed {
				return nil, nil, false
			}
			bound.environmentExternal = append(bound.environmentExternal, edgeIndex)
		}
		for index := 0; index < region.BackEnvironmentEdgeCount(); index++ {
			edge, edgeOK := region.BackEnvironmentEdgeAt(index)
			if !edgeOK {
				return nil, nil, false
			}
			edgeIndex, edgeIndexed := edges.environment(edge)
			if !edgeIndexed {
				return nil, nil, false
			}
			if edge.Input().IdentityTransport() == false {
				return nil, nil, false
			}
			bound.environmentBack = append(bound.environmentBack, edgeIndex)
		}
		for index := 0; index < region.ExternalFactorEdgeCount(); index++ {
			edge, edgeOK := region.ExternalFactorEdgeAt(index)
			if !edgeOK {
				return nil, nil, false
			}
			edgeIndex, edgeIndexed := edges.factor(edge)
			if !edgeIndexed {
				return nil, nil, false
			}
			bound.factorExternal = append(bound.factorExternal, edgeIndex)
		}
		for index := 0; index < region.BackFactorEdgeCount(); index++ {
			edge, edgeOK := region.BackFactorEdgeAt(index)
			if !edgeOK {
				return nil, nil, false
			}
			edgeIndex, edgeIndexed := edges.factor(edge)
			if !edgeIndexed {
				return nil, nil, false
			}
			bound.factorBack = append(bound.factorBack, edgeIndex)
		}
		if len(bound.external)+len(bound.back) != graph.ProducerCount(head) {
			return nil, nil, false
		}
		// Factor membership is static CSR topology; consume every internal Group
		// once and attach only its occurrence-local authored targets. Route scopes
		// retain the owning Factor identity and flags here. Their O(R) universe is
		// expanded once below per active Region/Factor pair, never per Group.
		factorSeen := make(map[composition.Key]bool, region.FactorCount())
		regionRoutes := make(map[composition.Key]struct {
			factor      runtimeFactor
			route       bool
			narrowRoute bool
		}, region.FactorCount())
		for factorIndex := 0; factorIndex < region.FactorCount(); factorIndex++ {
			factor, ok := region.FactorAt(factorIndex)
			if !ok || !factor.Available() {
				return nil, nil, false
			}
			if _, duplicate := factorSeen[factor]; duplicate {
				return nil, nil, false
			}
			factorSeen[factor] = false
		}
		// A structural FactorEdge can be the only internal recurrence
		// contributor. It has no Group occurrence footprint, and it need not
		// target the WTO head, so account for the complete internal CSR row when
		// proving the Region's M_K set.
		for internalIndex := 0; internalIndex < region.InternalFactorEdgeCount(); internalIndex++ {
			edge, edgeOK := region.InternalFactorEdgeAt(internalIndex)
			if !edgeOK {
				return nil, nil, false
			}
			if _, known := factorSeen[edge.Factor()]; !known {
				return nil, nil, false
			}
			factorSeen[edge.Factor()] = true
		}
		targets, narrowTargets := make([]carrier.Target, 0), make([]carrier.Target, 0)
		for internalIndex := 0; internalIndex < region.InternalGroupCount(); internalIndex++ {
			group, groupOK := region.InternalHyperedgeAt(internalIndex)
			groupIndex, indexed := graph.GroupIndex(group)
			if !groupOK || !indexed || groupIndex < 0 || groupIndex >= len(producers) || producers[groupIndex].group.Key() != group.Key() {
				return nil, nil, false
			}
			if environmentInput, hasEnvironment := group.EnvironmentInput(); hasEnvironment {
				sourcePoint, sourceIndexed := graph.PointIndex(environmentInput.Point())
				if !sourceIndexed {
					return nil, nil, false
				}
				inside := false
				for _, point := range bound.points {
					if point == sourcePoint {
						inside = true
						break
					}
				}
				if inside && !environmentInput.IdentityTransport() {
					return nil, nil, false
				}
			}
			for _, occurrence := range producers[groupIndex].footprint {
				route := regionRoutes[occurrence.key]
				if _, known := factorSeen[occurrence.key]; !known {
					return nil, nil, false
				}
				factorSeen[occurrence.key] = true
				targets = append(targets, occurrence.targets...)
				narrowTargets = append(narrowTargets, occurrence.narrowTargets...)
				if occurrence.route {
					if occurrence.routeFactor == nil || occurrence.routeFactor.semantic().compositionKey() != occurrence.key {
						return nil, nil, false
					}
					if route.factor != nil && route.factor != occurrence.routeFactor {
						return nil, nil, false
					}
					route.factor = occurrence.routeFactor
					route.route = true
					route.narrowRoute = route.narrowRoute || occurrence.narrowRoute
					regionRoutes[occurrence.key] = route
				}
			}
		}
		for _, found := range factorSeen {
			if !found {
				return nil, nil, false
			}
		}
		// Expand each exact route universe at most once for this active Region.
		// runtimeFactor is the sealed owner capability, so the resulting targets
		// are still checked by the carrier's own composition/factor fence when
		// the recurrence scopes are sealed.
		for factorIndex := 0; factorIndex < region.FactorCount(); factorIndex++ {
			factor, ok := region.FactorAt(factorIndex)
			if !ok || !factor.Available() {
				return nil, nil, false
			}
			route := regionRoutes[factor]
			if !route.route {
				continue
			}
			if route.factor == nil || route.factor.semantic().compositionKey() != factor {
				return nil, nil, false
			}
			if !route.factor.hasRouteUniverse() {
				return nil, nil, false
			}
			universe := route.factor.routeUniverse()
			targets = append(targets, universe...)
			if route.narrowRoute {
				narrowTargets = append(narrowTargets, universe...)
			}
		}
		targets = compactRuntimeTargets(targets)
		narrowTargets = compactRuntimeTargets(narrowTargets)
		var widen, narrow carrier.MergeScope
		var widenOK, narrowOK bool
		if edges.runtime {
			widen, widenOK = runtime.SealRuntimeWidening(targets)
			narrow, narrowOK = runtime.SealRuntimeNarrowing(narrowTargets)
		} else {
			widen, widenOK = runtime.SealWidening(targets)
			narrow, narrowOK = runtime.SealNarrowing(narrowTargets)
		}
		if !widenOK || !narrowOK {
			return nil, nil, false
		}
		bound.widen, bound.narrow = widen, narrow
		regions[regionIndex] = bound
	}
	// Parent is immutable graph topology.  The children row is only a compact
	// executor traversal aid for subtree cache invalidation; it never defines
	// recurrence membership or semantic evaluation.
	children := make([][]int, len(regions))
	for child, bound := range regions {
		if !active[child] {
			continue
		}
		parent := bound.parent
		if parent == schedule.NoRegion {
			continue
		}
		if parent < 0 || parent >= len(regions) || !active[parent] || !regions[parent].active {
			return nil, nil, false
		}
		children[parent] = append(children[parent], child)
	}
	return regions, children, true
}

// runtimeDemandMembership projects the existing equation.Demand into the two
// dense runtime masks.  It is a cold integrity check over the sole point WTO
// event stream, not a second dependency graph or activation mechanism.
func runtimeDemandMembership(graph *equation.Graph, points *equation.Demand) ([]bool, []bool, bool) {
	if graph == nil || points == nil || points.PointCount() == 0 || points.EventCount() == 0 {
		return nil, nil, false
	}
	activePoints := make([]bool, graph.PointCount())
	activeRegions := make([]bool, graph.RegionCount())
	entered := make([]bool, graph.RegionCount())
	exited := make([]bool, graph.RegionCount())
	stack := make([]int, 0, graph.RegionCount())
	for index := 0; index < points.EventCount(); index++ {
		event, _, ok := points.EventAt(index)
		if !ok {
			return nil, nil, false
		}
		switch event.Kind {
		case schedule.EventEnter:
			if event.Region < 0 || event.Region >= len(activeRegions) || entered[event.Region] {
				return nil, nil, false
			}
			region, regionOK := graph.RegionAt(event.Region)
			head, headOK := region.Head()
			headIndex, headIndexed := graph.PointIndex(head)
			parent, parentOK := region.Parent()
			if !regionOK || !headOK || !headIndexed || !parentOK || event.Node < 0 || headIndex != int(event.Node) {
				return nil, nil, false
			}
			if len(stack) == 0 {
				if parent != schedule.NoRegion {
					return nil, nil, false
				}
			} else if parent != stack[len(stack)-1] {
				return nil, nil, false
			}
			activeRegions[event.Region], entered[event.Region] = true, true
			stack = append(stack, event.Region)
		case schedule.EventExit:
			if event.Region < 0 || event.Region >= len(activeRegions) || !entered[event.Region] || exited[event.Region] || len(stack) == 0 || stack[len(stack)-1] != event.Region {
				return nil, nil, false
			}
			region, regionOK := graph.RegionAt(event.Region)
			head, headOK := region.Head()
			headIndex, headIndexed := graph.PointIndex(head)
			if !regionOK || !headOK || !headIndexed || event.Node < 0 || headIndex != int(event.Node) {
				return nil, nil, false
			}
			exited[event.Region] = true
			stack = stack[:len(stack)-1]
		case schedule.EventNode:
			if event.Node < 0 || int(event.Node) >= len(activePoints) {
				return nil, nil, false
			}
			if len(stack) == 0 {
				if event.Region != schedule.NoRegion {
					return nil, nil, false
				}
			} else if event.Region != stack[len(stack)-1] {
				return nil, nil, false
			}
			activePoints[event.Node] = true
		default:
			return nil, nil, false
		}
	}
	if len(stack) != 0 {
		return nil, nil, false
	}
	for index, active := range activeRegions {
		if active != (entered[index] && exited[index]) {
			return nil, nil, false
		}
	}
	for demandIndex := 0; demandIndex < points.PointCount(); demandIndex++ {
		point, pointOK := points.PointAt(demandIndex)
		pointIndex, indexed := graph.PointIndex(point)
		region, regionOK := graph.PointRegion(point)
		if !pointOK || !indexed || !regionOK || pointIndex < 0 || pointIndex >= len(activePoints) || !activePoints[pointIndex] {
			return nil, nil, false
		}
		for region != schedule.NoRegion {
			if region < 0 || region >= len(activeRegions) || !activeRegions[region] {
				return nil, nil, false
			}
			view, viewOK := graph.RegionAt(region)
			parent, parentOK := view.Parent()
			if !viewOK || !parentOK {
				return nil, nil, false
			}
			region = parent
		}
	}
	return activePoints, activeRegions, true
}

func runtimeContainsTarget(targets []carrier.Target, want carrier.Target) bool {
	for _, target := range targets {
		if target.Same(want) {
			return true
		}
	}
	return false
}

func compactRuntimeTargets(targets []carrier.Target) []carrier.Target {
	if len(targets) < 2 {
		return targets
	}
	sort.Slice(targets, func(left, right int) bool { return targets[left].Less(targets[right]) })
	end := 1
	for _, target := range targets[1:] {
		if !targets[end-1].Same(target) {
			targets[end] = target
			end++
		}
	}
	return targets[:end]
}

func lessRuntimeKey(left, right composition.Key) bool {
	for index := range left.ID {
		if left.ID[index] != right.ID[index] {
			return left.ID[index] < right.ID[index]
		}
	}
	return left.Version < right.Version
}
