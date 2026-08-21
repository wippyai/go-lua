package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/internal/canonical"
)

// Graph is the one immutable Point/Group equation topology. Points are the
// only schedule nodes, publishers, and query roots. Groups are private atomic
// RHS hyperedges; they never become schedule nodes or executable objects.
type Graph struct {
	self *Graph
	// payload is non-nil only for an activation revision view. Views share
	// every immutable structural row with the sealed initial graph and carry
	// only the publication header of the Relation they were issued for.
	payload        *Graph
	owner          *Topology
	relation       Relation
	composition    *composition.Composition
	scheduleRanked bool
	points         []Point
	pointAt        map[composition.Key]schedule.Node
	groups         []GroupNode
	groupAt        map[composition.Key]int
	memberAt       map[composition.Key]RuleMember
	producers      [][]int
	consumers      [][]int
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
	queryAt            map[composition.Key]Query
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
	graph         *Graph
	key           composition.Key
	target        Point
	input         Input
	transportOnly bool
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
	surface Surface
	route   uint64
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
	key           composition.Key
	target        Point
	input         Input
	transportOnly bool
}

type builtActivationReverse struct {
	target  Point
	trigger Point
}

// compiledRowDirectory is the immutable correspondence between disposable
// builder references and canonical graph-row identities. It is produced in
// the same compilation pass as the Graph; no later caller scans or rebuilds
// topology to recover that correspondence.
type compiledRowDirectory struct {
	points  []composition.Key
	members []composition.Key
	queries []composition.Key
}

// compileTopologyWithFailure is the sealed topology compiler's closed
// diagnostic form. The returned phase names only a compiler boundary; no
// mutable row or raw builder reference escapes this package.
// derivedActivationReverse is a compiler-local graph incidence. It is never a
// caller-authored activation row and is not retained by Topology.
type derivedActivationReverse struct {
	binding composition.Key
	target  composition.Key
	trigger composition.Key
}

func compileTopologyWithFailure(source *composition.Composition, topology TopologySpec, activationReverses []derivedActivationReverse, deferredQueries bool) (*Graph, compiledRowDirectory, SealFailure, bool) {
	if source == nil || !validTopologyBatch(topology.Batch, topology) || len(topology.Points) == 0 {
		return nil, compiledRowDirectory{}, sealRefused(SealFailureFamilyCompile, "input"), false
	}
	catalog, ok := buildTopologyCatalog(topology)
	if !ok {
		return nil, compiledRowDirectory{}, sealRefused(SealFailureFamilyCompile, "catalog"), false
	}
	if !validateTopologyCatalogUsage(topology, catalog) {
		return nil, compiledRowDirectory{}, sealRefused(SealFailureFamilyCompile, "catalog-usage"), false
	}
	instances, ok := buildInstances(source, topology.Batch, topology.Rules, catalog)
	if !ok {
		return nil, compiledRowDirectory{}, sealRefused(SealFailureFamilyCompile, "instances"), false
	}
	declared, sites, points, ok := buildPoints(topology.Points)
	if !ok {
		return nil, compiledRowDirectory{}, sealRefused(SealFailureFamilyCompile, "points"), false
	}
	groups, ok := buildGroups(source, instances, declared, sites, topology.Groups)
	if !ok {
		return nil, compiledRowDirectory{}, sealRefused(SealFailureFamilyCompile, "groups"), false
	}
	environments, ok := buildEnvironmentEdges(topology.EnvironmentEdges, declared, sites)
	if !ok {
		return nil, compiledRowDirectory{}, sealRefused(SealFailureFamilyCompile, "environment-edges"), false
	}
	factorEdges, ok := buildFactorEdges(source, topology.FactorEdges, declared, sites)
	if !ok {
		return nil, compiledRowDirectory{}, sealRefused(SealFailureFamilyCompile, "factor-edges"), false
	}
	queries, ok := buildQueries(source, declared, topology.Queries, catalog, deferredQueries)
	if !ok {
		return nil, compiledRowDirectory{}, sealRefused(SealFailureFamilyCompile, "queries"), false
	}
	reverses, ok := buildActivationReverseIndex(activationReverses, instances, declared, groups)
	if !ok {
		return nil, compiledRowDirectory{}, sealRefused(SealFailureFamilyCompile, "activation"), false
	}
	decisions, ok := collectDecisions(points, groups, environments, factorEdges)
	if !ok {
		return nil, compiledRowDirectory{}, sealRefused(SealFailureFamilyCompile, "decisions"), false
	}
	denseRanks := make([]int, 0)
	if len(topology.PointRanks) != 0 {
		denseRanks = make([]int, len(points))
		denseAt := make(map[composition.Key]int, len(points))
		for index, point := range points {
			if _, duplicate := denseAt[point.key]; duplicate {
				return nil, compiledRowDirectory{}, sealRefused(SealFailureFamilyCompile, "point-ranks"), false
			}
			denseAt[point.key] = index
		}
		for index := range topology.Points {
			point, found := declared[PointAt(index)]
			denseIndex, indexed := denseAt[point.key]
			if !found || !indexed {
				return nil, compiledRowDirectory{}, sealRefused(SealFailureFamilyCompile, "point-ranks"), false
			}
			denseRanks[denseIndex] = topology.PointRanks[index]
		}
	}
	graph, assembled := assembleGraph(source, points, groups, environments, factorEdges, queries, reverses, decisions, catalog, denseRanks)
	if !assembled || graph == nil {
		return nil, compiledRowDirectory{}, sealRefused(SealFailureFamilyCompile, "assembly"), false
	}
	directory := compiledRowDirectory{
		points:  make([]composition.Key, len(topology.Points)),
		members: make([]composition.Key, len(instances)),
		queries: make([]composition.Key, len(topology.Queries)),
	}
	for index := range topology.Points {
		point, found := declared[PointAt(index)]
		node, indexed := graph.pointAt[point.key]
		if !found || !indexed || int(node) < 0 || int(node) >= len(graph.points) || !graph.OwnsPoint(graph.points[node]) {
			return nil, compiledRowDirectory{}, sealRefused(SealFailureFamilyCompile, "row-directory"), false
		}
		directory.points[index] = point.key
	}
	for index, instance := range instances {
		member, found := graph.memberAt[instance.key]
		if !found || !graph.OwnsMember(member) {
			return nil, compiledRowDirectory{}, sealRefused(SealFailureFamilyCompile, "row-directory"), false
		}
		directory.members[index] = instance.key
	}
	for index, row := range topology.Queries {
		point, found := declared[row.Point]
		if !found {
			return nil, compiledRowDirectory{}, sealRefused(SealFailureFamilyCompile, "row-directory"), false
		}
		key, keyed := deriveQueryKey(row, point, catalog)
		query, found := graph.queryAt[key]
		if !keyed || !found || !graph.OwnsQuery(query) {
			return nil, compiledRowDirectory{}, sealRefused(SealFailureFamilyCompile, "row-directory"), false
		}
		directory.queries[index] = key
	}
	return graph, directory, SealFailure{}, true
}

func validTopologyBatch(batch *Batch, topology TopologySpec) bool {
	if !batch.Sealed() {
		return false
	}
	if len(topology.PointRanks) != 0 {
		if len(topology.PointRanks) != len(topology.Points) {
			return false
		}
		seenRanks := make([]bool, len(topology.PointRanks))
		for _, rank := range topology.PointRanks {
			if rank < 0 || rank >= len(seenRanks) || seenRanks[rank] {
				return false
			}
			seenRanks[rank] = true
		}
	}
	for _, point := range topology.Points {
		if !batch.ownsConcreteSite(point.Site) {
			return false
		}
	}
	for _, rule := range topology.Rules {
		if !batch.ownsOccurrence(rule.Occurrence) || !batch.ownsConcreteSite(rule.Occurrence.Site()) || !batch.ownsOperand(rule.Operand) || !rule.Operand.Occurrence().Same(rule.Occurrence) {
			return false
		}
	}
	for _, group := range topology.Groups {
		for _, input := range group.Inputs {
			if !batch.ownsConcreteSite(input.Source()) || !batch.ownsConcreteSite(input.Target()) {
				return false
			}
		}
	}
	for _, edge := range topology.EnvironmentEdges {
		if !batch.ownsConcreteSite(edge.Input.Source()) || !batch.ownsConcreteSite(edge.Input.Target()) {
			return false
		}
	}
	for _, edge := range topology.FactorEdges {
		if !batch.ownsConcreteSite(edge.Input.Source()) || !batch.ownsConcreteSite(edge.Input.Target()) || !edge.Factor.Available() {
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
	// A directory may legitimately contain only materialized boundary points
	// and transport inputs. Empty rule catalogs are closed, not malformed.
	return result, true
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
			return writeKey(writer, input.Key()) && writePoint(writer, target) && writer.Uint(boolUint(row.TransportOnly)) == nil
		})
		if !keyOK || !key.Available() {
			return nil, false
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		seen[key] = struct{}{}
		result[index] = builtEnvironmentEdge{key: key, target: target, input: input, transportOnly: row.TransportOnly}
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

func buildQueries(source *composition.Composition, declared map[PointRef]Point, rows []QueryInstance, catalog topologyCatalog, deferredQueries bool) ([]Query, bool) {
	families := source.Queries()
	if deferredQueries && len(rows) == 0 {
		return nil, true
	}
	// A callback-free Factor/Rule schema may legitimately have no Query
	// families while its graph is being compiled into a reusable transformer.
	// Preserve exact inventory coverage: the empty cold denominator accepts
	// only an empty instance set; any nonempty denominator still requires every
	// family and at least one concrete observation.
	if len(families) == 0 {
		return nil, len(rows) == 0
	}
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

func assembleGraph(source *composition.Composition, points []Point, built []builtGroup, environments []builtEnvironmentEdge, factorEdges []builtFactorEdge, queries []Query, reverses []builtActivationReverse, decisions []Decision, catalog topologyCatalog, semanticRanks ...[]int) (*Graph, bool) {
	graph := &Graph{composition: source, scheduleRanked: len(semanticRanks) != 0 && len(semanticRanks[0]) != 0, points: append([]Point(nil), points...), pointAt: make(map[composition.Key]schedule.Node, len(points)), groups: make([]GroupNode, len(built)), groupAt: make(map[composition.Key]int, len(built)), memberAt: make(map[composition.Key]RuleMember), producers: make([][]int, len(points)), consumers: make([][]int, len(points)), environments: make([]EnvironmentEdgeNode, len(environments)), environmentIncoming: make([][]int, len(points)), environmentOutgoing: make([][]int, len(points)), environmentGroups: make([][]int, len(points)), factorEdges: make([]FactorEdgeNode, len(factorEdges)), factorIncoming: make([][]int, len(points)), factorOutgoing: make([][]int, len(points)), activationReverses: make([][]schedule.Node, len(points)), queries: append([]Query(nil), queries...), queryAt: make(map[composition.Key]Query, len(queries)), decisions: append([]Decision(nil), decisions...)}
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
		graph.queryAt[query.key] = graph.queries[index]
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
			graph.memberAt[members[memberIndex].key] = members[memberIndex]
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
		graph.environments[edgeIndex] = EnvironmentEdgeNode{graph: graph, key: row.key, target: graph.points[target], input: input, transportOnly: row.transportOnly}
		graph.environmentIncoming[target] = append(graph.environmentIncoming[target], edgeIndex)
		graph.environmentOutgoing[sourcePoint] = append(graph.environmentOutgoing[sourcePoint], edgeIndex)
		if !row.transportOnly {
			edges[schedule.Edge{From: sourcePoint, To: target}] = struct{}{}
		}
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
	var prepared *schedule.Schedule
	var err error
	if len(semanticRanks) != 0 && len(semanticRanks[0]) != 0 {
		prepared, err = schedule.PrepareOrdered(len(graph.points), ordered, semanticRanks[0])
	} else {
		prepared, err = schedule.Prepare(len(graph.points), ordered)
	}
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
		binding.writes[index] = ruleWrite{surface: write.Surface, route: write.Route}
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
		resolved := ResolvedWrite{Index: uint64(index), Surface: write.surface, Route: write.route}
		if !matchesWriteSurface(write.surface, schema.Writes[index]) || !validResolvedWriteRoute(resolved, schema.Writes[index]) {
			return false
		}
	}
	return true
}

func (binding ruleBinding) clone() ruleBinding {
	return ruleBinding{reads: append([]Surface(nil), binding.reads...), writes: append([]ruleWrite(nil), binding.writes...)}
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

// validateResolvedInstanceAt authenticates one instance against scalar
// projections of the sealed cold Rule.  It is the hot path used by
// RuleInstance.ValidFor: unlike Rules/RuleAt it never detaches the Rule or
// any of its child slices from the immutable Composition.
func validateResolvedInstanceAt(row RuleInstance, source *composition.Composition, ordinal uint64) bool {
	if source == nil || !row.Schema.Available() || !row.OperandFamily.Available() || !row.Operand.Available() {
		return false
	}
	shape, ok := source.RuleShapeAt(ordinal)
	if !ok || row.OperandFamily != shape.OperandFamily || shape.ActivationCount > 1 ||
		uint64(len(row.Reads)) != shape.ReadCount || uint64(len(row.Carries)) != shape.CarryCount ||
		uint64(len(row.Writes)) != shape.WriteCount || uint64(len(row.Supports)) != shape.SupportCount ||
		uint64(len(row.Prunes)) != shape.PruneCount {
		return false
	}
	for index, value := range row.Reads {
		expected, expectedOK := source.RuleReadShapeAt(ordinal, uint64(index))
		if !expectedOK || value.Index != uint64(index) || !matchesReadShape(value.Surface, expected) {
			return false
		}
	}
	for index, value := range row.Carries {
		if value.Index != uint64(index) {
			return false
		}
	}
	for index, value := range row.Writes {
		expected, expectedOK := source.RuleWriteShapeAt(ordinal, uint64(index))
		if !expectedOK || value.Index != uint64(index) || !matchesWriteShape(value.Surface, expected) || !validResolvedWriteRouteShape(value, expected) {
			return false
		}
	}
	for index, value := range row.Supports {
		expected, expectedOK := source.RuleSupportShapeAt(ordinal, uint64(index))
		if !expectedOK || value.Index != uint64(index) || !matchesSupportShape(value.Surface, expected) {
			return false
		}
	}
	for index, value := range row.Prunes {
		expected, expectedOK := source.RulePruneShapeAt(ordinal, uint64(index))
		if !expectedOK || value.Index != uint64(index) || !matchesPruneShape(value.Surface, expected) {
			return false
		}
	}
	return true
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
		if value.Index != uint64(index) || !matchesWriteSurface(value.Surface, schemas[index]) || !validResolvedWriteRoute(value, schemas[index]) {
			return false
		}
	}
	return true
}

func validResolvedWriteRoute(value ResolvedWrite, schema composition.Write) bool {
	return validResolvedWriteRouteFields(value.Route, schema.Kind, schema.Route)
}

func validResolvedWriteRouteShape(value ResolvedWrite, schema composition.RuleWriteShape) bool {
	return validResolvedWriteRouteFields(value.Route, schema.Kind, schema.Route)
}

func validResolvedWriteRouteFields(route uint64, kind composition.WriteKind, expected uint64) bool {
	switch kind {
	case composition.WriteExact:
		return route == 0
	case composition.WriteRoute:
		// Schema validation already proves Route is a one-based ReadSelect
		// ordinal. It is not a write ordinal, so its range is unrelated to
		// len(writes) and may validly exceed it.
		return route == expected && route != 0
	default:
		return false
	}
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
	return matchesReadFields(surface, schema.Kind, schema.Factor, schema.Semantic, schema.Normalizer)
}

func matchesReadShape(surface Surface, schema composition.RuleReadShape) bool {
	return matchesReadFields(surface, schema.Kind, schema.Factor, schema.Semantic, schema.Normalizer)
}

func matchesReadFields(surface Surface, kind composition.ReadKind, factor, semantic, normalizer composition.Key) bool {
	if !surface.Available() || surface.Factor != factor {
		return false
	}
	switch kind {
	case composition.ReadExact:
		return surface.Form == SurfaceReadExact && surface.Mode == TargetModeNone && !surface.Semantic.Available() && !surface.Normalizer.Available()
	case composition.ReadSummary:
		return surface.Form == SurfaceReadSummary && surface.Mode == TargetModeNone && surface.Semantic == semantic && surface.Normalizer == normalizer
	case composition.ReadSelect:
		return surface.Form == SurfaceReadSelect && surface.Mode == TargetModeNone && surface.Semantic == semantic && surface.Normalizer == normalizer
	default:
		return false
	}
}
func matchesWriteSurface(surface Surface, schema composition.Write) bool {
	return matchesWriteFields(surface, schema.Kind, schema.Factor)
}

func matchesWriteShape(surface Surface, schema composition.RuleWriteShape) bool {
	return matchesWriteFields(surface, schema.Kind, schema.Factor)
}

func matchesWriteFields(surface Surface, kind composition.WriteKind, factor composition.Key) bool {
	if !surface.Available() || surface.Factor != factor {
		return false
	}
	switch kind {
	case composition.WriteExact:
		return surface.Form == SurfaceWriteExact && (surface.Mode == TargetModeStrong || surface.Mode == TargetModeWeak) && !surface.Semantic.Available() && !surface.Normalizer.Available()
	case composition.WriteRoute:
		return surface.Form == SurfaceWriteRoute && surface.Mode == TargetModeNone && !surface.Semantic.Available() && !surface.Normalizer.Available()
	default:
		return false
	}
}
func matchesSupportSurface(surface StructuralSurface, schema composition.Support) bool {
	return matchesStructuralSurface(surface, schema.Semantic)
}
func matchesPruneSurface(surface StructuralSurface, schema composition.Prune) bool {
	return matchesStructuralSurface(surface, schema.Semantic)
}
func matchesSupportShape(surface StructuralSurface, schema composition.RuleSupportShape) bool {
	return matchesStructuralSurface(surface, schema.Semantic)
}
func matchesPruneShape(surface StructuralSurface, schema composition.RulePruneShape) bool {
	return matchesStructuralSurface(surface, schema.Semantic)
}
func matchesStructuralSurface(surface StructuralSurface, semantic composition.Key) bool {
	return surface.Available() && surface.Semantic == semantic
}
func copyInstance(row RuleInstance) RuleInstance {
	result := row
	result.Reads = append([]ResolvedRead(nil), row.Reads...)
	result.Carries = append([]ResolvedCarry(nil), row.Carries...)
	result.Writes = append([]ResolvedWrite(nil), row.Writes...)
	result.Supports = append([]ResolvedSupport(nil), row.Supports...)
	result.Prunes = append([]ResolvedPrune(nil), row.Prunes...)
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

func (graph *Graph) ownsNode(owner *Graph) bool {
	return graph != nil && (owner == graph || graph.payload != nil && owner == graph.payload)
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
	if !graph.valid() || !graph.ownsNode(edge.graph) || !edge.key.Available() {
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
	if !graph.valid() || !graph.ownsNode(edge.graph) || !edge.key.Available() {
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
	if !graph.valid() || !graph.ownsNode(query.graph) || !query.key.Available() {
		return false
	}
	owned, ok := graph.queryAt[query.key]
	return ok && graph.ownsNode(owned.graph) && owned.key == query.key
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
	if !graph.valid() || !graph.ownsNode(point.graph) || !point.Available() {
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
	if !graph.valid() || !graph.ownsNode(group.graph) || !group.key.Available() {
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
	if !graph.valid() || !graph.ownsNode(member.graph) || !member.key.Available() {
		return false
	}
	owned, ok := graph.memberAt[member.key]
	return ok && graph.ownsNode(owned.graph) && owned.key == member.key
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
func (edge EnvironmentEdgeNode) TransportOnly() bool  { return edge.transportOnly }
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
