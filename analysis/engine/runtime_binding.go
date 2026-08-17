package engine

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/identity"
)

type readFormKind uint8

const (
	exactReadForm readFormKind = iota + 1
	summaryReadForm
)

type boundUnit struct {
	unit        carrier.Unit
	kind        carrier.UnitKind
	local       uint64
	summaryKeys []uint64
}

type boundTarget struct {
	target carrier.Target
	mode   carrier.TargetMode
	local  uint64
}

// boundFactor is the private concrete half of a cold Factor. It is made by
// assembly, not declaration: Factor remains a cold owner capability and never
// receives a carrier slot, binding, Unit, Target, or Selector.
type boundFactor[K ~uint32 | ~uint64, V any] struct {
	implementation *FactorImplementation[K, V]
	receipt        factorRuntimeReceipt
	binding        *factbinding.Binding[K, V]
	slot           shape.Slot
	hasSlot        bool
	reads          map[equation.Surface]boundUnit
	writes         map[equation.Surface]boundTarget
	// dynamicUnits is the one Factor-owned exact Unit universe needed by
	// staged reads. It is allocated once only when a sealed ReadSelect targets
	// this Factor; no Rule/input/root candidate product is retained.
	dynamicUnits []carrier.Unit
	// routeTargets is the Factor-owned presealed strong target universe paired
	// positionally with dynamicUnits. It exists once for a Factor that owns a
	// route write, never once per Rule member or source root.
	routeTargets     []carrier.Target
	routeTransform   factbinding.TransformClosure[K, V]
	routeTransformOK bool
	carryTargets     map[composition.Key][]carrier.Target
	carryRouteScope  map[composition.Key]bool
}

type runtimeFactor interface {
	semantic() identity.SemanticKey
	operation() carrier.FactorOperation
	runtimeSlot() (shape.Slot, bool)
	carryTargetsFor(equation.RuleMember) ([]carrier.Target, bool)
	supports(carrier.MergeKind) bool
	readUnit(equation.Surface) (carrier.Unit, bool)
	writeTarget(equation.Surface) (carrier.Target, bool)
	hasRouteUniverse() bool
	routeUniverse() []carrier.Target
	carryRouteScopeFor(equation.RuleMember) bool
	releaseColdBindings()
}

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

// runtimeBinding is the one private binding cut from the equation graph's closed
// decision catalog to the carrier's dense guard universe.  It is deliberately
// graph-owned: a caller cannot choose atoms, their order, or a second Manager
// for a set of Factors.
type runtimeBinding struct {
	schema    *Schema
	state     *schemaBindingState
	authority *schemaBindingAuthority
	mode      runtimeBindingMode
	graph     *equation.Graph
	guards    *guard.Manager
	catalog   *graphBindingCatalog // cold only; sole compiler releases it after binding
	validated bool                 // newRuntimeBinding checked the dense atom catalog once
}

type runtimeBindingMode uint8

const (
	runtimeBindingReceipt runtimeBindingMode = iota + 1
)

// newRuntimeBinding derives dense atoms in the Graph catalog order.
// Atom numbers are implementation-local dense ranks; equation Decisions stay
// the only semantic identity.
func newRuntimeBinding(schema *Schema, graph *equation.Graph) (*runtimeBinding, bool) {
	if schema == nil || !schema.Available() || graph == nil || !graph.OwnsComposition(schema.cold) || graph.CompositionID() != schema.cold.ID() {
		return nil, false
	}
	catalog, catalogOK := buildGraphBindingCatalog(schema, graph)
	if !catalogOK || catalog == nil {
		return nil, false
	}
	atoms := make([]guard.Atom, graph.DecisionCount())
	for index := range atoms {
		decision, ok := graph.DecisionAt(index)
		if !ok || !decision.Available() || index > 0 {
			previous, previousOK := graph.DecisionAt(index - 1)
			if !ok || !decision.Available() || !previousOK || !previous.Available() || decision.Key() == previous.Key() {
				return nil, false
			}
		}
		atoms[index] = guard.Atom(index + 1)
	}
	manager, err := guard.New(atoms)
	if err != nil || manager == nil {
		return nil, false
	}
	return &runtimeBinding{schema: schema, mode: runtimeBindingReceipt, graph: graph, guards: manager, catalog: catalog, validated: true}, true
}

// newReceiptRuntimeBinding is the pre-fenced constructor for the callback-free
// Factor vertical.  It shares the same graph/catalog/runtime path as the
// constructor publishes the exact sealed SchemaBinding state and
// authority before any Factor implementation can be consumed.  A receipt can
// therefore never claim ownership of an unpinned runtime or mix bindings.
func newReceiptRuntimeBinding(binding *SchemaBinding, graph *equation.Graph) (*runtimeBinding, bool) {
	state := bindingState(binding)
	if state == nil {
		return nil, false
	}
	state.mu.Lock()
	schema, authority, sealed := state.schema, state.authority, state.phase == schemaBindingSealed && state.authority != nil
	state.mu.Unlock()
	if !sealed {
		return nil, false
	}
	runtime, ok := newRuntimeBinding(schema, graph)
	if !ok || runtime == nil {
		return nil, false
	}
	runtime.mode, runtime.state, runtime.authority = runtimeBindingReceipt, state, authority
	return runtime, true
}

func (binding *runtimeBinding) valid() bool {
	// The Graph and Manager are immutable after successful construction. The
	// complete decision/atom correspondence is proved above once; rewalking it
	// for every Factor would turn cold binding into F×Decision work.
	return binding != nil && binding.validated && binding.mode == runtimeBindingReceipt && binding.schema != nil && binding.schema.Available() && binding.graph != nil && binding.graph.CompositionID() == binding.schema.coldID() && binding.guards != nil && binding.state != nil && binding.authority != nil && binding.state.authority == binding.authority && binding.state.schema == binding.schema && binding.state.phase == schemaBindingSealed
}

func (binding *runtimeBinding) pinReceipt(receipt factorRuntimeReceipt) bool {
	if binding == nil || binding.mode != runtimeBindingReceipt || !receipt.valid() || receipt.schema != binding.schema {
		return false
	}
	return binding.state == receipt.state && binding.authority == receipt.authority
}

// takeFactorUses consumes one typed binder's cold graph partition. It is a
// one-shot compiler operation: a failed or duplicate bind cannot keep a
// second materialization route alive.
func (binding *runtimeBinding) takeFactorUses(key composition.Key) (graphFactorUses, bool) {
	if binding == nil || !binding.valid() || binding.catalog == nil || binding.catalog.factors == nil || !key.Available() {
		return graphFactorUses{}, false
	}
	uses, ok := binding.catalog.factors[key]
	if !ok || uses == nil {
		return graphFactorUses{}, false
	}
	delete(binding.catalog.factors, key)
	return *uses, true
}

// freezeCatalog is the compiler's release cut after every sealed Factor has
// bound. Runtime execution retains only concrete handles in members/queries;
// it must never retain graph lookup maps, vectors, or typed schema pointers.
func (binding *runtimeBinding) freezeCatalog() bool {
	if binding == nil || !binding.valid() || binding.catalog == nil || binding.catalog.factors == nil || len(binding.catalog.factors) != 0 {
		return false
	}
	binding.catalog.factors = nil
	binding.catalog = nil
	return true
}

// summaryUnitKey is the carrier Unit identity of one declared summary read.
// Two surfaces share a Unit only when they name the same canonical key vector
// under the same declared fold: a coordinate-wise reader and a correlated
// reader of the same keys observe different partitions, so they are distinct
// Units even though their key vectors agree.
type summaryUnitKey struct {
	representative equation.Surface
	distributive   bool
}

type summaryBindingRow[K ~uint32 | ~uint64] struct {
	unit summaryUnitKey
	keys []K
}

// surfaceVectorRow keeps selector vectors positional. Weak rows later sort
// their resolved Units because weak coverage is a set; selector rows never do.
type surfaceVectorRow struct {
	surface    equation.Surface
	candidates []equation.Surface
}

type factorGraphCatalog[K ~uint32 | ~uint64] struct {
	exactReads     []equation.Surface
	summaries      []summaryBindingRow[K]
	summaryAliases map[equation.Surface]summaryUnitKey
	strongWrites   []equation.Surface
	weakWrites     []surfaceVectorRow
	dynamicRead    bool
	routeWrite     bool
	carryTargets   []carryTargetRow
}

type carryTargetRow struct {
	member  composition.Key
	targets []equation.Surface
	route   bool
}

func bindFactorFromGraph[K ~uint32 | ~uint64, V any](implementation *FactorImplementation[K, V], runtime *runtimeBinding) (*boundFactor[K, V], bool) {
	if implementation == nil || implementation.algebra == nil || !implementation.descriptor.valid() || runtime == nil || !runtime.valid() {
		return nil, false
	}
	descriptor := implementation.descriptor
	receipt := implementation.receipt
	if runtime.mode == runtimeBindingReceipt {
		if !receipt.valid() {
			return nil, false
		}
		if !runtime.pinReceipt(receipt) {
			return nil, false
		}
		descriptor = factorRuntimeDescriptor{schema: receipt.schema, state: receipt.state, ordinal: receipt.ordinal, semantic: receipt.semantic, keyEnd: receipt.keyEnd, algebra: receipt.algebra}
	} else {
		return nil, false
	}
	if runtime.graph.CompositionID() != descriptor.schema.coldID() {
		return nil, false
	}
	index, indexed := descriptor.schema.cold.FactorIndex(descriptor.semantic)
	if !indexed || index != descriptor.ordinal {
		return nil, false
	}
	uses, taken := runtime.takeFactorUses(descriptor.semantic)
	if !taken {
		return nil, false
	}
	catalog, catalogOK := collectFactorGraphCatalog[K, V](descriptor, runtime.graph, uses)
	if !catalogOK {
		return nil, false
	}
	bound := &boundFactor[K, V]{
		implementation:  implementation,
		receipt:         receipt,
		reads:           make(map[equation.Surface]boundUnit, len(catalog.exactReads)+len(catalog.summaryAliases)),
		writes:          make(map[equation.Surface]boundTarget, len(catalog.strongWrites)+len(catalog.weakWrites)),
		carryTargets:    make(map[composition.Key][]carrier.Target, len(catalog.carryTargets)),
		carryRouteScope: make(map[composition.Key]bool, len(catalog.carryTargets)),
	}
	binding, ok := factbinding.Bind(implementation.algebra, runtime.guards, func(binding *factbinding.Binding[K, V]) bool {
		if catalog.dynamicRead {
			// A staged target Factor declares each exact Unit once, in owner key
			// order. This is O(R) per actually targeted Factor, rather than the
			// former candidate×root cold surface. Static exact reads still share
			// these same Units through bound.reads.
			if implementation.algebra.KeyEnd() > uint64(^uint(0)>>1) {
				return false
			}
			bound.dynamicUnits = make([]carrier.Unit, int(implementation.algebra.KeyEnd()))
			exactIndex := 0
			for raw := uint64(0); raw < implementation.algebra.KeyEnd(); raw++ {
				key := K(raw)
				if uint64(key) != raw {
					return false
				}
				unit, declared := binding.DeclareExact(key)
				if !declared {
					return false
				}
				bound.dynamicUnits[int(raw)] = unit
				for exactIndex < len(catalog.exactReads) && catalog.exactReads[exactIndex].Local == raw+1 {
					surface := catalog.exactReads[exactIndex]
					bound.reads[surface] = boundUnit{unit: unit, kind: carrier.ExactUnit, local: surface.Local}
					exactIndex++
				}
			}
			if exactIndex != len(catalog.exactReads) {
				return false
			}
		} else {
			for _, surface := range catalog.exactReads {
				if surface.Local == 0 {
					return false
				}
				raw := surface.Local - 1
				key := K(raw)
				if uint64(key) != raw {
					return false
				}
				unit, declared := binding.DeclareExact(key)
				if !declared {
					return false
				}
				bound.reads[surface] = boundUnit{unit: unit, kind: carrier.ExactUnit, local: surface.Local}
			}
		}
		summaryUnits := make(map[summaryUnitKey]boundUnit, len(catalog.summaries))
		for ordinal, summary := range catalog.summaries {
			var unit carrier.Unit
			var declared bool
			if summary.unit.distributive {
				unit, declared = binding.DeclareDistributiveSummary(summary.keys)
			} else {
				unit, declared = binding.DeclareSummary(summary.keys)
			}
			if !declared {
				return false
			}
			keys := make([]uint64, len(summary.keys))
			for index, key := range summary.keys {
				keys[index] = uint64(key)
			}
			summaryUnits[summary.unit] = boundUnit{unit: unit, kind: carrier.SummaryUnit, local: uint64(ordinal) + 1, summaryKeys: keys}
		}
		for alias, summary := range catalog.summaryAliases {
			unit, present := summaryUnits[summary]
			if !present {
				return false
			}
			bound.reads[alias] = unit
		}
		if catalog.routeWrite {
			if !catalog.dynamicRead || len(bound.dynamicUnits) != int(implementation.algebra.KeyEnd()) {
				return false
			}
			bound.routeTargets = make([]carrier.Target, len(bound.dynamicUnits))
			for index, unit := range bound.dynamicUnits {
				if unit == (carrier.Unit{}) {
					return false
				}
				target, declared := binding.DeclareStrong(unit)
				if !declared {
					return false
				}
				bound.routeTargets[index] = target
			}
		}
		for _, surface := range catalog.strongWrites {
			unit, present := bound.reads[equation.Surface{Factor: descriptor.semantic, Form: equation.SurfaceReadExact, Local: surface.Local}]
			if !present {
				return false
			}
			var target carrier.Target
			if catalog.routeWrite {
				raw := surface.Local - 1
				if raw >= uint64(len(bound.routeTargets)) {
					return false
				}
				target = bound.routeTargets[int(raw)]
			} else {
				var declared bool
				target, declared = binding.DeclareStrong(unit.unit)
				if !declared {
					return false
				}
			}
			bound.writes[surface] = boundTarget{target: target, mode: carrier.StrongTarget, local: surface.Local}
		}
		return declareWeakTargets(binding, bound, catalog.weakWrites)
	})
	if !ok || binding == nil || len(bound.reads) != len(catalog.exactReads)+len(catalog.summaryAliases) || len(bound.writes) != len(catalog.strongWrites)+len(catalog.weakWrites) || catalog.dynamicRead && len(bound.dynamicUnits) != int(implementation.algebra.KeyEnd()) || catalog.routeWrite && len(bound.routeTargets) != int(implementation.algebra.KeyEnd()) {
		return nil, false
	}
	for _, row := range catalog.carryTargets {
		targets := make([]carrier.Target, len(row.targets))
		for index, surface := range row.targets {
			target, present := bound.writes[surface]
			if !present {
				return nil, false
			}
			targets[index] = target.target
		}
		bound.carryTargets[row.member] = targets
		bound.carryRouteScope[row.member] = row.route
	}
	bound.binding = binding
	return bound, true
}

func exactReadDescriptorSurface(descriptor factorRuntimeDescriptor, local uint64) equation.Surface {
	return equation.Surface{Factor: descriptor.semantic, Form: equation.SurfaceReadExact, Local: local}
}

func exactWriteReceiptSurface(receipt factorRuntimeReceipt, local uint64) equation.Surface {
	return equation.Surface{Factor: receipt.semantic, Form: equation.SurfaceWriteExact, Local: local, Mode: equation.TargetModeStrong}
}

func matchesFactorReadShape(schema *Schema, ordinal uint64, surface equation.Surface, kind readFormKind) bool {
	if schema == nil || !schema.Available() || ordinal >= schema.factorCount() || !surface.Available() || surface.Factor != schema.factorSemanticAt(ordinal) {
		return false
	}
	if kind == exactReadForm {
		return surface.Form == equation.SurfaceReadExact && surface.Mode == equation.TargetModeNone && !surface.Semantic.Available() && !surface.Normalizer.Available()
	}
	if kind != summaryReadForm || surface.Form != equation.SurfaceReadSummary || surface.Mode != equation.TargetModeNone || !surface.Semantic.Available() || surface.Normalizer != surface.Semantic {
		return false
	}
	count, ok := schema.factorFormCount(ordinal)
	if !ok {
		return false
	}
	for index := 0; index < count; index++ {
		form, formOK := schema.factorFormShapeAt(ordinal, uint64(index))
		if formOK && summaryReadRowKind(form.Kind) && form.Semantic == surface.Semantic {
			return true
		}
	}
	return false
}

// summaryReadFormFold resolves the declared fold of one summary read form
// from the sealed cold schema. The normalizer key names exactly one declared
// form, so the fold is recovered without any Rule, Query, or caller input.
func summaryReadFormFold(schema *Schema, ordinal uint64, semantic composition.Key) (bool, bool) {
	if schema == nil || !schema.Available() || ordinal >= schema.factorCount() || !semantic.Available() {
		return false, false
	}
	count, ok := schema.factorFormCount(ordinal)
	if !ok {
		return false, false
	}
	for index := 0; index < count; index++ {
		form, formOK := schema.factorFormShapeAt(ordinal, uint64(index))
		if !formOK || form.Semantic != semantic || !summaryReadRowKind(form.Kind) {
			continue
		}
		return form.Kind == composition.FactorDistributiveSummaryRead, true
	}
	return false, false
}

// collectFactorGraphCatalog performs only cold work. Graph owns every
// occurrence surface, summary key row, weak cover, and selector target; the
// Factor owns only the typed conversion from raw key to K and the carrier
// declarations. There is no caller-supplied materialization language.
func collectFactorGraphCatalog[K ~uint32 | ~uint64, V any](descriptor factorRuntimeDescriptor, graph *equation.Graph, uses graphFactorUses) (factorGraphCatalog[K], bool) {
	if descriptor.schema == nil || !descriptor.schema.Available() || descriptor.algebra == nil || graph == nil {
		return factorGraphCatalog[K]{}, false
	}
	key := descriptor.semantic
	exact := make(map[equation.Surface]struct{})
	summaries := make(map[summaryUnitKey][]K)
	aliases := make(map[equation.Surface]summaryUnitKey)
	strong := make(map[equation.Surface]struct{})
	weak := make(map[equation.Surface][]equation.Surface)
	dynamicRead := false
	routeWrite := false

	var collectRead func(equation.Surface) bool
	collectSummary := func(surface equation.Surface) bool {
		if !matchesFactorReadShape(descriptor.schema, descriptor.ordinal, surface, summaryReadForm) || surface.Mode != equation.TargetModeNone {
			return false
		}
		// The fold is read from the declared cold form the surface names, so a
		// summary can never acquire a fold from the Rule or Query that reads it.
		distributive, foldOK := summaryReadFormFold(descriptor.schema, descriptor.ordinal, surface.Semantic)
		if !foldOK {
			return false
		}
		representative, represented := graph.SummaryRepresentative(surface)
		if !represented || !matchesFactorReadShape(descriptor.schema, descriptor.ordinal, representative, summaryReadForm) {
			return false
		}
		count, counted := graph.SummaryKeyCount(representative)
		if !counted || count == 0 {
			return false
		}
		keys := make([]K, count)
		for index := range keys {
			raw, present := graph.SummaryKeyAt(representative, index)
			if !present || raw >= descriptor.keyEnd {
				return false
			}
			keys[index] = K(raw)
			if uint64(keys[index]) != raw || index > 0 && keys[index-1] >= keys[index] {
				return false
			}
		}
		unit := summaryUnitKey{representative: representative, distributive: distributive}
		if prior, exists := summaries[unit]; exists && !sameSummaryKeys(prior, keys) {
			return false
		}
		summaries[unit] = keys
		aliases[surface] = unit
		if _, present := aliases[representative]; !present {
			representativeDistributive, representativeFoldOK := summaryReadFormFold(descriptor.schema, descriptor.ordinal, representative.Semantic)
			if !representativeFoldOK {
				return false
			}
			representativeUnit := summaryUnitKey{representative: representative, distributive: representativeDistributive}
			if prior, exists := summaries[representativeUnit]; exists && !sameSummaryKeys(prior, keys) {
				return false
			}
			summaries[representativeUnit] = keys
			aliases[representative] = representativeUnit
		}
		return true
	}
	collectRead = func(surface equation.Surface) bool {
		if !surface.Available() || surface.Factor != key || surface.Mode != equation.TargetModeNone {
			return false
		}
		switch surface.Form {
		case equation.SurfaceReadExact:
			if surface.Semantic.Available() || surface.Normalizer.Available() || surface.Local == 0 || surface.Local > descriptor.keyEnd {
				return false
			}
			exact[surface] = struct{}{}
			return true
		case equation.SurfaceReadSummary:
			return collectSummary(surface)
		default:
			return false
		}
	}
	collectWeak := func(surface equation.Surface) bool {
		if surface.Factor != key || surface.Form != equation.SurfaceWriteExact || surface.Mode != equation.TargetModeWeak || surface.Semantic.Available() || surface.Normalizer.Available() {
			return false
		}
		count, counted := graph.WeakTargetCandidateCount(surface)
		if !counted || count == 0 {
			return false
		}
		candidates := make([]equation.Surface, count)
		for index := range candidates {
			candidate, present := graph.WeakTargetCandidateAt(surface, index)
			if !present || !collectRead(candidate) {
				return false
			}
			candidates[index] = candidate
		}
		if prior, exists := weak[surface]; exists && !sameSurfaceVector(prior, candidates) {
			return false
		}
		weak[surface] = candidates
		return true
	}
	collectTarget := func(surface equation.Surface) bool {
		if !surface.Available() || surface.Factor != key || surface.Form != equation.SurfaceWriteExact || surface.Semantic.Available() || surface.Normalizer.Available() {
			return false
		}
		switch surface.Mode {
		case equation.TargetModeStrong:
			if surface.Local == 0 || surface.Local > descriptor.keyEnd {
				return false
			}
			strong[surface] = struct{}{}
			exact[exactReadDescriptorSurface(descriptor, surface.Local)] = struct{}{}
			return true
		case equation.TargetModeWeak:
			return collectWeak(surface)
		default:
			return false
		}
	}
	for _, use := range uses.reads {
		readShape, readOK := use.rule.read(uint64(use.index))
		if use.rule == nil || use.index < 0 || !readOK || readShape.Factor != key {
			return factorGraphCatalog[K]{}, false
		}
		if use.surface.Form == equation.SurfaceReadSelect {
			// A ReadSelect names its target Factor only. Exact Ref routes are
			// chosen row-locally by the staged locator; no target Unit or
			// candidate vector is present in the graph catalog.
			if readShape.Kind != composition.ReadSelect || readShape.DependencyCount == 0 ||
				use.surface.Mode != equation.TargetModeNone || use.surface.Semantic != key || use.surface.Normalizer.Available() || !use.surface.LocalAvailable() {
				return factorGraphCatalog[K]{}, false
			}
			dynamicRead = true
			continue
		}
		if readShape.DependencyCount != 0 || !collectRead(use.surface) {
			return factorGraphCatalog[K]{}, false
		}
	}
	for _, use := range uses.writes {
		writeShape, writeOK := use.rule.write(uint64(use.index))
		if use.rule == nil || use.index < 0 || !writeOK || writeShape.Factor != key {
			return factorGraphCatalog[K]{}, false
		}
		switch writeShape.Kind {
		case composition.WriteExact:
			if writeShape.Route != 0 || !collectTarget(use.surface) {
				return factorGraphCatalog[K]{}, false
			}
		case composition.WriteRoute:
			if writeShape.Route == 0 || use.surface.Form != equation.SurfaceWriteRoute || use.surface.Mode != equation.TargetModeNone || use.surface.Semantic.Available() || use.surface.Normalizer.Available() {
				return factorGraphCatalog[K]{}, false
			}
			routeWrite = true
		default:
			return factorGraphCatalog[K]{}, false
		}
	}
	for _, target := range uses.targets {
		if !collectTarget(target) {
			return factorGraphCatalog[K]{}, false
		}
	}
	for _, closure := range uses.carryTargets {
		for _, target := range closure.targets {
			if !collectTarget(target) {
				return factorGraphCatalog[K]{}, false
			}
		}
	}
	for _, surface := range uses.queries {
		if surface.Form == equation.SurfaceReadSelect || !collectRead(surface) {
			return factorGraphCatalog[K]{}, false
		}
	}
	if routeWrite && !dynamicRead {
		return factorGraphCatalog[K]{}, false
	}
	result := factorGraphCatalog[K]{summaryAliases: aliases, dynamicRead: dynamicRead, routeWrite: routeWrite}
	for surface := range exact {
		result.exactReads = append(result.exactReads, surface)
	}
	sort.Slice(result.exactReads, func(left, right int) bool {
		return result.exactReads[left].Local < result.exactReads[right].Local
	})
	for unit, keys := range summaries {
		result.summaries = append(result.summaries, summaryBindingRow[K]{unit: unit, keys: keys})
	}
	sort.Slice(result.summaries, func(left, right int) bool {
		if comparison := equation.CompareKeyVectors(result.summaries[left].keys, result.summaries[right].keys); comparison != 0 {
			return comparison < 0
		}
		if result.summaries[left].unit.distributive != result.summaries[right].unit.distributive {
			return !result.summaries[left].unit.distributive
		}
		return lessRuntimeSurface(result.summaries[left].unit.representative, result.summaries[right].unit.representative)
	})
	for surface := range strong {
		result.strongWrites = append(result.strongWrites, surface)
	}
	sort.Slice(result.strongWrites, func(left, right int) bool {
		return lessRuntimeSurface(result.strongWrites[left], result.strongWrites[right])
	})
	result.weakWrites = sortedSurfaceVectors(weak)
	for member, closure := range uses.carryTargets {
		result.carryTargets = append(result.carryTargets, carryTargetRow{member: member, targets: append([]equation.Surface(nil), closure.targets...), route: closure.route})
	}
	sort.Slice(result.carryTargets, func(left, right int) bool {
		return lessRuntimeKey(result.carryTargets[left].member, result.carryTargets[right].member)
	})
	return result, true
}

func sameSurfaceVector(left, right []equation.Surface) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sortedSurfaceVectors(rows map[equation.Surface][]equation.Surface) []surfaceVectorRow {
	result := make([]surfaceVectorRow, 0, len(rows))
	for surface, candidates := range rows {
		result = append(result, surfaceVectorRow{surface: surface, candidates: candidates})
	}
	sort.Slice(result, func(left, right int) bool { return lessRuntimeSurface(result[left].surface, result[right].surface) })
	return result
}

func compactCarrySurfaces(surfaces []equation.Surface) []equation.Surface {
	if len(surfaces) < 2 {
		return surfaces
	}
	sort.Slice(surfaces, func(left, right int) bool { return lessRuntimeSurface(surfaces[left], surfaces[right]) })
	end := 1
	for _, surface := range surfaces[1:] {
		if surface != surfaces[end-1] {
			surfaces[end] = surface
			end++
		}
	}
	return surfaces[:end]
}

func lessRuntimeSurface(left, right equation.Surface) bool {
	if comparison := compareRuntimeKey(left.Factor, right.Factor); comparison != 0 {
		return comparison < 0
	}
	if left.Form != right.Form {
		return left.Form < right.Form
	}
	if left.Local != right.Local {
		return left.Local < right.Local
	}
	if comparison := bytes.Compare(left.Content[:], right.Content[:]); comparison != 0 {
		return comparison < 0
	}
	if left.Mode != right.Mode {
		return left.Mode < right.Mode
	}
	if comparison := compareRuntimeKey(left.Semantic, right.Semantic); comparison != 0 {
		return comparison < 0
	}
	return compareRuntimeKey(left.Normalizer, right.Normalizer) < 0
}

func compareRuntimeKey(left, right composition.Key) int {
	for index := range left.ID {
		if left.ID[index] < right.ID[index] {
			return -1
		}
		if left.ID[index] > right.ID[index] {
			return 1
		}
	}
	if left.Version < right.Version {
		return -1
	}
	if left.Version > right.Version {
		return 1
	}
	return 0
}

func sameSummaryKeys[K ~uint32 | ~uint64](left, right []K) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func unitLess(left, right boundUnit) bool {
	return left.kind < right.kind || left.kind == right.kind && left.local < right.local
}

type resolvedWeakTarget struct {
	surface    equation.Surface
	candidates []boundUnit
}

func declareWeakTargets[K ~uint32 | ~uint64, V any](binding *factbinding.Binding[K, V], bound *boundFactor[K, V], plans []surfaceVectorRow) bool {
	resolved := make([]resolvedWeakTarget, len(plans))
	for index, plan := range plans {
		if plan.surface.Form != equation.SurfaceWriteExact || plan.surface.Mode != equation.TargetModeWeak || len(plan.candidates) == 0 {
			return false
		}
		candidates := make([]boundUnit, len(plan.candidates))
		for candidateIndex, surface := range plan.candidates {
			unit, ok := bound.reads[surface]
			if !ok {
				return false
			}
			candidates[candidateIndex] = unit
		}
		sort.Slice(candidates, func(left, right int) bool { return unitLess(candidates[left], candidates[right]) })
		for candidateIndex := range candidates {
			if candidateIndex > 0 && !unitLess(candidates[candidateIndex-1], candidates[candidateIndex]) {
				return false
			}
		}
		resolved[index] = resolvedWeakTarget{surface: plan.surface, candidates: candidates}
	}
	sort.Slice(resolved, func(left, right int) bool {
		if lessUnitVector(resolved[left].candidates, resolved[right].candidates) {
			return true
		}
		if lessUnitVector(resolved[right].candidates, resolved[left].candidates) {
			return false
		}
		return lessRuntimeSurface(resolved[left].surface, resolved[right].surface)
	})
	for index, weak := range resolved {
		if index > 0 && sameUnitVector(resolved[index-1].candidates, weak.candidates) {
			bound.writes[weak.surface] = bound.writes[resolved[index-1].surface]
			continue
		}
		if index > 0 && !lessUnitVector(resolved[index-1].candidates, weak.candidates) {
			return false
		}
		units := make([]carrier.Unit, len(weak.candidates))
		for candidateIndex, candidate := range weak.candidates {
			units[candidateIndex] = candidate.unit
		}
		target, ok := binding.DeclareWeak(units)
		if !ok {
			return false
		}
		bound.writes[weak.surface] = boundTarget{target: target, mode: carrier.WeakTarget, local: uint64(index) + 1}
	}
	return true
}

func lessUnitVector(left, right []boundUnit) bool {
	for index := 0; index < len(left) && index < len(right); index++ {
		if unitLess(left[index], right[index]) {
			return true
		}
		if unitLess(right[index], left[index]) {
			return false
		}
	}
	return len(left) < len(right)
}

func sameUnitVector(left, right []boundUnit) bool {
	return !lessUnitVector(left, right) && !lessUnitVector(right, left)
}

func (bound *boundFactor[K, V]) semantic() identity.SemanticKey {
	if bound == nil || bound.implementation == nil || !bound.implementation.descriptor.valid() {
		return identity.SemanticKey{}
	}
	semantic, ok := semanticKeyFromComposition(bound.implementation.descriptor.semantic)
	if !ok {
		return identity.SemanticKey{}
	}
	return semantic
}

func (bound *boundFactor[K, V]) operation() carrier.FactorOperation {
	if bound == nil {
		return nil
	}
	return bound.binding
}

// supports is cold typed recurrence metadata.  Assembly uses it to derive the
// exact Narrow-capable subset of an occurrence footprint before carrier scopes
// are sealed; it never probes a live operation or falls back after a rejected
// scope.
func (bound *boundFactor[K, V]) supports(kind carrier.MergeKind) bool {
	return bound != nil && bound.binding != nil && bound.binding.Supports(kind)
}

func (bound *boundFactor[K, V]) bindRuntimeSlot(slot shape.Slot) bool {
	if bound == nil || bound.binding == nil || bound.hasSlot || slot < 0 {
		return false
	}
	bound.slot, bound.hasSlot = slot, true
	return true
}

func (bound *boundFactor[K, V]) runtimeSlot() (shape.Slot, bool) {
	if bound == nil || !bound.hasSlot {
		return 0, false
	}
	return bound.slot, true
}

func (bound *boundFactor[K, V]) carryTargetsFor(member equation.RuleMember) ([]carrier.Target, bool) {
	if bound == nil || !member.Key().Available() {
		return nil, false
	}
	targets, ok := bound.carryTargets[member.Key()]
	if !ok {
		return nil, false
	}
	return append([]carrier.Target(nil), targets...), true
}

func (bound *boundFactor[K, V]) readUnit(surface equation.Surface) (carrier.Unit, bool) {
	if bound == nil {
		return carrier.Unit{}, false
	}
	unit, ok := bound.reads[surface]
	return unit.unit, ok
}

// receiptMatches is the factor-owned authority bridge for receipt queries.
// It deliberately exposes no coordinate type: the sealed Factor cell checks
// the exact SchemaBinding, factor ordinal, and semantic key internally.
func (bound *boundFactor[K, V]) receiptMatches(state *schemaBindingState, authority *schemaBindingAuthority, ordinal uint64, semantic composition.Key) bool {
	return bound != nil && bound.receipt.valid() && bound.receipt.state == state && bound.receipt.authority == authority && bound.receipt.ordinal == ordinal && bound.receipt.semantic == semantic
}

// summaryReadReceiptProof is the slot-native counterpart of summaryReadProof.
// It authenticates the exact sealed form ordinal and semantic key without
// reconstructing a declaration form or consulting a semantic lookup table.
func (bound *boundFactor[K, V]) summaryReadReceiptProof(surface equation.Surface, formOrdinal uint64, semantic composition.Key) (ruleSummaryReadProof, bool) {
	if bound == nil || !bound.receipt.valid() || !semantic.Available() {
		return ruleSummaryReadProof{}, false
	}
	unit, found := bound.reads[surface]
	if !found || unit.kind != carrier.SummaryUnit || len(unit.summaryKeys) == 0 || !matchesFactorReadShape(bound.receipt.schema, bound.receipt.ordinal, surface, summaryReadForm) || surface.Semantic != semantic || surface.Normalizer != semantic {
		return ruleSummaryReadProof{}, false
	}
	formReceipt, formOK := bound.receipt.formAt(formOrdinal, SchemaFormReadSummary, semantic)
	if !formOK {
		return ruleSummaryReadProof{}, false
	}
	return ruleSummaryReadProof{receipt: bound.receipt, formReceipt: formReceipt, keys: unit.summaryKeys, digest: SummaryVectorDigest(unit.summaryKeys)}, true
}

// stagedUnit resolves only a Factor-issued exact Ref through the predeclared
// dynamic exact universe. The Ref validates the same sealed Factor before its
// raw coordinate is used; no key, Unit, or graph lookup is exposed to the
// locator.
func (bound *boundFactor[K, V]) stagedUnit(ref exactRef) (carrier.Unit, bool) {
	if bound == nil || len(bound.dynamicUnits) == 0 || ref == nil {
		return carrier.Unit{}, false
	}
	var raw uint64
	var ok bool
	if bound.receipt.valid() {
		if typed, valid := ref.(interface {
			receiptRaw(factorRuntimeReceipt) (uint64, bool)
		}); valid {
			raw, ok = typed.receiptRaw(bound.receipt)
		}
	}
	if !ok || raw >= uint64(len(bound.dynamicUnits)) {
		return carrier.Unit{}, false
	}
	unit := bound.dynamicUnits[int(raw)]
	if unit == (carrier.Unit{}) {
		return carrier.Unit{}, false
	}
	return unit, true
}

// stagedTarget resolves the same authenticated exact Ref through the
// presealed route-target universe. Its positional pairing with stagedUnit is
// established during Factor binding, so runtime never declares a target or
// reconstructs a key after sealing.
func (bound *boundFactor[K, V]) stagedTarget(ref exactRef) (carrier.Target, ruleTargetProof, bool) {
	if bound == nil || len(bound.routeTargets) != len(bound.dynamicUnits) || len(bound.routeTargets) == 0 || ref == nil {
		return carrier.Target{}, ruleTargetProof{}, false
	}
	var raw uint64
	var ok bool
	if bound.receipt.valid() {
		if typed, valid := ref.(interface {
			receiptRaw(factorRuntimeReceipt) (uint64, bool)
		}); valid {
			raw, ok = typed.receiptRaw(bound.receipt)
		}
	}
	if !ok || raw >= uint64(len(bound.routeTargets)) {
		return carrier.Target{}, ruleTargetProof{}, false
	}
	target := bound.routeTargets[int(raw)]
	if target == (carrier.Target{}) || target.Mode() != carrier.StrongTarget {
		return carrier.Target{}, ruleTargetProof{}, false
	}
	var proof ruleTargetProof
	var proven bool
	if bound.receipt.valid() {
		proof, proven = newRuleTargetReceiptProof(bound.receipt, exactWriteReceiptSurface(bound.receipt, raw+1))
	}
	if !proven {
		return carrier.Target{}, ruleTargetProof{}, false
	}
	return target, proof, true
}

func (bound *boundFactor[K, V]) routeUniverse() []carrier.Target {
	if bound == nil || len(bound.routeTargets) == 0 {
		return nil
	}
	// routeTargets is sealed Factor-owned data. Runtime consumers only range
	// over this immutable view or append its elements into their own scoped
	// seal buffer; returning a copy here would recreate the old per-member
	// route-universe materialization that the recurrence cut deliberately
	// moved to (Region, Factor).
	return bound.routeTargets
}

func (bound *boundFactor[K, V]) routeTransformClosure() (factbinding.TransformClosure[K, V], bool) {
	if bound == nil || !bound.routeTransformOK {
		return factbinding.TransformClosure[K, V]{}, false
	}
	return bound.routeTransform, true
}

// prepareRouteTransformClosure is called exactly once after the prepared
// carrier attaches every Factor Binding to its sealed SlotOwner. Binding's
// TransformClosure intentionally rejects cold, unattached authorities; this
// post-attach cut keeps the immutable route closure Factor-owned without
// rebuilding it for each transformed-carry member.
func (bound *boundFactor[K, V]) prepareRouteTransformClosure() bool {
	if bound == nil || bound.binding == nil {
		return false
	}
	if bound.routeTransformOK {
		return true
	}
	closure, ok := bound.binding.TransformClosure(bound.routeTargets)
	if !ok {
		return false
	}
	bound.routeTransform, bound.routeTransformOK = closure, true
	return true
}

func (bound *boundFactor[K, V]) hasRouteUniverse() bool {
	return bound != nil && bound.implementation != nil && (len(bound.routeTargets) != 0 || bound.implementation.algebra != nil && bound.implementation.algebra.KeyEnd() == 0)
}

func (bound *boundFactor[K, V]) carryRouteScopeFor(member equation.RuleMember) bool {
	return bound != nil && member.Key().Available() && bound.carryRouteScope != nil && bound.carryRouteScope[member.Key()]
}

func (bound *boundFactor[K, V]) stagedSlot() (shape.Slot, bool) {
	return bound.runtimeSlot()
}

// stagedObserve is the typed owner-side bridge from a selected opaque exact
// Unit to Product refinement. The generic engine supplies only the current
// carrier Work, input State, and guard piece; this method keeps root lookup,
// typed observation decoding, and the V payload entirely inside the Factor
// owner. One Begin/End generation surrounds exactly one selected Unit.
func (bound *boundFactor[K, V]) stagedObserve(work *carrier.Work, input carrier.State, unit carrier.Unit, within support.Mask, visit func(factbinding.Observation[V], support.Mask) bool) bool {
	_, ok := bound.stagedObserveWithFailure(work, input, unit, within, visit)
	return ok
}

type stagedObservationFailure uint8

const (
	stagedObservationFailureNone stagedObservationFailure = iota
	stagedObservationFailureArguments
	stagedObservationFailureCheckpoint
	stagedObservationFailureUnit
	stagedObservationFailureSupport
	stagedObservationFailureSlot
	stagedObservationFailureWork
	stagedObservationFailureRoot
	stagedObservationFailureCarrier
	stagedObservationFailureDecode
	stagedObservationFailureVisitor
)

// stagedObserveWithFailure keeps the same typed owner boundary as
// stagedObserve while classifying the first closed read predicate. Optional
// solve-local observations use this classification for failure telemetry;
// ordinary rule and query behavior is unchanged.
func (bound *boundFactor[K, V]) stagedObserveWithFailure(work *carrier.Work, input carrier.State, unit carrier.Unit, within support.Mask, visit func(factbinding.Observation[V], support.Mask) bool) (stagedObservationFailure, bool) {
	if bound == nil || bound.binding == nil || work == nil || visit == nil {
		return stagedObservationFailureArguments, false
	}
	if !work.Checkpoint() {
		return stagedObservationFailureCheckpoint, false
	}
	if unit == (carrier.Unit{}) {
		return stagedObservationFailureUnit, false
	}
	if !within.Valid() {
		return stagedObservationFailureSupport, false
	}
	slot, slotOK := bound.runtimeSlot()
	if !slotOK {
		return stagedObservationFailureSlot, false
	}
	slotWork, workOK := work.SlotWork(slot)
	if !workOK || !slotWork.BeginObservation() {
		return stagedObservationFailureWork, false
	}
	defer slotWork.EndObservation()
	root, rootOK := input.HandleAt(slot)
	if !rootOK {
		return stagedObservationFailureRoot, false
	}
	failure := stagedObservationFailureNone
	completed := slotWork.ObserveUnder(root, unit, within, func(row carrier.ObservationRow) bool {
		if !work.Checkpoint() {
			failure = stagedObservationFailureVisitor
			return false
		}
		observation, resolved := bound.binding.ResolveObservation(slotWork, row)
		if !resolved || !observation.Valid() {
			failure = stagedObservationFailureDecode
			return false
		}
		if !visit(observation, row.Region()) {
			failure = stagedObservationFailureVisitor
			return false
		}
		return true
	})
	if !completed {
		if failure == stagedObservationFailureNone {
			failure = stagedObservationFailureCarrier
		}
		return failure, false
	}
	return stagedObservationFailureNone, true
}

func (bound *boundFactor[K, V]) writeTarget(surface equation.Surface) (carrier.Target, bool) {
	if bound == nil {
		return carrier.Target{}, false
	}
	target, ok := bound.writes[surface]
	return target.target, ok
}

// releaseColdBindings is called only after the sole compiler has attached all
// members and queries. Those runtime objects retain concrete Units, Targets,
// and Selectors; keeping surface maps or target vectors past that cut would
// make an inert graph catalog a second hot-path authority.
func (bound *boundFactor[K, V]) releaseColdBindings() {
	if bound == nil {
		return
	}
	bound.reads = nil
	bound.writes = nil
}

// prepareRuntimeComposition establishes the one carrier composition for this
// graph.  A factor-free structural graph is the same composition with an
// empty root vector; its guard manager still comes from newRuntimeBinding's
// graph catalog and it proceeds through the ordinary Work, contribution, and
// publication protocol.
func prepareRuntimeComposition(factors []runtimeFactor, guards *guard.Manager) (*carrier.PreparedComposition, []runtimeFactor, bool) {
	if guards == nil {
		return nil, nil, false
	}
	if len(factors) == 0 {
		prepared, ok := carrier.PrepareComposition(nil, guards)
		if !ok || prepared == nil || prepared.Guards() != guards || prepared.Shape() == nil || prepared.Shape().Count() != 0 {
			return nil, nil, false
		}
		return prepared, nil, true
	}
	ordered := append([]runtimeFactor(nil), factors...)
	sort.Slice(ordered, func(left, right int) bool {
		return identity.CompareSemanticKey(ordered[left].semantic(), ordered[right].semantic()) < 0
	})
	operations := make([]carrier.FactorOperation, len(ordered))
	for index, factor := range ordered {
		if factor == nil || !factor.semantic().Available() || index > 0 && identity.CompareSemanticKey(ordered[index-1].semantic(), factor.semantic()) >= 0 {
			return nil, nil, false
		}
		operations[index] = factor.operation()
		if operations[index] == nil {
			return nil, nil, false
		}
	}
	prepared, ok := carrier.PrepareComposition(operations, guards)
	if !ok || prepared == nil || prepared.Guards() != guards || prepared.Shape() == nil || prepared.Shape().Count() != len(ordered) {
		return nil, nil, false
	}
	for index, factor := range ordered {
		bound, boundOK := factor.(interface{ bindRuntimeSlot(shape.Slot) bool })
		if !boundOK || !prepared.Shape().ValidSlot(shape.Slot(index)) || !bound.bindRuntimeSlot(shape.Slot(index)) {
			return nil, nil, false
		}
	}
	return prepared, ordered, true
}
