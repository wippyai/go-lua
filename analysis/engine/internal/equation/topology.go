package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

// Graph is the one immutable Point/Group equation topology. Points are the
// only schedule nodes, publishers, and query roots. Groups are private atomic
// RHS hyperedges; they never become schedule nodes or executable objects.
type Graph struct {
	self        *Graph
	composition *composition.Composition
	points      []Point
	pointAt     map[composition.Key]schedule.Node
	groups      []GroupNode
	groupAt     map[composition.Key]int
	memberAt    map[composition.Key]struct{}
	producers   [][]int
	consumers   [][]int
	// Environment edges are structural Point-to-Point transports. They are
	// kept beside, never inside, Group hyperedges so no fake Rule authority is
	// needed for a control-only predecessor.
	environments        []EnvironmentEdgeNode
	environmentIncoming [][]int
	environmentOutgoing [][]int
	environmentGroups   [][]int
	factorEdges         []FactorEdgeNode
	factorIncoming      [][]int
	factorOutgoing      [][]int
	// activationReverses are demand predecessor incidences derived exclusively
	// from sealed activation-binding export ports. They are intentionally
	// absent from schedule edges and producer lists.
	activationReverses [][]schedule.Node
	queries            []Query
	queryAt            map[composition.Key]struct{}
	decisions          []Decision
	schedule           *schedule.Schedule
	eventNodes         []int
	// eventPoints is the sole canonical permutation of Points induced by the
	// immutable WTO event stream. Every recurrence Point membership is an
	// interval in this row; we deliberately do not retain a PointCount bitmap
	// for every Region.
	eventPoints []schedule.Node
	// pointOrder and pointRegion are the inverse event-node index and its
	// innermost Region. They are derived once while the schedule is validated.
	// A Region's ancestors are reached through schedule.Region.Parent.
	pointOrder  []int
	pointRegion []int
	regionNodes []int
	regions     []regionData
	// Region-owned sparse rows are appended once during recurrence derivation.
	// regionData records immutable half-open offsets into these rows.
	regionInterfaces          []interfaceRef
	regionFaces               []int
	regionExternal            []int
	regionBack                []int
	regionInternal            []int
	regionEnvironmentExternal []int
	regionEnvironmentBack     []int
	regionFactorExternal      []int
	regionFactorBack          []int
	regionFactorInternal      []int
	regionFactors             []composition.Key
	// Summary and weak target rows are immutable cold binding metadata. Their
	// offsets address flat CSR payloads; no map, sort, or allocation is needed
	// when a later binder walks the frozen topology.
	summarySurfaces        []Surface
	summaryRepresentatives []Surface
	summaryOffsets         []int
	summaryKeys            []uint64
	weakTargetSurfaces     []Surface
	weakTargetOffsets      []int
	weakTargetCandidates   []Surface
}

// GroupNode is one compiled RHS hyperedge. Its ordered inputs are read from
// one committed Point snapshot; its members contribute atomically to Output.
type GroupNode struct {
	graph            *Graph
	key              composition.Key
	output           Point
	inputs           []Input
	members          []RuleMember
	environmentInput Input
	premise          Expr // issued only by accepted activation expansion
}

// EnvironmentEdgeNode is one graph-owned structural transport row.
type EnvironmentEdgeNode struct {
	graph  *Graph
	key    composition.Key
	target Point
	input  Input
}

// FactorEdgeNode is one graph-owned structural transport that projects the
// source Contribution to exactly one sealed Factor plane at Target.
type FactorEdgeNode struct {
	graph  *Graph
	key    composition.Key
	target Point
	input  Input
	factor composition.Key
}

// RuleMember is a Group-local view of one canonical Rule instance. It is not
// independently scheduled or published.
type RuleMember struct {
	graph         *Graph
	key           composition.Key
	rule          composition.Key
	operandFamily composition.Key
	occurrence    Occurrence
	operand       Operand
	binding       ruleBinding
	activation    Member
}

type ruleBinding struct {
	reads  []Surface
	writes []ruleWrite
}

type ruleWrite struct {
	surface      Surface
	route        uint64
	candidates   []uint64
	targets      []Surface
	dependencies []composition.Dependency
	relations    []CandidateRelation
}

type builtGroup struct {
	key              composition.Key
	output           Point
	inputs           []Input
	members          []RuleMember
	environmentInput Input
	premise          Expr
}

type builtEnvironmentEdge struct {
	key    composition.Key
	target Point
	input  Input
}

type builtActivationReverse struct {
	target  Point
	trigger Point
}

// compileTopology is the one raw topology compiler. Its input is reachable
// only through a sealed Topology selection; direct compilation of a mutable
// builder spelling is deliberately not exposed outside this package.
func compileTopology(source *composition.Composition, topology TopologySpec, activationReverses []derivedActivationReverse) (*Graph, bool) {
	if source == nil || !validTopologyBatch(topology.Batch, topology) || len(topology.Rules) == 0 || len(topology.Points) == 0 || len(topology.Groups) == 0 {
		return nil, false
	}
	catalog, ok := buildTopologyCatalog(topology)
	if !ok || !validateTopologyCatalogUsage(topology, catalog) {
		return nil, false
	}
	instances, ok := buildInstances(source, topology.Batch, topology.Rules, catalog)
	if !ok {
		return nil, false
	}
	declared, sites, points, ok := buildPoints(topology.Points)
	if !ok {
		return nil, false
	}
	groups, ok := buildGroups(source, instances, declared, sites, topology.Groups)
	if !ok {
		return nil, false
	}
	environments, ok := buildEnvironmentEdges(topology.EnvironmentEdges, declared, sites)
	if !ok {
		return nil, false
	}
	factorEdges, ok := buildFactorEdges(source, topology.FactorEdges, declared, sites)
	if !ok {
		return nil, false
	}
	queries, ok := buildQueries(source, declared, topology.Queries, catalog)
	if !ok {
		return nil, false
	}
	reverses, ok := buildActivationReverseIndex(activationReverses, instances, declared, groups)
	if !ok {
		return nil, false
	}
	decisions, ok := collectDecisions(points, groups, environments, factorEdges)
	if !ok {
		return nil, false
	}
	graph, assembled := assembleGraph(source, points, groups, environments, factorEdges, queries, reverses, decisions, catalog)
	return graph, assembled
}

func validTopologyBatch(batch *Batch, topology TopologySpec) bool {
	if !batch.Sealed() {
		return false
	}
	for _, point := range topology.Points {
		if !batch.ownsSite(point.Site) {
			return false
		}
	}
	for _, rule := range topology.Rules {
		if !batch.ownsOccurrence(rule.Occurrence) || !batch.ownsOperand(rule.Operand) || !rule.Operand.Occurrence().Same(rule.Occurrence) {
			return false
		}
	}
	for _, group := range topology.Groups {
		for _, input := range group.Inputs {
			if !batch.ownsSite(input.Source()) || !batch.ownsSite(input.Target()) {
				return false
			}
		}
	}
	for _, edge := range topology.EnvironmentEdges {
		if !batch.ownsSite(edge.Input.Source()) || !batch.ownsSite(edge.Input.Target()) {
			return false
		}
	}
	for _, edge := range topology.FactorEdges {
		if !batch.ownsSite(edge.Input.Source()) || !batch.ownsSite(edge.Input.Target()) || !edge.Factor.Available() {
			return false
		}
	}
	return true
}

func buildActivationReverseIndex(rows []derivedActivationReverse, instances []canonicalInstance, declared map[PointRef]Point, groups []builtGroup) ([]builtActivationReverse, bool) {
	points := make(map[composition.Key]Point, len(declared))
	for _, point := range declared {
		if !point.Available() {
			return nil, false
		}
		if _, duplicate := points[point.key]; duplicate {
			return nil, false
		}
		points[point.key] = point
	}
	instanceAt := make(map[composition.Key]canonicalInstance, len(instances))
	for _, instance := range instances {
		if !instance.key.Available() {
			return nil, false
		}
		if _, duplicate := instanceAt[instance.key]; duplicate {
			return nil, false
		}
		instanceAt[instance.key] = instance
	}
	outputs := make(map[composition.Key]Point, len(instances))
	for _, group := range groups {
		for _, member := range group.members {
			if _, duplicate := outputs[member.key]; duplicate {
				return nil, false
			}
			outputs[member.key] = group.output
		}
	}
	if len(outputs) != len(instances) {
		return nil, false
	}
	result := make([]builtActivationReverse, len(rows))
	seen := make(map[composition.Key]struct{}, len(rows))
	for index, row := range rows {
		if !row.binding.Available() || !row.target.Available() || !row.trigger.Available() {
			return nil, false
		}
		target, targetOK := points[row.target]
		instance, triggerOK := instanceAt[row.trigger]
		if !targetOK || !triggerOK {
			return nil, false
		}
		trigger, found := outputs[instance.key]
		if !found {
			return nil, false
		}
		key, keyOK := identityKey("analysis/engine/equation/activation-reverse", func(writer *canonical.DigestWriter) bool {
			return writeKey(writer, row.binding) && writeKey(writer, target.key) && writeKey(writer, instance.key)
		})
		if !keyOK {
			return nil, false
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		seen[key] = struct{}{}
		result[index] = builtActivationReverse{target: target, trigger: trigger}
	}
	// Binding coverage is proved when Topology seals the exact trigger. The
	// demand scheduler sees only Points, so multiple bindings may induce the
	// same target-to-trigger incidence. Canonicalize that derived view only
	// after every export-port reverse has been validated.
	sort.Slice(result, func(left, right int) bool {
		if result[left].target.key != result[right].target.key {
			return lessKey(result[left].target.key, result[right].target.key)
		}
		return lessKey(result[left].trigger.key, result[right].trigger.key)
	})
	retained := 0
	for _, follower := range result {
		if retained != 0 && result[retained-1].target.key == follower.target.key && result[retained-1].trigger.key == follower.trigger.key {
			continue
		}
		result[retained] = follower
		retained++
	}
	return result[:retained], true
}

// buildPoints gives every builder reference its issued Point and indexes the
// one exact sealed Site that owns its scope and initialization disposition.
func buildPoints(rows []PointSpec) (map[PointRef]Point, map[composition.Key]Point, []Point, bool) {
	declared := make(map[PointRef]Point, len(rows))
	bySite := make(map[composition.Key]Point, len(rows))
	points := make([]Point, len(rows))
	for index, row := range rows {
		ref := PointAt(index)
		if !row.Site.Available() {
			return nil, nil, nil, false
		}
		point, ok := derivePoint(row.Site)
		if !ok {
			return nil, nil, nil, false
		}
		if _, duplicate := bySite[row.Site.Key()]; duplicate {
			return nil, nil, nil, false
		}
		bySite[row.Site.Key()] = point
		declared[ref], points[index] = point, point
	}
	sort.Slice(points, func(i, j int) bool { return lessKey(points[i].key, points[j].key) })
	return declared, bySite, points, true
}

func buildInstances(source *composition.Composition, batch *Batch, rows []RuleInstance, catalog topologyCatalog) ([]canonicalInstance, bool) {
	result := make([]canonicalInstance, len(rows))
	seen := make(map[composition.Key]struct{}, len(rows))
	for index, row := range rows {
		schema, ok := ruleSchema(source, row.Schema)
		if !ok || !batch.ownsOccurrence(row.Occurrence) || !batch.ownsOperand(row.Operand) || !row.Schema.Available() || !row.OperandFamily.Available() || !row.Operand.Occurrence().Same(row.Occurrence) || !validateResolvedInstance(row, schema) {
			return nil, false
		}
		key, ok := deriveInstanceKey(row, catalog)
		if !ok {
			return nil, false
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		seen[key] = struct{}{}
		result[index] = canonicalInstance{key: key, row: copyInstance(row)}
	}
	return result, len(result) != 0
}

func buildGroups(source *composition.Composition, instances []canonicalInstance, declared map[PointRef]Point, sites map[composition.Key]Point, rows []Group) ([]builtGroup, bool) {
	used := make([]bool, len(instances))
	groups := make([]builtGroup, len(rows))
	seen := make(map[composition.Key]struct{}, len(rows))
	for index, row := range rows {
		output, ok := declared[row.Output]
		if !ok || !output.Available() || len(row.Members) == 0 {
			return nil, false
		}
		members, ok := resolveGroupMembers(source, instances, row, used)
		if !ok {
			return nil, false
		}
		inputs, ok := resolveInputs(row.Inputs, sites, output)
		if !ok {
			return nil, false
		}
		core, ok := deriveGroupCore(members)
		if !ok {
			return nil, false
		}
		premise := row.premise
		if !premise.Available() {
			premise = TrueExpr()
		}
		for _, decision := range premise.Decisions() {
			if !output.Scope().contains(decision) {
				return nil, false
			}
		}
		environmentInput := row.EnvironmentInput
		if environmentInput.Available() {
			if environmentInput.point.Available() || !environmentInput.Target().Same(output.Site()) || !sameScope(environmentInput.Reindex().Target(), output.Scope()) {
				return nil, false
			}
			environmentSource, sourceOK := sites[environmentInput.Source().Key()]
			if !sourceOK || !environmentSource.Available() || !environmentSource.Site().Same(environmentInput.Source()) || !sameScope(environmentSource.Scope(), environmentInput.Reindex().Source()) {
				return nil, false
			}
			environmentInput.point = environmentSource
		}
		key, ok := derivePremisedGroupKey(core, output, inputs, premise)
		if environmentInput.Available() {
			key, ok = derivePremisedGroupKeyWithEnvironment(core, output, inputs, premise, environmentInput)
		}
		if !ok {
			return nil, false
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		seen[key] = struct{}{}
		compiled := make([]RuleMember, len(members))
		for memberIndex, member := range members {
			schema, ok := ruleSchema(source, member.row.Schema)
			if !ok {
				return nil, false
			}
			binding, ok := deriveRuleBinding(member.row, schema)
			if !ok {
				return nil, false
			}
			compiled[memberIndex] = RuleMember{key: member.key, rule: member.row.Schema, operandFamily: member.row.OperandFamily, occurrence: member.row.Occurrence, operand: member.row.Operand, binding: binding, activation: member.row.activation}
		}
		groups[index] = builtGroup{key: key, output: output, inputs: inputs, members: compiled, environmentInput: environmentInput, premise: premise}
	}
	for _, memberUsed := range used {
		if !memberUsed {
			return nil, false
		}
	}
	sort.Slice(groups, func(i, j int) bool { return lessKey(groups[i].key, groups[j].key) })
	return groups, true
}

func buildEnvironmentEdges(rows []EnvironmentEdge, declared map[PointRef]Point, sites map[composition.Key]Point) ([]builtEnvironmentEdge, bool) {
	result := make([]builtEnvironmentEdge, len(rows))
	seen := make(map[composition.Key]struct{}, len(rows))
	for index, row := range rows {
		target, ok := declared[row.Target]
		if !ok || !target.Available() || !row.Input.Available() || row.Input.point.Available() || !row.Input.Target().Same(target.Site()) {
			return nil, false
		}
		source, ok := sites[row.Input.Source().Key()]
		if !ok || !source.Available() || !source.Site().Same(row.Input.Source()) || !sameScope(source.Scope(), row.Input.Reindex().Source()) {
			return nil, false
		}
		if !sameScope(target.Scope(), row.Input.Reindex().Target()) {
			return nil, false
		}
		input := row.Input
		input.point = source
		key, keyOK := identityKey("analysis/engine/equation/environment-edge", func(writer *canonical.DigestWriter) bool {
			return writeKey(writer, input.Key()) && writePoint(writer, target)
		})
		if !keyOK || !key.Available() {
			return nil, false
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		seen[key] = struct{}{}
		result[index] = builtEnvironmentEdge{key: key, target: target, input: input}
	}
	sort.Slice(result, func(left, right int) bool { return lessKey(result[left].key, result[right].key) })
	return result, true
}

type builtFactorEdge struct {
	key    composition.Key
	target Point
	input  Input
	factor composition.Key
}

func buildFactorEdges(source *composition.Composition, rows []FactorEdge, declared map[PointRef]Point, sites map[composition.Key]Point) ([]builtFactorEdge, bool) {
	if source == nil {
		return nil, false
	}
	result := make([]builtFactorEdge, len(rows))
	seen := make(map[composition.Key]struct{}, len(rows))
	for index, row := range rows {
		target, ok := declared[row.Target]
		if !ok || !target.Available() || !row.Input.Available() || row.Input.point.Available() || !row.Input.Target().Same(target.Site()) || !row.Factor.Available() {
			return nil, false
		}
		if _, known := source.FactorIndex(row.Factor); !known {
			return nil, false
		}
		sourcePoint, ok := sites[row.Input.Source().Key()]
		if !ok || !sourcePoint.Available() || !sourcePoint.Site().Same(row.Input.Source()) || !sameScope(sourcePoint.Scope(), row.Input.Reindex().Source()) || !sameScope(target.Scope(), row.Input.Reindex().Target()) {
			return nil, false
		}
		input := row.Input
		input.point = sourcePoint
		key, keyOK := identityKey("analysis/engine/equation/factor-edge", func(writer *canonical.DigestWriter) bool {
			return writeKey(writer, input.Key()) && writePoint(writer, target) && writeKey(writer, row.Factor)
		})
		if !keyOK || !key.Available() {
			return nil, false
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		seen[key] = struct{}{}
		result[index] = builtFactorEdge{key: key, target: target, input: input, factor: row.Factor}
	}
	sort.Slice(result, func(left, right int) bool { return lessKey(result[left].key, result[right].key) })
	return result, true
}

func resolveGroupMembers(source *composition.Composition, instances []canonicalInstance, row Group, used []bool) ([]canonicalInstance, bool) {
	members := make([]canonicalInstance, len(row.Members))
	writers := make(map[composition.Key]struct{}, len(row.Members))
	for index, ref := range row.Members {
		instanceIndex, ok := ruleIndex(ref, len(instances))
		if !ok || used[instanceIndex] {
			return nil, false
		}
		member := instances[instanceIndex]
		schema, ok := ruleSchema(source, member.row.Schema)
		if !ok || schema.Inputs != uint64(len(row.Inputs)) {
			return nil, false
		}
		if schema.Output.Available() {
			if _, duplicate := writers[schema.Output]; duplicate {
				return nil, false
			}
			writers[schema.Output] = struct{}{}
		}
		used[instanceIndex] = true
		members[index] = member
	}
	sort.Slice(members, func(i, j int) bool { return lessKey(members[i].key, members[j].key) })
	return members, true
}

func resolveInputs(rows []Input, sites map[composition.Key]Point, target Point) ([]Input, bool) {
	if !target.Available() || !target.Site().Available() {
		return nil, false
	}
	result := make([]Input, len(rows))
	for index, input := range rows {
		if !input.Available() || input.point.Available() || !input.target.Same(target.Site()) || !sameScope(input.omega.Target(), target.Scope()) {
			return nil, false
		}
		point, ok := sites[input.source.Key()]
		if !ok || !point.Available() || !point.Site().Same(input.source) || !sameScope(point.Scope(), input.omega.Source()) {
			return nil, false
		}
		input.point = point
		result[index] = input
	}
	return result, true
}

func buildQueries(source *composition.Composition, declared map[PointRef]Point, rows []QueryInstance, catalog topologyCatalog) ([]Query, bool) {
	families := source.Queries()
	if len(rows) < len(families) || len(rows) == 0 {
		return nil, false
	}
	queries := make([]Query, len(rows))
	seen := make([]bool, len(families))
	for index, row := range rows {
		point, ok := declared[row.Point]
		if !ok || !validQueryInstance(source, row) {
			return nil, false
		}
		familyIndex, indexed := source.QueryIndex(row.Family)
		if !indexed || familyIndex >= uint64(len(families)) || families[familyIndex].Key != row.Family {
			return nil, false
		}
		key, ok := deriveQueryKey(row, point, catalog)
		if !ok {
			return nil, false
		}
		seen[familyIndex] = true
		queries[index] = Query{key: key, point: point, family: row.Family, surfaces: append([]Surface(nil), row.Surfaces...)}
	}
	for _, present := range seen {
		if !present {
			return nil, false
		}
	}
	sort.Slice(queries, func(i, j int) bool { return lessKey(queries[i].key, queries[j].key) })
	return queries, true
}

func assembleGraph(source *composition.Composition, points []Point, built []builtGroup, environments []builtEnvironmentEdge, factorEdges []builtFactorEdge, queries []Query, reverses []builtActivationReverse, decisions []Decision, catalog topologyCatalog) (*Graph, bool) {
	graph := &Graph{composition: source, points: append([]Point(nil), points...), pointAt: make(map[composition.Key]schedule.Node, len(points)), groups: make([]GroupNode, len(built)), groupAt: make(map[composition.Key]int, len(built)), memberAt: make(map[composition.Key]struct{}), producers: make([][]int, len(points)), consumers: make([][]int, len(points)), environments: make([]EnvironmentEdgeNode, len(environments)), environmentIncoming: make([][]int, len(points)), environmentOutgoing: make([][]int, len(points)), environmentGroups: make([][]int, len(points)), factorEdges: make([]FactorEdgeNode, len(factorEdges)), factorIncoming: make([][]int, len(points)), factorOutgoing: make([][]int, len(points)), activationReverses: make([][]schedule.Node, len(points)), queries: append([]Query(nil), queries...), queryAt: make(map[composition.Key]struct{}, len(queries)), decisions: append([]Decision(nil), decisions...)}
	graph.self = graph
	if !graph.installCatalog(catalog) {
		return nil, false
	}
	for index, point := range graph.points {
		if _, duplicate := graph.pointAt[point.key]; duplicate {
			return nil, false
		}
		graph.points[index].graph = graph
		graph.pointAt[point.key] = schedule.Node(index)
	}
	for index, query := range graph.queries {
		node, ok := graph.pointAt[query.point.key]
		if !ok {
			return nil, false
		}
		graph.queries[index].point = graph.points[node]
		graph.queries[index].graph = graph
		if _, duplicate := graph.queryAt[query.key]; duplicate {
			return nil, false
		}
		graph.queryAt[query.key] = struct{}{}
	}
	edges := make(map[schedule.Edge]struct{})
	for groupIndex, row := range built {
		output, ok := graph.pointAt[row.output.key]
		if !ok {
			return nil, false
		}
		inputs := append([]Input(nil), row.inputs...)
		for inputIndex := range inputs {
			node, ok := graph.pointAt[inputs[inputIndex].point.key]
			if !ok {
				return nil, false
			}
			inputs[inputIndex].point = graph.points[node]
		}
		members := cloneMembers(row.members)
		for memberIndex := range members {
			members[memberIndex].graph = graph
			if _, duplicate := graph.memberAt[members[memberIndex].key]; duplicate {
				return nil, false
			}
			graph.memberAt[members[memberIndex].key] = struct{}{}
		}
		environmentInput := row.environmentInput
		if environmentInput.Available() {
			source, ok := graph.pointAt[environmentInput.point.key]
			if !ok {
				return nil, false
			}
			environmentInput.point = graph.points[source]
		}
		graph.groups[groupIndex] = GroupNode{graph: graph, key: row.key, output: graph.points[output], inputs: inputs, members: members, environmentInput: environmentInput, premise: row.premise}
		if _, duplicate := graph.groupAt[row.key]; duplicate {
			return nil, false
		}
		graph.groupAt[row.key] = groupIndex
		graph.producers[output] = append(graph.producers[output], groupIndex)
		for _, input := range row.inputs {
			from, ok := graph.pointAt[input.point.key]
			if !ok {
				return nil, false
			}
			graph.consumers[from] = append(graph.consumers[from], groupIndex)
			edges[schedule.Edge{From: from, To: output}] = struct{}{}
		}
		if row.environmentInput.Available() {
			environmentSource, ok := graph.pointAt[row.environmentInput.point.key]
			if !ok {
				return nil, false
			}
			graph.environmentGroups[environmentSource] = append(graph.environmentGroups[environmentSource], groupIndex)
			edges[schedule.Edge{From: environmentSource, To: output}] = struct{}{}
		}
	}
	for edgeIndex, row := range environments {
		target, targetOK := graph.pointAt[row.target.key]
		sourcePoint, sourceOK := graph.pointAt[row.input.point.key]
		if !targetOK || !sourceOK || row.input.Target().Key() != row.target.Site().Key() {
			return nil, false
		}
		input := row.input
		input.point = graph.points[sourcePoint]
		graph.environments[edgeIndex] = EnvironmentEdgeNode{graph: graph, key: row.key, target: graph.points[target], input: input}
		graph.environmentIncoming[target] = append(graph.environmentIncoming[target], edgeIndex)
		graph.environmentOutgoing[sourcePoint] = append(graph.environmentOutgoing[sourcePoint], edgeIndex)
		edges[schedule.Edge{From: sourcePoint, To: target}] = struct{}{}
	}
	for edgeIndex, row := range factorEdges {
		target, targetOK := graph.pointAt[row.target.key]
		sourcePoint, sourceOK := graph.pointAt[row.input.point.key]
		if !targetOK || !sourceOK || row.input.Target().Key() != row.target.Site().Key() || !row.factor.Available() {
			return nil, false
		}
		input := row.input
		input.point = graph.points[sourcePoint]
		graph.factorEdges[edgeIndex] = FactorEdgeNode{graph: graph, key: row.key, target: graph.points[target], input: input, factor: row.factor}
		graph.factorIncoming[target] = append(graph.factorIncoming[target], edgeIndex)
		graph.factorOutgoing[sourcePoint] = append(graph.factorOutgoing[sourcePoint], edgeIndex)
		edges[schedule.Edge{From: sourcePoint, To: target}] = struct{}{}
	}
	for point := range graph.producers {
		sort.Ints(graph.producers[point])
		sort.Ints(graph.consumers[point])
		sort.Ints(graph.environmentIncoming[point])
		sort.Ints(graph.environmentOutgoing[point])
		sort.Ints(graph.environmentGroups[point])
		sort.Ints(graph.factorIncoming[point])
		sort.Ints(graph.factorOutgoing[point])
	}
	for _, reverse := range reverses {
		target, targetOK := graph.pointAt[reverse.target.key]
		trigger, triggerOK := graph.pointAt[reverse.trigger.key]
		if !targetOK || !triggerOK {
			return nil, false
		}
		graph.activationReverses[target] = append(graph.activationReverses[target], trigger)
	}
	for index := range graph.activationReverses {
		row := graph.activationReverses[index]
		sort.Slice(row, func(left, right int) bool { return row[left] < row[right] })
		for cursor := 1; cursor < len(row); cursor++ {
			if row[cursor] == row[cursor-1] {
				return nil, false
			}
		}
	}
	ordered := make([]schedule.Edge, 0, len(edges))
	for edge := range edges {
		ordered = append(ordered, edge)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].From != ordered[j].From {
			return ordered[i].From < ordered[j].From
		}
		return ordered[i].To < ordered[j].To
	})
	prepared, err := schedule.Prepare(len(graph.points), ordered)
	if err != nil || prepared == nil {
		return nil, false
	}
	graph.schedule = prepared
	graph.eventNodes = make([]int, prepared.EventCount()+1)
	graph.eventPoints = make([]schedule.Node, 0, len(graph.points))
	graph.pointOrder = make([]int, len(graph.points))
	graph.pointRegion = make([]int, len(graph.points))
	for index := range graph.pointOrder {
		graph.pointOrder[index] = -1
		graph.pointRegion[index] = schedule.NoRegion
	}
	for index := 0; index < prepared.EventCount(); index++ {
		event, ok := prepared.EventAt(index)
		if !ok {
			return nil, false
		}
		graph.eventNodes[index+1] = graph.eventNodes[index]
		if event.Kind == schedule.EventNode {
			if event.Node < 0 || int(event.Node) >= len(graph.points) || graph.pointOrder[event.Node] != -1 || event.Region < schedule.NoRegion || event.Region >= prepared.RegionCount() {
				return nil, false
			}
			graph.pointOrder[event.Node] = len(graph.eventPoints)
			graph.pointRegion[event.Node] = event.Region
			graph.eventPoints = append(graph.eventPoints, event.Node)
			graph.eventNodes[index+1]++
		}
	}
	if len(graph.eventPoints) != len(graph.points) {
		return nil, false
	}
	for _, order := range graph.pointOrder {
		if order < 0 || order >= len(graph.eventPoints) {
			return nil, false
		}
	}
	graph.regionNodes = make([]int, prepared.RegionCount())
	for index := range graph.regionNodes {
		region, ok := prepared.RegionAt(index)
		if !ok || region.Enter < 0 || region.Exit < region.Enter || region.Exit >= prepared.EventCount() {
			return nil, false
		}
		graph.regionNodes[index] = graph.eventNodes[region.Exit+1] - graph.eventNodes[region.Enter]
		if graph.regionNodes[index] == 0 {
			return nil, false
		}
	}
	if !graph.deriveRegions() {
		return nil, false
	}
	return graph, true
}

func collectDecisions(points []Point, groups []builtGroup, environments []builtEnvironmentEdge, factorEdges []builtFactorEdge) ([]Decision, bool) {
	set := make(map[composition.Key]Decision)
	addScope := func(scope Scope) bool {
		if !scope.Available() {
			return false
		}
		for _, decision := range scope.row.decisions {
			if !decision.Available() {
				return false
			}
			set[decision.key] = decision
		}
		return true
	}
	addExpr := func(expr Expr) bool {
		if !expr.Available() {
			return false
		}
		for _, decision := range expr.Decisions() {
			if !decision.Available() {
				return false
			}
			set[decision.key] = decision
		}
		return true
	}
	for _, point := range points {
		if !addScope(point.Scope()) {
			return nil, false
		}
	}
	for _, group := range groups {
		if !addExpr(group.premise) {
			return nil, false
		}
		for _, input := range group.inputs {
			if !addScope(input.omega.Source()) || !addScope(input.omega.Target()) || !addExpr(input.pre) || !addExpr(input.post) {
				return nil, false
			}
		}
		if group.environmentInput.Available() {
			input := group.environmentInput
			if !addScope(input.omega.Source()) || !addScope(input.omega.Target()) || !addExpr(input.pre) || !addExpr(input.post) {
				return nil, false
			}
		}
	}
	for _, edge := range environments {
		input := edge.input
		if !addScope(input.omega.Source()) || !addScope(input.omega.Target()) || !addExpr(input.pre) || !addExpr(input.post) {
			return nil, false
		}
	}
	for _, edge := range factorEdges {
		input := edge.input
		if !addScope(input.omega.Source()) || !addScope(input.omega.Target()) || !addExpr(input.pre) || !addExpr(input.post) {
			return nil, false
		}
	}
	result := make([]Decision, 0, len(set))
	for _, decision := range set {
		result = append(result, decision)
	}
	sort.Slice(result, func(i, j int) bool { return lessKey(result[i].key, result[j].key) })
	return result, true
}

func deriveRuleBinding(row RuleInstance, schema composition.Rule) (ruleBinding, bool) {
	if !validateResolvedInstance(row, schema) {
		return ruleBinding{}, false
	}
	binding := ruleBinding{reads: make([]Surface, len(row.Reads)), writes: make([]ruleWrite, len(row.Writes))}
	for index, read := range row.Reads {
		binding.reads[index] = read.Surface
	}
	for index, write := range row.Writes {
		binding.writes[index] = ruleWrite{surface: write.Surface, route: write.Route, candidates: append([]uint64(nil), write.Candidates...), targets: append([]Surface(nil), write.TargetCandidates...), dependencies: append([]composition.Dependency(nil), schema.Writes[index].Dependencies...), relations: cloneCandidateRelations(write.Relations)}
	}
	return binding, binding.valid(schema)
}

func (binding ruleBinding) valid(schema composition.Rule) bool {
	if len(binding.reads) != len(schema.Reads) || len(binding.writes) != len(schema.Writes) {
		return false
	}
	for index, surface := range binding.reads {
		if !matchesReadSurface(surface, schema.Reads[index]) {
			return false
		}
	}
	for index, write := range binding.writes {
		resolved := ResolvedWrite{Index: uint64(index), Surface: write.surface, Route: write.route, Candidates: write.candidates, TargetCandidates: write.targets, Relations: write.relations}
		if !matchesWriteSurface(write.surface, schema.Writes[index]) || !sameDependencies(write.dependencies, schema.Writes[index].Dependencies) || !validResolvedWriteSelection(resolved, schema.Writes[index], schema.Writes, index) {
			return false
		}
	}
	return true
}

func (binding ruleBinding) clone() ruleBinding {
	result := ruleBinding{reads: append([]Surface(nil), binding.reads...), writes: make([]ruleWrite, len(binding.writes))}
	for index, write := range binding.writes {
		result.writes[index] = ruleWrite{surface: write.surface, route: write.route, candidates: append([]uint64(nil), write.candidates...), targets: append([]Surface(nil), write.targets...), dependencies: append([]composition.Dependency(nil), write.dependencies...), relations: cloneCandidateRelations(write.relations)}
	}
	return result
}

func cloneMembers(rows []RuleMember) []RuleMember {
	result := make([]RuleMember, len(rows))
	for index, row := range rows {
		result[index] = RuleMember{key: row.key, rule: row.rule, operandFamily: row.operandFamily, occurrence: row.occurrence, operand: row.operand, binding: row.binding.clone(), activation: row.activation}
	}
	return result
}

func validQueryInstance(source *composition.Composition, query QueryInstance) bool {
	if !query.Family.Available() {
		return false
	}
	index, ok := source.QueryIndex(query.Family)
	if !ok {
		return false
	}
	families := source.Queries()
	if index >= uint64(len(families)) {
		return false
	}
	family := families[index]
	if len(family.Projections) == 1 && family.Projections[0].Kind == composition.QuerySupport {
		return len(query.Surfaces) == 0
	}
	if len(query.Surfaces) != len(family.Projections) || len(query.Surfaces) == 0 {
		return false
	}
	for index, projection := range family.Projections {
		surface := query.Surfaces[index]
		switch projection.Kind {
		case composition.QueryFactorExact:
			if !matchesReadSurface(surface, composition.Read{Kind: composition.ReadExact, Factor: projection.Factor}) {
				return false
			}
		case composition.QueryFactorSummary:
			if !matchesReadSurface(surface, composition.Read{Kind: composition.ReadSummary, Factor: projection.Factor, Semantic: projection.Normalizer, Normalizer: projection.Normalizer}) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func validateResolvedInstance(row RuleInstance, schema composition.Rule) bool {
	if !row.Schema.Available() || !row.OperandFamily.Available() || !row.Operand.Available() || row.OperandFamily != schema.OperandFamily || len(schema.Activations) > 1 {
		return false
	}
	return validateReads(row.Reads, schema.Reads) && validateCarries(row.Carries, schema.Carries) && validateWrites(row.Writes, schema.Writes) && validateSupports(row.Supports, schema.Supports) && validatePrunes(row.Prunes, schema.Prunes)
}
func validateReads(values []ResolvedRead, schemas []composition.Read) bool {
	if len(values) != len(schemas) {
		return false
	}
	for index, value := range values {
		if value.Index != uint64(index) || !matchesReadSurface(value.Surface, schemas[index]) {
			return false
		}
	}
	return true
}
func validateCarries(values []ResolvedCarry, schemas []composition.Carry) bool {
	if len(values) != len(schemas) {
		return false
	}
	for index, value := range values {
		if value.Index != uint64(index) {
			return false
		}
	}
	return true
}
func validateWrites(values []ResolvedWrite, schemas []composition.Write) bool {
	if len(values) != len(schemas) {
		return false
	}
	for index, value := range values {
		if value.Index != uint64(index) || !matchesWriteSurface(value.Surface, schemas[index]) || !validResolvedWriteSelection(value, schemas[index], schemas, index) {
			return false
		}
	}
	return true
}

func validResolvedWriteSelection(value ResolvedWrite, schema composition.Write, writes []composition.Write, index int) bool {
	switch schema.Kind {
	case composition.WriteExact:
		return value.Route == 0 && len(value.Candidates) == 0 && len(value.TargetCandidates) == 0 && len(value.Relations) == 0
	case composition.WriteSelect:
		if value.Route != 0 || !sameOrdinals(value.Candidates, schema.Candidates) || len(value.TargetCandidates) != len(value.Candidates) || !validTargetCandidates(value.TargetCandidates, schema.Factor) {
			return false
		}
		targets := make([]uint64, 0, len(schema.Dependencies))
		for _, dependency := range schema.Dependencies {
			if dependency.Target {
				targets = append(targets, dependency.Index)
			}
		}
		if len(value.Relations) != len(targets) {
			return false
		}
		for relationIndex, relation := range value.Relations {
			if relation.Prior != targets[relationIndex] || relation.Prior >= uint64(index) || len(relation.Matches) != len(value.Candidates) {
				return false
			}
			count, ok := writeCandidateCount(writes[relation.Prior])
			if !ok {
				return false
			}
			for _, matches := range relation.Matches {
				if !validOrdinals(matches, count) {
					return false
				}
			}
		}
		return true
	case composition.WriteRoute:
		// Schema validation already proves Route is a one-based ReadSelect
		// ordinal. It is not a write ordinal, so its range is unrelated to
		// len(writes) and may validly exceed it.
		return value.Route == schema.Route && value.Route != 0 && len(value.Candidates) == 0 && len(value.TargetCandidates) == 0 && len(value.Relations) == 0
	default:
		return false
	}
}

// TargetCandidates are selector-position data, not a set. Repetition and a
// non-monotone order are meaningful: candidate i is paired with cold read
// ordinal Candidates[i]. Each target itself remains an exact strong/weak
// capability owned by the output Factor.
func validTargetCandidates(values []Surface, factor composition.Key) bool {
	for _, value := range values {
		if !value.Available() || value.Factor != factor || value.Form != SurfaceWriteExact ||
			(value.Mode != TargetModeStrong && value.Mode != TargetModeWeak) || value.Semantic.Available() || value.Normalizer.Available() {
			return false
		}
	}
	return true
}
func writeCandidateCount(write composition.Write) (uint64, bool) {
	switch write.Kind {
	case composition.WriteExact:
		return 1, true
	case composition.WriteSelect:
		if len(write.Candidates) == 0 {
			return 0, false
		}
		return uint64(len(write.Candidates)), true
	default:
		return 0, false
	}
}
func validOrdinals(values []uint64, limit uint64) bool {
	for index, value := range values {
		if value >= limit || index > 0 && values[index-1] >= value {
			return false
		}
	}
	return true
}
func sameOrdinals(left, right []uint64) bool {
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
func validateSupports(values []ResolvedSupport, schemas []composition.Support) bool {
	if len(values) != len(schemas) {
		return false
	}
	for index, value := range values {
		if value.Index != uint64(index) || !matchesSupportSurface(value.Surface, schemas[index]) {
			return false
		}
	}
	return true
}
func validatePrunes(values []ResolvedPrune, schemas []composition.Prune) bool {
	if len(values) != len(schemas) {
		return false
	}
	for index, value := range values {
		if value.Index != uint64(index) || !matchesPruneSurface(value.Surface, schemas[index]) {
			return false
		}
	}
	return true
}
func matchesReadSurface(surface Surface, schema composition.Read) bool {
	if !surface.Available() || surface.Factor != schema.Factor {
		return false
	}
	switch schema.Kind {
	case composition.ReadExact:
		return surface.Form == SurfaceReadExact && surface.Mode == TargetModeNone && !surface.Semantic.Available() && !surface.Normalizer.Available()
	case composition.ReadSummary:
		return surface.Form == SurfaceReadSummary && surface.Mode == TargetModeNone && surface.Semantic == schema.Semantic && surface.Normalizer == schema.Normalizer
	case composition.ReadSelect:
		return surface.Form == SurfaceReadSelect && surface.Mode == TargetModeNone && surface.Semantic == schema.Semantic && surface.Normalizer == schema.Normalizer
	default:
		return false
	}
}
func matchesWriteSurface(surface Surface, schema composition.Write) bool {
	if !surface.Available() || surface.Factor != schema.Factor {
		return false
	}
	switch schema.Kind {
	case composition.WriteExact:
		return surface.Form == SurfaceWriteExact && (surface.Mode == TargetModeStrong || surface.Mode == TargetModeWeak) && !surface.Semantic.Available() && !surface.Normalizer.Available()
	case composition.WriteSelect:
		return surface.Form == SurfaceWriteSelect && surface.Mode == TargetModeNone && surface.Semantic == schema.Semantic && !surface.Normalizer.Available()
	case composition.WriteRoute:
		return surface.Form == SurfaceWriteRoute && surface.Mode == TargetModeNone && !surface.Semantic.Available() && !surface.Normalizer.Available()
	default:
		return false
	}
}
func matchesSupportSurface(surface StructuralSurface, schema composition.Support) bool {
	return surface.Available() && surface.Semantic == schema.Semantic
}
func matchesPruneSurface(surface StructuralSurface, schema composition.Prune) bool {
	return surface.Available() && surface.Semantic == schema.Semantic
}
func sameDependencies(left, right []composition.Dependency) bool {
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
func copyInstance(row RuleInstance) RuleInstance {
	result := row
	result.Reads = append([]ResolvedRead(nil), row.Reads...)
	result.Carries = append([]ResolvedCarry(nil), row.Carries...)
	result.Writes = append([]ResolvedWrite(nil), row.Writes...)
	for index := range result.Writes {
		result.Writes[index].Candidates = append([]uint64(nil), row.Writes[index].Candidates...)
		result.Writes[index].TargetCandidates = append([]Surface(nil), row.Writes[index].TargetCandidates...)
		result.Writes[index].Relations = cloneCandidateRelations(row.Writes[index].Relations)
	}
	result.Supports = append([]ResolvedSupport(nil), row.Supports...)
	result.Prunes = append([]ResolvedPrune(nil), row.Prunes...)
	return result
}
func cloneCandidateRelations(values []CandidateRelation) []CandidateRelation {
	result := make([]CandidateRelation, len(values))
	for index, relation := range values {
		result[index].Prior = relation.Prior
		result[index].Matches = make([][]uint64, len(relation.Matches))
		for current, matches := range relation.Matches {
			result[index].Matches[current] = append([]uint64(nil), matches...)
		}
	}
	return result
}
func ruleSchema(source *composition.Composition, key composition.Key) (composition.Rule, bool) {
	index, ok := source.RuleIndex(key)
	if !ok {
		return composition.Rule{}, false
	}
	return source.RuleAt(index)
}
func ruleIndex(ref RuleRef, count int) (int, bool) {
	if ref == 0 || uint64(ref) > uint64(count) {
		return 0, false
	}
	return int(uint64(ref) - 1), true
}

func (graph *Graph) valid() bool {
	return graph != nil && graph.self == graph && graph.composition != nil && graph.schedule != nil
}

// CompositionID identifies the sealed cold schema from which this immutable
// topology was compiled. It lets the typed runtime binder reject an equally
// named Factor from another Composition without exposing the Composition or
// creating a second topology authority.
func (graph *Graph) CompositionID() composition.ID {
	if !graph.valid() {
		return composition.ID{}
	}
	return graph.composition.ID()
}

// OwnsComposition proves pointer identity with the one sealed source used to
// compile this Graph. Equal content/digests from a distinct Composition are
// intentionally not interchangeable runtime authority.
func (graph *Graph) OwnsComposition(source *composition.Composition) bool {
	return graph != nil && graph.valid() && source != nil && graph.composition == source
}

func (graph *Graph) Schedule() *schedule.Schedule {
	if !graph.valid() {
		return nil
	}
	return graph.schedule
}
func (graph *Graph) PointCount() int {
	if !graph.valid() {
		return 0
	}
	return len(graph.points)
}
func (graph *Graph) PointAt(node schedule.Node) (Point, bool) {
	if !graph.valid() || node < 0 || int(node) >= len(graph.points) {
		return Point{}, false
	}
	return graph.points[node], true
}
func (graph *Graph) GroupCount() int {
	if !graph.valid() {
		return 0
	}
	return len(graph.groups)
}
func (graph *Graph) HyperedgeAt(index int) (GroupNode, bool) {
	if !graph.valid() || index < 0 || index >= len(graph.groups) {
		return GroupNode{}, false
	}
	return graph.groups[index], true
}
func (graph *Graph) ProducerCount(point Point) int {
	if !graph.valid() {
		return 0
	}
	node, ok := graph.pointAt[point.key]
	if !ok {
		return 0
	}
	return len(graph.producers[node])
}
func (graph *Graph) ProducerAt(point Point, index int) (GroupNode, bool) {
	if !graph.valid() {
		return GroupNode{}, false
	}
	node, ok := graph.pointAt[point.key]
	if !ok || index < 0 || index >= len(graph.producers[node]) {
		return GroupNode{}, false
	}
	return graph.groups[graph.producers[node][index]], true
}
func (graph *Graph) ConsumerCount(point Point) int {
	if !graph.valid() {
		return 0
	}
	node, ok := graph.pointAt[point.key]
	if !ok {
		return 0
	}
	return len(graph.consumers[node])
}
func (graph *Graph) ConsumerAt(point Point, index int) (GroupNode, bool) {
	if !graph.valid() {
		return GroupNode{}, false
	}
	node, ok := graph.pointAt[point.key]
	if !ok || index < 0 || index >= len(graph.consumers[node]) {
		return GroupNode{}, false
	}
	return graph.groups[graph.consumers[node][index]], true
}

// EnvironmentEdgeCount reports structural control transports entering point.
func (graph *Graph) EnvironmentEdgeCount(point Point) int {
	if !graph.valid() {
		return 0
	}
	node, ok := graph.pointAt[point.key]
	if !ok {
		return 0
	}
	return len(graph.environmentIncoming[node])
}

func (graph *Graph) EnvironmentEdgeAt(point Point, index int) (EnvironmentEdgeNode, bool) {
	if !graph.valid() {
		return EnvironmentEdgeNode{}, false
	}
	node, ok := graph.pointAt[point.key]
	if !ok || index < 0 || index >= len(graph.environmentIncoming[node]) {
		return EnvironmentEdgeNode{}, false
	}
	return graph.environments[graph.environmentIncoming[node][index]], true
}

func (graph *Graph) EnvironmentEdgeAtIndex(index int) (EnvironmentEdgeNode, bool) {
	if !graph.valid() || index < 0 || index >= len(graph.environments) {
		return EnvironmentEdgeNode{}, false
	}
	return graph.environments[index], true
}

func (graph *Graph) EnvironmentEdgeTotal() int {
	if !graph.valid() {
		return 0
	}
	return len(graph.environments)
}

func (graph *Graph) EnvironmentEdgeIndex(edge EnvironmentEdgeNode) (int, bool) {
	if !graph.valid() || edge.graph != graph || !edge.key.Available() {
		return 0, false
	}
	for index := range graph.environments {
		if graph.environments[index].key == edge.key {
			return index, true
		}
	}
	return 0, false
}

func (graph *Graph) EnvironmentOutgoingCount(point Point) int {
	if !graph.valid() {
		return 0
	}
	node, ok := graph.pointAt[point.key]
	if !ok {
		return 0
	}
	return len(graph.environmentOutgoing[node])
}

func (graph *Graph) EnvironmentOutgoingAt(point Point, index int) (EnvironmentEdgeNode, bool) {
	if !graph.valid() {
		return EnvironmentEdgeNode{}, false
	}
	node, ok := graph.pointAt[point.key]
	if !ok || index < 0 || index >= len(graph.environmentOutgoing[node]) {
		return EnvironmentEdgeNode{}, false
	}
	return graph.environments[graph.environmentOutgoing[node][index]], true
}

// EnvironmentGroupCount/At expose only Groups whose extra environment
// boundary names this source Point. Ordinary dependency ports are not
// conflated with this wake relation.
func (graph *Graph) EnvironmentGroupCount(point Point) int {
	if !graph.valid() {
		return 0
	}
	node, ok := graph.pointAt[point.key]
	if !ok {
		return 0
	}
	return len(graph.environmentGroups[node])
}

func (graph *Graph) EnvironmentGroupAt(point Point, index int) (GroupNode, bool) {
	if !graph.valid() {
		return GroupNode{}, false
	}
	node, ok := graph.pointAt[point.key]
	if !ok || index < 0 || index >= len(graph.environmentGroups[node]) {
		return GroupNode{}, false
	}
	return graph.groups[graph.environmentGroups[node][index]], true
}

// FactorEdgeCount reports Factor-local structural transports entering point.
func (graph *Graph) FactorEdgeCount(point Point) int {
	if !graph.valid() {
		return 0
	}
	node, ok := graph.pointAt[point.key]
	if !ok {
		return 0
	}
	return len(graph.factorIncoming[node])
}

func (graph *Graph) FactorEdgeAt(point Point, index int) (FactorEdgeNode, bool) {
	if !graph.valid() {
		return FactorEdgeNode{}, false
	}
	node, ok := graph.pointAt[point.key]
	if !ok || index < 0 || index >= len(graph.factorIncoming[node]) {
		return FactorEdgeNode{}, false
	}
	return graph.factorEdges[graph.factorIncoming[node][index]], true
}

func (graph *Graph) FactorEdgeAtIndex(index int) (FactorEdgeNode, bool) {
	if !graph.valid() || index < 0 || index >= len(graph.factorEdges) {
		return FactorEdgeNode{}, false
	}
	return graph.factorEdges[index], true
}

func (graph *Graph) FactorEdgeTotal() int {
	if !graph.valid() {
		return 0
	}
	return len(graph.factorEdges)
}

func (graph *Graph) FactorEdgeIndex(edge FactorEdgeNode) (int, bool) {
	if !graph.valid() || edge.graph != graph || !edge.key.Available() {
		return 0, false
	}
	for index := range graph.factorEdges {
		if graph.factorEdges[index].key == edge.key {
			return index, true
		}
	}
	return 0, false
}

func (graph *Graph) FactorOutgoingCount(point Point) int {
	if !graph.valid() {
		return 0
	}
	node, ok := graph.pointAt[point.key]
	if !ok {
		return 0
	}
	return len(graph.factorOutgoing[node])
}

func (graph *Graph) FactorOutgoingAt(point Point, index int) (FactorEdgeNode, bool) {
	if !graph.valid() {
		return FactorEdgeNode{}, false
	}
	node, ok := graph.pointAt[point.key]
	if !ok || index < 0 || index >= len(graph.factorOutgoing[node]) {
		return FactorEdgeNode{}, false
	}
	return graph.factorEdges[graph.factorOutgoing[node][index]], true
}

func (graph *Graph) QueryCount() int {
	if !graph.valid() {
		return 0
	}
	return len(graph.queries)
}
func (graph *Graph) QueryAt(index int) (Query, bool) {
	if !graph.valid() || index < 0 || index >= len(graph.queries) {
		return Query{}, false
	}
	return graph.queries[index], true
}

func (graph *Graph) OwnsQuery(query Query) bool {
	if !graph.valid() || query.graph != graph || !query.key.Available() {
		return false
	}
	_, ok := graph.queryAt[query.key]
	return ok
}
func (graph *Graph) DecisionCount() int {
	if !graph.valid() {
		return 0
	}
	return len(graph.decisions)
}
func (graph *Graph) DecisionAt(index int) (Decision, bool) {
	if !graph.valid() || index < 0 || index >= len(graph.decisions) {
		return Decision{}, false
	}
	return graph.decisions[index], true
}
func (graph *Graph) OwnsPoint(point Point) bool {
	if !graph.valid() || point.graph != graph || !point.Available() {
		return false
	}
	node, ok := graph.pointAt[point.key]
	return ok && graph.points[node].key == point.key
}

func (graph *Graph) PointIndex(point Point) (int, bool) {
	if !graph.OwnsPoint(point) {
		return 0, false
	}
	return int(graph.pointAt[point.key]), true
}

func (graph *Graph) OwnsGroup(group GroupNode) bool {
	if !graph.valid() || group.graph != graph || !group.key.Available() {
		return false
	}
	index, ok := graph.groupAt[group.key]
	return ok && index >= 0 && index < len(graph.groups) && graph.groups[index].key == group.key
}

func (graph *Graph) GroupIndex(group GroupNode) (int, bool) {
	if !graph.OwnsGroup(group) {
		return 0, false
	}
	return graph.groupAt[group.key], true
}
func (graph *Graph) OwnsMember(member RuleMember) bool {
	if !graph.valid() || member.graph != graph || !member.key.Available() {
		return false
	}
	_, ok := graph.memberAt[member.key]
	return ok
}
func (group GroupNode) Key() composition.Key { return group.key }
func (group GroupNode) Output() Point        { return group.output }

// Premise is the exact private accepted condition carried by this Group. It
// is unavailable to builder inputs; runtime lowering consumes it once at the
// Group output scope.
func (group GroupNode) Premise() Expr   { return group.premise }
func (group GroupNode) InputCount() int { return len(group.inputs) }
func (group GroupNode) InputAt(index int) (Input, bool) {
	if index < 0 || index >= len(group.inputs) {
		return Input{}, false
	}
	return group.inputs[index], true
}
func (group GroupNode) EnvironmentInput() (Input, bool) {
	return group.environmentInput, group.environmentInput.Available()
}
func (edge EnvironmentEdgeNode) Key() composition.Key { return edge.key }
func (edge EnvironmentEdgeNode) Target() Point        { return edge.target }
func (edge EnvironmentEdgeNode) Input() Input         { return edge.input }
func (edge FactorEdgeNode) Key() composition.Key      { return edge.key }
func (edge FactorEdgeNode) Target() Point             { return edge.target }
func (edge FactorEdgeNode) Input() Input              { return edge.input }
func (edge FactorEdgeNode) Factor() composition.Key   { return edge.factor }
func (group GroupNode) MemberCount() int              { return len(group.members) }
func (group GroupNode) MemberAt(index int) (RuleMember, bool) {
	if index < 0 || index >= len(group.members) {
		return RuleMember{}, false
	}
	return group.members[index], true
}
func (member RuleMember) Key() composition.Key           { return member.key }
func (member RuleMember) Rule() composition.Key          { return member.rule }
func (member RuleMember) OperandFamily() composition.Key { return member.operandFamily }
func (member RuleMember) Occurrence() Occurrence         { return member.occurrence }
func (member RuleMember) Operand() Operand               { return member.operand }

// ActivationMember returns the one topology-owned accepted relation that
// materialized this row. Ordinary rows have none. It remains equation-internal
// so domains can receive only the engine's opaque coordinate projection.
func (member RuleMember) ActivationMember() (Member, bool) {
	if !member.activation.Available() {
		return Member{}, false
	}
	return member.activation, true
}

func (member RuleMember) ReadCount() int { return len(member.binding.reads) }
func (member RuleMember) ReadAt(index int) (Surface, bool) {
	if index < 0 || index >= len(member.binding.reads) {
		return Surface{}, false
	}
	return member.binding.reads[index], true
}
func (member RuleMember) WriteCount() int { return len(member.binding.writes) }
func (member RuleMember) WriteAt(index int) (Surface, bool) {
	if index < 0 || index >= len(member.binding.writes) {
		return Surface{}, false
	}
	return member.binding.writes[index].surface, true
}

// WriteRouteRead returns the one-based ReadSelect ordinal consumed by a
// route write. It is zero for the ordinary exact and static-selector forms.
func (member RuleMember) WriteRouteRead(write int) (uint64, bool) {
	if write < 0 || write >= len(member.binding.writes) {
		return 0, false
	}
	return member.binding.writes[write].route, true
}
func (member RuleMember) WriteCandidateCount(write int) (int, bool) {
	if write < 0 || write >= len(member.binding.writes) {
		return 0, false
	}
	return len(member.binding.writes[write].candidates), true
}
func (member RuleMember) WriteCandidateAt(write, candidate int) (uint64, bool) {
	if write < 0 || write >= len(member.binding.writes) || candidate < 0 || candidate >= len(member.binding.writes[write].candidates) {
		return 0, false
	}
	return member.binding.writes[write].candidates[candidate], true
}

// WriteTargetCandidateAt returns the exact target paired positionally with a
// selector's cold read candidate. Unlike weak coverage, this vector is not a
// set: duplicates and authored order are preserved.
func (member RuleMember) WriteTargetCandidateAt(write, candidate int) (Surface, bool) {
	if write < 0 || write >= len(member.binding.writes) || candidate < 0 || candidate >= len(member.binding.writes[write].targets) {
		return Surface{}, false
	}
	return member.binding.writes[write].targets[candidate], true
}
func (member RuleMember) WriteDependencyCount(write int) (int, bool) {
	if write < 0 || write >= len(member.binding.writes) {
		return 0, false
	}
	return len(member.binding.writes[write].dependencies), true
}
func (member RuleMember) WriteDependencyAt(write, dependency int) (composition.Dependency, bool) {
	if write < 0 || write >= len(member.binding.writes) || dependency < 0 || dependency >= len(member.binding.writes[write].dependencies) {
		return composition.Dependency{}, false
	}
	return member.binding.writes[write].dependencies[dependency], true
}
func (member RuleMember) WriteRelationCount(write int) (int, bool) {
	if write < 0 || write >= len(member.binding.writes) {
		return 0, false
	}
	return len(member.binding.writes[write].relations), true
}
func (member RuleMember) WriteRelationAt(write, relation int) (CandidateRelation, bool) {
	if write < 0 || write >= len(member.binding.writes) || relation < 0 || relation >= len(member.binding.writes[write].relations) {
		return CandidateRelation{}, false
	}
	return cloneCandidateRelations(member.binding.writes[write].relations[relation : relation+1])[0], true
}

func compareKey(left, right composition.Key) int {
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
func lessKey(left, right composition.Key) bool { return compareKey(left, right) < 0 }
