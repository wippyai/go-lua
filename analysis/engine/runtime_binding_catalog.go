// runtime_binding_catalog.go collects graph use rows, carry closures, and the
// one-shot binding catalog.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// graphFactorUses is the graph-owned, factor-partitioned cold input to typed
// binding. It contains only equation identities and member ordinals: no raw
// key conversion, carrier capability, callback, or caller-authored plan.
// newRuntimeBinding constructs the partition in one Graph walk, so binding F
// Factors never performs F full topology scans.
type graphFactorUses struct {
	reads        []graphReadUse
	writes       []graphWriteUse
	targets      []equation.Surface // exact targets named by carry closures
	queries      []equation.Surface
	carryTargets map[composition.Key]graphCarryClosure // exact predecessor slice for one carrying member
}

type graphReadUse struct {
	row     *schemaRuleReadRow
	index   int
	surface equation.Surface
	// sealedExact marks a structurally sealed exact read. Its complete contract
	// is the graph surface itself, so no legacy schemaRuleReadRow exists or is
	// synthesized. The graph compiler depends on read shape, not Rule lineage.
	sealedExact bool
}

type graphWriteUse struct {
	index     int
	routeRead uint64
	surface   equation.Surface
}

type graphBindingCatalog struct {
	factors       map[composition.Key]*graphFactorUses
	carryClosures map[graphCarryClosureKey]graphCarryClosure
}

// graphCarryClosureKey names one immutable, graph-owned predecessor closure.
// Factor is part of the key: the same equation Point can carry unrelated
// Factor roots with different direct-write surfaces.
type graphCarryClosureKey struct {
	factor composition.Key
	point  composition.Key
}

// graphCarryClosureNode is cold compilation scratch for the equation
// C(F,p) = direct(F,p) union C(F,pred). Its predecessor row follows only
// Carry's declared input port; it is not a general control-flow traversal.
type graphCarryClosureNode struct {
	point        equation.Point
	direct       []equation.Surface
	route        bool
	predecessors []composition.Key
}

// graphCarryClosure is the exact finite static target closure plus the one
// Factor-owned route-universe bit. The bit carries no target vector through
// graph members; runtime expands the Factor universe once per Group scope.
type graphCarryClosure struct {
	targets []equation.Surface
	route   bool
}

type graphCarryFactorClosure struct {
	nodes   []*graphCarryClosureNode
	byPoint map[composition.Key]int
}

func graphUsesFor(catalog *graphBindingCatalog, factor composition.Key) *graphFactorUses {
	if catalog == nil || catalog.factors == nil || !factor.Available() {
		return nil
	}
	uses, ok := catalog.factors[factor]
	if !ok || uses == nil {
		return nil
	}
	return uses
}

func appendGraphRead(catalog *graphBindingCatalog, surface equation.Surface, use graphReadUse) bool {
	uses := graphUsesFor(catalog, surface.Factor)
	if uses == nil {
		return false
	}
	uses.reads = append(uses.reads, use)
	return true
}

func appendGraphWrite(catalog *graphBindingCatalog, surface equation.Surface, use graphWriteUse) bool {
	uses := graphUsesFor(catalog, surface.Factor)
	if uses == nil {
		return false
	}
	uses.writes = append(uses.writes, use)
	return true
}

func appendGraphTarget(catalog *graphBindingCatalog, surface equation.Surface) bool {
	uses := graphUsesFor(catalog, surface.Factor)
	if uses == nil {
		return false
	}
	uses.targets = append(uses.targets, surface)
	return true
}

func appendGraphQuery(catalog *graphBindingCatalog, surface equation.Surface) bool {
	uses := graphUsesFor(catalog, surface.Factor)
	if uses == nil {
		return false
	}
	uses.queries = append(uses.queries, surface)
	return true
}

func appendGraphCarryTargets(catalog *graphBindingCatalog, factor, member composition.Key, closure graphCarryClosure) bool {
	uses := graphUsesFor(catalog, factor)
	if uses == nil || !member.Available() {
		return false
	}
	if uses.carryTargets == nil {
		uses.carryTargets = make(map[composition.Key]graphCarryClosure)
	}
	if _, duplicate := uses.carryTargets[member]; duplicate {
		return false
	}
	// Carry closures are immutable cold slices. Keep the graph-owned result
	// shared until factor binding materializes carrier Targets; copying here
	// would reintroduce occurrence-times-closure allocation.
	uses.carryTargets[member] = closure
	return true
}

func activationGraphCellReady(state *schemaBindingState, cell *schemaActivationRuleBindingCell) bool {
	if state == nil || cell == nil || state.schema == nil || state.phase != schemaBindingSealed || state.authority == nil || cell.state != state || cell.schema != state.schema || cell.ordinal >= uint64(len(state.rules)) || state.rules[cell.ordinal] != cell || cell.impl == nil || cell.impl.state != state || cell.impl.rule == nil || cell.impl.fold == nil {
		return false
	}
	for index, read := range cell.impl.reads {
		if read == nil {
			return false
		}
		row := read.readRow()
		if row == nil || row != cell.schemaRuleReadAt(uint64(index)) || !row.sealed() || row.owner != cell || row.ownerOrdinal != cell.ordinal || row.readOrdinal != uint64(index) {
			return false
		}
	}
	return true
}

func buildSealedGraphRuleMaps(state *schemaBindingState) (map[composition.Key]sealedRuleGeometry, map[composition.Key]*schemaActivationRuleBindingCell, bool) {
	if state == nil || state.schema == nil || !state.schema.Available() || state.phase != schemaBindingSealed || state.authority == nil {
		return nil, nil, false
	}
	ordinary := make(map[composition.Key]sealedRuleGeometry)
	activations := make(map[composition.Key]*schemaActivationRuleBindingCell)
	for ordinal, raw := range state.rules {
		cell, cellOK := raw.(schemaRuleBindingCell)
		if !cellOK || cell == nil || cell.schemaBindingSchema() != state.schema || cell.schemaRuleOrdinal() != uint64(ordinal) || cell.schemaRuleBindingState() != state {
			return nil, nil, false
		}
		key := state.schema.ruleSemanticAt(uint64(ordinal))
		if !key.Available() {
			return nil, nil, false
		}
		if direct, directOK := raw.(sealedRuleGeometry); directOK {
			if !direct.sealedRuleComplete() || direct.directRuleSemantic() != key || !direct.directRuleOperandFamily().Available() || !direct.directRuleOutputFactor().Available() {
				return nil, nil, false
			}
			if _, duplicate := ordinary[key]; duplicate {
				return nil, nil, false
			}
			ordinary[key] = direct
			continue
		}
		activation, activationOK := raw.(*schemaActivationRuleBindingCell)
		if !activationOK || !activationGraphCellReady(state, activation) {
			return nil, nil, false
		}
		if _, duplicate := activations[key]; duplicate {
			return nil, nil, false
		}
		activations[key] = activation
	}
	return ordinary, activations, true
}

// graphMemberWriteArity states how many writes the graph members of one sealed
// rule carry. It is the rule's declared publication disposition read once: a
// fact publication addresses exactly one write cell, and a structural
// publication - whose output is the activation row set its candidate branches
// mount into the construct topology - addresses none, because there is no
// Factor cell for a write to address. A disposition with no arm here is not a
// graph member at all, so a later disposition cannot enter the catalog by
// defaulting to the writing shape.
func graphMemberWriteArity(rule sealedRuleGeometry) (int, bool) {
	if rule == nil {
		return 0, false
	}
	switch rule.directRuleWriteMode() {
	case directRuleWriteExact, directRuleWriteRoute:
		return 1, true
	case directRuleWriteStructural:
		return 0, true
	default:
		return 0, false
	}
}

// structuralGraphMember reports whether this rule's members publish structurally.
// A structural member reaches the same catalog walks as every other ordinary
// member and contributes the same reads; what it never contributes is a write
// surface or a carry, so the two carry passes and the write pass skip it by
// this predicate rather than by the rule table it was found in.
func structuralGraphMember(rule sealedRuleGeometry) bool {
	writes, arityOK := graphMemberWriteArity(rule)
	return arityOK && writes == 0
}

func validOrdinaryGraphMember(rule sealedRuleGeometry, member equation.RuleMember) bool {
	if rule == nil || !member.Rule().Available() {
		return false
	}
	if _, activation := member.ActivationMember(); activation {
		return false
	}
	writes, arityOK := graphMemberWriteArity(rule)
	if !arityOK || member.Rule() != rule.directRuleSemantic() || member.OperandFamily() != rule.directRuleOperandFamily() || uint64(member.ReadCount()) != rule.directRuleReadCount() || member.WriteCount() != writes {
		return false
	}
	if writes == 0 {
		// A structural publication computes no fact, so it neither addresses a
		// write cell nor carries one into the next point.
		return !rule.directRuleCarryPresent()
	}
	surface, surfaceOK := member.WriteAt(0)
	routeRead, routeOK := member.WriteRouteRead(0)
	if !surfaceOK || !surface.Available() || !routeOK || surface.Factor != rule.directRuleOutputFactor() {
		return false
	}
	switch rule.directRuleWriteMode() {
	case directRuleWriteExact:
		return routeRead == 0 && surface.Form == equation.SurfaceWriteExact && (surface.Mode == equation.TargetModeStrong || surface.Mode == equation.TargetModeWeak) && !surface.Semantic.Available() && !surface.Normalizer.Available()
	case directRuleWriteRoute:
		return routeRead == rule.directRuleRouteRead() && routeRead != 0 && surface.Form == equation.SurfaceWriteRoute && surface.Mode == equation.TargetModeNone && !surface.Semantic.Available() && !surface.Normalizer.Available()
	default:
		return false
	}
}

// buildGraphCarryClosures solves every Carry predecessor equation once during
// compilation. A closure is keyed by (Factor, Point), not Point alone, because
// one equation Point can transport independent factor roots. The per-factor
// predecessor graph is condensed with an iterative SCC walk, then its finite
// DAG is evaluated from predecessor-free components. This is the exact least
// closure of declared direct writes and Carry predecessors: no depth limit,
// cardinality cap, or runtime topology traversal is involved.
func buildGraphCarryClosures(state *schemaBindingState, graph *equation.Graph, rules map[composition.Key]sealedRuleGeometry, activations map[composition.Key]*schemaActivationRuleBindingCell) (map[graphCarryClosureKey]graphCarryClosure, bool) {
	if state == nil || state.schema == nil || !state.schema.Available() || graph == nil || len(rules)+len(activations) == 0 {
		return nil, false
	}
	carriedFactors := make(map[composition.Key]struct{})
	for groupIndex := 0; groupIndex < graph.GroupCount(); groupIndex++ {
		group, ok := graph.HyperedgeAt(groupIndex)
		if !ok || !group.Output().Available() {
			return nil, false
		}
		for memberIndex := 0; memberIndex < group.MemberCount(); memberIndex++ {
			member, memberOK := group.MemberAt(memberIndex)
			if !memberOK || !member.Rule().Available() {
				return nil, false
			}
			if activation, activationOK := activations[member.Rule()]; activationOK {
				witness, witnessOK := member.ActivationMember()
				if activation == nil || !activationGraphCellReady(state, activation) || witnessOK && !witness.Available() || member.WriteCount() != 0 {
					return nil, false
				}
				continue
			}
			rule, ruleOK := rules[member.Rule()]
			if !ruleOK || !validOrdinaryGraphMember(rule, member) {
				return nil, false
			}
			if structuralGraphMember(rule) || !rule.directRuleCarryPresent() {
				continue
			}
			factor := rule.directRuleOutputFactor()
			if !factor.Available() {
				return nil, false
			}
			carriedFactors[factor] = struct{}{}
		}
	}
	if len(carriedFactors) == 0 {
		return make(map[graphCarryClosureKey]graphCarryClosure), true
	}
	plans := make(map[composition.Key]*graphCarryFactorClosure, len(carriedFactors))
	for factor := range carriedFactors {
		plans[factor] = &graphCarryFactorClosure{byPoint: make(map[composition.Key]int)}
	}
	ensureNode := func(factor composition.Key, point equation.Point) (*graphCarryClosureNode, bool) {
		if !factor.Available() || !point.Available() {
			return nil, false
		}
		plan := plans[factor]
		if plan == nil {
			return nil, false
		}
		if index, present := plan.byPoint[point.Key()]; present {
			if index < 0 || index >= len(plan.nodes) || plan.nodes[index] == nil {
				return nil, false
			}
			return plan.nodes[index], true
		}
		node := &graphCarryClosureNode{point: point}
		plan.byPoint[point.Key()] = len(plan.nodes)
		plan.nodes = append(plan.nodes, node)
		return node, true
	}
	for groupIndex := 0; groupIndex < graph.GroupCount(); groupIndex++ {
		group, ok := graph.HyperedgeAt(groupIndex)
		if !ok || !group.Output().Available() {
			return nil, false
		}
		for memberIndex := 0; memberIndex < group.MemberCount(); memberIndex++ {
			member, memberOK := group.MemberAt(memberIndex)
			if !memberOK || !member.Rule().Available() {
				return nil, false
			}
			if activation, activationOK := activations[member.Rule()]; activationOK {
				witness, witnessOK := member.ActivationMember()
				if activation == nil || !activationGraphCellReady(state, activation) || witnessOK && !witness.Available() || member.WriteCount() != 0 {
					return nil, false
				}
				continue
			}
			rule, ruleOK := rules[member.Rule()]
			if !ruleOK || !validOrdinaryGraphMember(rule, member) {
				return nil, false
			}
			if structuralGraphMember(rule) {
				continue
			}
			factor := rule.directRuleOutputFactor()
			if _, carried := carriedFactors[factor]; !carried {
				continue
			}
			node, nodeOK := ensureNode(factor, group.Output())
			if !nodeOK {
				return nil, false
			}
			surface, surfaceOK := member.WriteAt(0)
			routeRead, routeOK := member.WriteRouteRead(0)
			if !surfaceOK || !routeOK {
				return nil, false
			}
			switch rule.directRuleWriteMode() {
			case directRuleWriteExact:
				if routeRead != 0 || surface.Form != equation.SurfaceWriteExact {
					return nil, false
				}
				node.direct = append(node.direct, surface)
			case directRuleWriteRoute:
				if routeRead != rule.directRuleRouteRead() || routeRead == 0 || surface.Form != equation.SurfaceWriteRoute || surface.Mode != equation.TargetModeNone {
					return nil, false
				}
				node.route = true
			default:
				return nil, false
			}
			if !rule.directRuleCarryPresent() {
				continue
			}
			input, inputOK := group.InputAt(int(rule.directRuleCarryInput()))
			if !inputOK {
				return nil, false
			}
			predecessor, predecessorOK := ensureNode(factor, input.Point())
			if !predecessorOK {
				return nil, false
			}
			node.predecessors = append(node.predecessors, predecessor.point.Key())
		}
	}
	closures := make(map[graphCarryClosureKey]graphCarryClosure)
	for factor, plan := range plans {
		if !buildGraphCarryFactorClosures(closures, factor, plan) {
			return nil, false
		}
	}
	return closures, true
}

func buildGraphCarryFactorClosures(closures map[graphCarryClosureKey]graphCarryClosure, factor composition.Key, plan *graphCarryFactorClosure) bool {
	if closures == nil || !factor.Available() || plan == nil || len(plan.nodes) == 0 || len(plan.byPoint) != len(plan.nodes) {
		return false
	}
	nodeCount := len(plan.nodes)
	edges := make([][]int, nodeCount)
	reverse := make([][]int, nodeCount)
	for nodeIndex, node := range plan.nodes {
		if node == nil || !node.point.Available() || plan.byPoint[node.point.Key()] != nodeIndex {
			return false
		}
		for _, predecessor := range node.predecessors {
			predecessorIndex, present := plan.byPoint[predecessor]
			if !present || predecessorIndex < 0 || predecessorIndex >= nodeCount || containsCarryIndex(edges[nodeIndex], predecessorIndex) {
				if !present || predecessorIndex < 0 || predecessorIndex >= nodeCount {
					return false
				}
				continue
			}
			edges[nodeIndex] = append(edges[nodeIndex], predecessorIndex)
			reverse[predecessorIndex] = append(reverse[predecessorIndex], nodeIndex)
		}
	}
	finish := make([]int, 0, nodeCount)
	seen := make([]bool, nodeCount)
	type frame struct{ node, next int }
	for root := 0; root < nodeCount; root++ {
		if seen[root] {
			continue
		}
		seen[root] = true
		stack := []frame{{node: root}}
		for len(stack) != 0 {
			top := &stack[len(stack)-1]
			if top.next < len(edges[top.node]) {
				next := edges[top.node][top.next]
				top.next++
				if !seen[next] {
					seen[next] = true
					stack = append(stack, frame{node: next})
				}
				continue
			}
			finish = append(finish, top.node)
			stack = stack[:len(stack)-1]
		}
	}
	component := make([]int, nodeCount)
	for index := range component {
		component[index] = -1
	}
	componentCount := 0
	for finishIndex := len(finish) - 1; finishIndex >= 0; finishIndex-- {
		root := finish[finishIndex]
		if component[root] >= 0 {
			continue
		}
		component[root] = componentCount
		stack := []int{root}
		for len(stack) != 0 {
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, next := range reverse[node] {
				if component[next] < 0 {
					component[next] = componentCount
					stack = append(stack, next)
				}
			}
		}
		componentCount++
	}
	if componentCount == 0 {
		return false
	}
	values := make([]graphCarryClosure, componentCount)
	dependencies := make([][]int, componentCount)
	dependents := make([][]int, componentCount)
	for nodeIndex, node := range plan.nodes {
		componentIndex := component[nodeIndex]
		if componentIndex < 0 || componentIndex >= componentCount {
			return false
		}
		direct := compactCarrySurfaces(append([]equation.Surface(nil), node.direct...))
		values[componentIndex].targets = mergeCarrySurfaces(values[componentIndex].targets, direct)
		values[componentIndex].route = values[componentIndex].route || node.route
		for _, predecessor := range edges[nodeIndex] {
			predecessorComponent := component[predecessor]
			if predecessorComponent == componentIndex || containsCarryIndex(dependencies[componentIndex], predecessorComponent) {
				continue
			}
			dependencies[componentIndex] = append(dependencies[componentIndex], predecessorComponent)
			dependents[predecessorComponent] = append(dependents[predecessorComponent], componentIndex)
		}
	}
	remaining := make([]int, componentCount)
	queue := make([]int, 0, componentCount)
	for componentIndex := 0; componentIndex < componentCount; componentIndex++ {
		remaining[componentIndex] = len(dependencies[componentIndex])
		if remaining[componentIndex] == 0 {
			queue = append(queue, componentIndex)
		}
	}
	processed := 0
	for head := 0; head < len(queue); head++ {
		resolved := queue[head]
		processed++
		for _, dependent := range dependents[resolved] {
			values[dependent].targets = mergeCarrySurfaces(values[dependent].targets, values[resolved].targets)
			values[dependent].route = values[dependent].route || values[resolved].route
			remaining[dependent]--
			if remaining[dependent] < 0 {
				return false
			}
			if remaining[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}
	if processed != componentCount {
		return false
	}
	for nodeIndex, node := range plan.nodes {
		closureKey := graphCarryClosureKey{factor: factor, point: node.point.Key()}
		if _, duplicate := closures[closureKey]; duplicate {
			return false
		}
		closure := values[component[nodeIndex]]
		closure.targets = append([]equation.Surface(nil), closure.targets...)
		closures[closureKey] = closure
	}
	return true
}

func containsCarryIndex(values []int, want int) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func mergeCarrySurfaces(left, right []equation.Surface) []equation.Surface {
	if len(left) == 0 {
		return right
	}
	if len(right) == 0 {
		return left
	}
	merged := make([]equation.Surface, 0, len(left)+len(right))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) && rightIndex < len(right) {
		if left[leftIndex] == right[rightIndex] {
			merged = append(merged, left[leftIndex])
			leftIndex++
			rightIndex++
			continue
		}
		if lessRuntimeSurface(left[leftIndex], right[rightIndex]) {
			merged = append(merged, left[leftIndex])
			leftIndex++
			continue
		}
		merged = append(merged, right[rightIndex])
		rightIndex++
	}
	merged = append(merged, left[leftIndex:]...)
	merged = append(merged, right[rightIndex:]...)
	return merged
}

func buildGraphBindingCatalog(state *schemaBindingState, graph *equation.Graph) (*graphBindingCatalog, bool) {
	if state == nil {
		return nil, false
	}
	state.mu.Lock()
	schema, sealed := state.schema, state.phase == schemaBindingSealed
	state.mu.Unlock()
	if !sealed || schema == nil || !schema.Available() || graph == nil || !graph.OwnsComposition(schema.cold) || graph.CompositionID() != schema.cold.ID() {
		return nil, false
	}
	ordinary, activations, mapsOK := buildSealedGraphRuleMaps(state)
	if !mapsOK {
		return nil, false
	}
	factorCount, _, queryCount, _, shapeOK := schema.shapeCount()
	if !shapeOK {
		return nil, false
	}
	queries := make(map[composition.Key]struct{}, queryCount)
	for ordinal := 0; ordinal < queryCount; ordinal++ {
		key := schema.querySemanticAt(uint64(ordinal))
		if !key.Available() {
			return nil, false
		}
		if _, duplicate := queries[key]; duplicate {
			return nil, false
		}
		queries[key] = struct{}{}
	}
	catalog := &graphBindingCatalog{factors: make(map[composition.Key]*graphFactorUses, factorCount)}
	for ordinal := 0; ordinal < factorCount; ordinal++ {
		key := schema.factorSemanticAt(uint64(ordinal))
		if !key.Available() {
			return nil, false
		}
		if _, duplicate := catalog.factors[key]; duplicate {
			return nil, false
		}
		catalog.factors[key] = &graphFactorUses{}
	}
	closures, closuresOK := buildGraphCarryClosures(state, graph, ordinary, activations)
	if !closuresOK {
		return nil, false
	}
	catalog.carryClosures = closures
	for groupIndex := 0; groupIndex < graph.GroupCount(); groupIndex++ {
		group, ok := graph.HyperedgeAt(groupIndex)
		if !ok {
			return nil, false
		}
		for memberIndex := 0; memberIndex < group.MemberCount(); memberIndex++ {
			member, ok := group.MemberAt(memberIndex)
			if !ok || !member.Key().Available() || !member.Rule().Available() {
				return nil, false
			}
			activation, activationMember := activations[member.Rule()]
			activationWitness, activationWitnessPresent := member.ActivationMember()
			if activationMember {
				if activation == nil || !activationGraphCellReady(state, activation) || activationWitnessPresent && !activationWitness.Available() || member.WriteCount() != 0 {
					return nil, false
				}
			} else {
				rule, present := ordinary[member.Rule()]
				if !present || !validOrdinaryGraphMember(rule, member) {
					return nil, false
				}
				if !structuralGraphMember(rule) && rule.directRuleCarryPresent() {
					input, inputOK := group.InputAt(int(rule.directRuleCarryInput()))
					if !inputOK {
						return nil, false
					}
					factor := rule.directRuleOutputFactor()
					closureKey := graphCarryClosureKey{factor: factor, point: input.Point().Key()}
					closure, closureOK := catalog.carryClosures[closureKey]
					// A member that publishes routed carries over coordinates its
					// own routes select, which are not in the closure its
					// predecessor point sealed: that closure is what reaches this
					// point, and a route set is decided per invocation. The scope
					// is the member's, so the claim is made on its own
					// registration rather than on the shared predecessor node,
					// where it would over-claim for every other member there.
					if rule.directRuleWriteMode() == directRuleWriteRoute {
						closure.route = true
					}
					if !factor.Available() || !closureOK || !appendGraphCarryTargets(catalog, factor, member.Key(), closure) {
						return nil, false
					}
					for _, target := range closure.targets {
						if !appendGraphTarget(catalog, target) {
							return nil, false
						}
					}
				}
			}
			var owner schemaRuleBindingCell
			if activationMember {
				owner = activations[member.Rule()]
			} else {
				owner = ordinary[member.Rule()]
			}
			for readIndex := 0; readIndex < member.ReadCount(); readIndex++ {
				surface, ok := member.ReadAt(readIndex)
				if !ok || !surface.Available() {
					return nil, false
				}
				row := owner.schemaRuleReadAt(uint64(readIndex))
				sealedExact := row == nil
				if sealedExact {
					if surface.Form != equation.SurfaceReadExact || surface.Mode != equation.TargetModeNone || surface.Semantic.Available() || surface.Normalizer.Available() || surface.Local == 0 {
						return nil, false
					}
				} else if row == nil || !row.sealed() || row.owner != owner || row.ownerOrdinal != owner.schemaRuleOrdinal() || row.readOrdinal != uint64(readIndex) || !row.factor.Available() || row.factor != surface.Factor {
					return nil, false
				}
				use := graphReadUse{row: row, index: readIndex, surface: surface, sealedExact: sealedExact}
				if !appendGraphRead(catalog, surface, use) {
					return nil, false
				}
			}
			// A structural member publishes no fact, so the write table records
			// nothing for it; the reads above are its whole contribution.
			if rule := ordinary[member.Rule()]; !activationMember && !structuralGraphMember(rule) {
				surface, surfaceOK := member.WriteAt(0)
				routeRead, routeOK := member.WriteRouteRead(0)
				if rule == nil || !surfaceOK || !routeOK || !surface.Available() {
					return nil, false
				}
				use := graphWriteUse{index: 0, routeRead: routeRead, surface: surface}
				if !appendGraphWrite(catalog, surface, use) {
					return nil, false
				}
			}
		}
	}
	for queryIndex := 0; queryIndex < graph.QueryCount(); queryIndex++ {
		query, ok := graph.QueryAt(queryIndex)
		if !ok || !query.Key().Available() || !query.Family().Available() {
			return nil, false
		}
		if _, present := queries[query.Family()]; !present {
			return nil, false
		}
		for _, surface := range query.Surfaces() {
			if !surface.Available() || !appendGraphQuery(catalog, surface) {
				return nil, false
			}
		}
	}
	return catalog, true
}
