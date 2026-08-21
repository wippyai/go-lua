// runtime_binding_catalog.go collects the graph use rows, the schema Rule ref, the carry closures and the binding catalog.

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
	rule    *schemaRuleRef
	index   int
	surface equation.Surface
}

type graphWriteUse struct {
	rule    *schemaRuleRef
	index   int
	surface equation.Surface
}

// schemaRuleRef is a no-copy cold Rule proof. It names one canonical Schema
// row and exposes only scalar shape accessors; no Link callback or declaration schema
// pointer crosses into Factor carrier binding.
type schemaRuleRef struct {
	schema  *Schema
	ordinal uint64
}

func (ref *schemaRuleRef) valid() bool {
	return ref != nil && ref.schema != nil && ref.schema.Available() && ref.schema.ruleSemanticAt(ref.ordinal).Available()
}

func (ref *schemaRuleRef) shape() (composition.RuleShape, bool) {
	if !ref.valid() {
		return composition.RuleShape{}, false
	}
	return ref.schema.ruleShapeAt(ref.ordinal)
}

func (ref *schemaRuleRef) read(index uint64) (composition.RuleReadShape, bool) {
	if !ref.valid() {
		return composition.RuleReadShape{}, false
	}
	return ref.schema.ruleReadShapeAt(ref.ordinal, index)
}

func (ref *schemaRuleRef) carry(index uint64) (composition.RuleCarryShape, bool) {
	if !ref.valid() {
		return composition.RuleCarryShape{}, false
	}
	return ref.schema.ruleCarryShapeAt(ref.ordinal, index)
}

func (ref *schemaRuleRef) write(index uint64) (composition.RuleWriteShape, bool) {
	if !ref.valid() {
		return composition.RuleWriteShape{}, false
	}
	return ref.schema.ruleWriteShapeAt(ref.ordinal, index)
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

// buildGraphCarryClosures solves every Carry predecessor equation once during
// compilation. A closure is keyed by (Factor, Point), not Point alone, because
// one equation Point can transport independent factor roots. The per-factor
// predecessor graph is condensed with an iterative SCC walk, then its finite
// DAG is evaluated from predecessor-free components. This is the exact least
// closure of declared direct writes and Carry predecessors: no depth limit,
// cardinality cap, or runtime topology traversal is involved.
func buildGraphCarryClosures(schema *Schema, graph *equation.Graph, rules map[composition.Key]*schemaRuleRef) (map[graphCarryClosureKey]graphCarryClosure, bool) {
	if schema == nil || !schema.Available() || graph == nil || len(rules) == 0 {
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
			rule := rules[member.Rule()]
			shape, shapeOK := rule.shape()
			if !shapeOK || shape.CarryCount == 0 {
				continue
			}
			if shape.OutputKind != composition.FactorOutput || shape.CarryCount != 1 {
				return nil, false
			}
			factor := shape.Output
			carry, carryOK := rule.carry(0)
			if !factor.Available() || !carryOK || carry.Factor != factor {
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
			if !memberOK {
				return nil, false
			}
			rule := rules[member.Rule()]
			shape, shapeOK := rule.shape()
			if !shapeOK {
				return nil, false
			}
			// Carry closure is defined only over direct writes and carry
			// predecessors of carried Factor-output Rules. A structural member
			// has neither; it remains a valid graph member when an unrelated
			// Factor carries through the same graph.
			if shape.OutputKind != composition.FactorOutput {
				if shape.CarryCount != 0 {
					return nil, false
				}
				continue
			}
			factor := shape.Output
			if _, carried := carriedFactors[factor]; !carried {
				continue
			}
			node, nodeOK := ensureNode(factor, group.Output())
			if !nodeOK || uint64(member.WriteCount()) != shape.WriteCount {
				return nil, false
			}
			for writeIndex := uint64(0); writeIndex < shape.WriteCount; writeIndex++ {
				write, writeOK := rule.write(writeIndex)
				if !writeOK {
					return nil, false
				}
				surface, surfaceOK := member.WriteAt(int(writeIndex))
				if !surfaceOK {
					return nil, false
				}
				switch write.Kind {
				case composition.WriteExact:
					if write.Route != 0 {
						return nil, false
					}
					node.direct = append(node.direct, surface)
				case composition.WriteRoute:
					if write.Route == 0 || surface.Form != equation.SurfaceWriteRoute || surface.Mode != equation.TargetModeNone {
						return nil, false
					}
					node.route = true
				default:
					return nil, false
				}
			}
			if shape.CarryCount == 0 {
				continue
			}
			if shape.CarryCount != 1 {
				return nil, false
			}
			carry, carryOK := rule.carry(0)
			if !carryOK || carry.Factor != factor {
				return nil, false
			}
			input, inputOK := group.InputAt(int(carry.Input))
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

func buildGraphBindingCatalog(schema *Schema, graph *equation.Graph) (*graphBindingCatalog, bool) {
	if schema == nil || !schema.Available() || graph == nil || graph.CompositionID() != schema.cold.ID() {
		return nil, false
	}
	factorCount, ruleCount, queryCount, _, shapeOK := schema.shapeCount()
	if !shapeOK {
		return nil, false
	}
	rules := make(map[composition.Key]*schemaRuleRef, ruleCount)
	for ordinal := 0; ordinal < ruleCount; ordinal++ {
		key := schema.ruleSemanticAt(uint64(ordinal))
		if !key.Available() {
			return nil, false
		}
		ref := &schemaRuleRef{schema: schema, ordinal: uint64(ordinal)}
		if _, duplicate := rules[key]; duplicate {
			return nil, false
		}
		rules[key] = ref
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
	closures, closuresOK := buildGraphCarryClosures(schema, graph, rules)
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
			rule, present := rules[member.Rule()]
			shape, shapeOK := rule.shape()
			if !present || !shapeOK || uint64(member.ReadCount()) != shape.ReadCount || uint64(member.WriteCount()) != shape.WriteCount {
				return nil, false
			}
			if shape.CarryCount != 0 {
				if shape.OutputKind != composition.FactorOutput || shape.CarryCount != 1 {
					return nil, false
				}
				carry, carryOK := rule.carry(0)
				input, inputOK := group.InputAt(int(carry.Input))
				if !carryOK || !inputOK {
					return nil, false
				}
				closureKey := graphCarryClosureKey{factor: shape.Output, point: input.Point().Key()}
				closure, closureOK := catalog.carryClosures[closureKey]
				if !closureOK || !appendGraphCarryTargets(catalog, shape.Output, member.Key(), closure) {
					return nil, false
				}
				for _, target := range closure.targets {
					if !appendGraphTarget(catalog, target) {
						return nil, false
					}
				}
			}
			for readIndex := 0; readIndex < member.ReadCount(); readIndex++ {
				surface, ok := member.ReadAt(readIndex)
				if !ok || !surface.Available() {
					return nil, false
				}
				use := graphReadUse{rule: rule, index: readIndex, surface: surface}
				readShape, readOK := rule.read(uint64(readIndex))
				if !readOK || !readShape.Factor.Available() {
					return nil, false
				}
				if !appendGraphRead(catalog, surface, use) {
					return nil, false
				}
			}
			for writeIndex := 0; writeIndex < member.WriteCount(); writeIndex++ {
				surface, ok := member.WriteAt(writeIndex)
				if !ok || !surface.Available() {
					return nil, false
				}
				use := graphWriteUse{rule: rule, index: writeIndex, surface: surface}
				if _, writeOK := rule.write(uint64(writeIndex)); !writeOK {
					return nil, false
				}
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
