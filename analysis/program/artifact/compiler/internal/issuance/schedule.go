package issuance

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	artifactdigest "github.com/wippyai/go-lua/analysis/program/artifact/digest"
	"github.com/wippyai/go-lua/analysis/schema"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
)

// Node is one canonical execution cut. A node may be present only to close a
// declared stage predecessor; Emission remains the set of rule-producing
// requests. Both are returned in the same deterministic stage order.
type Node struct {
	stage *schemaissuance.Entry
	base  identity.ContentID
	point identity.ContentID
	args  []value
}

func (node Node) Stage() *schemaissuance.Entry { return node.stage }
func (node Node) Base() identity.ContentID     { return node.base }
func (node Node) Point() identity.ContentID    { return node.point }

// Route answers the route this node carries in its own identity, if it carries
// one. It is the scheduled counterpart of the declaration's route parameter,
// and it is how a host tells a stage standing on a route from one standing in
// its base's chain without reading the stage's key.
func (node Node) Route() (identity.ContentID, bool) {
	return nodeRoute(&node)
}

// Emission is the atomic final result of one emitted request. Inputs are
// selected from the sealed input policies before the compiler can publish a
// rule row, so no downstream input rewrite exists.
type Emission struct {
	request    Request
	point      identity.ContentID
	inputs     [6]identity.ContentID
	inputCount uint8
	native     bool
}

func (emission Emission) Request() Request          { return emission.request }
func (emission Emission) Point() identity.ContentID { return emission.point }
func (emission Emission) InputPointCount() int {
	if emission.request.stage == nil || emission.inputCount > uint8(len(emission.inputs)) {
		return 0
	}
	return int(emission.inputCount)
}
func (emission Emission) InputPointAt(index int) (identity.ContentID, bool) {
	if index < 0 || index >= emission.InputPointCount() {
		return identity.ContentID{}, false
	}
	return emission.inputs[index], true
}
func (emission Emission) Native() (bool, bool) {
	return emission.native, emission.request.stage != nil && emission.request.stage.Kind() == schemaissuance.KindStage
}

type Schedule struct {
	nodes       []Node
	emissions   []Emission
	pointWrites map[identity.ContentID][]schema.Key
	stageWrites map[identity.ContentID]map[schema.Key][]schema.Key
	sealed      bool
}

func (schedule Schedule) NodeCount() int     { return len(schedule.nodes) }
func (schedule Schedule) EmissionCount() int { return len(schedule.emissions) }
func (schedule Schedule) NodeAt(index int) (Node, bool) {
	if index < 0 || index >= len(schedule.nodes) {
		return Node{}, false
	}
	return schedule.nodes[index], true
}
func (schedule Schedule) EmissionAt(index int) (Emission, bool) {
	if index < 0 || index >= len(schedule.emissions) {
		return Emission{}, false
	}
	return schedule.emissions[index], true
}

// PointWriters returns the exact axis union issued by final emissions at one
// point. found is false when no emission owns that point; the caller must not
// confuse absence with an empty transport declaration.
func (schedule Schedule) PointWriters(point identity.ContentID) ([]schema.Key, bool) {
	if !schedule.sealed || !point.Available() {
		return nil, false
	}
	writes, found := schedule.pointWrites[point]
	return append([]schema.Key(nil), writes...), found
}

// StageWriters returns the exact axis union issued for one base/stage pair.
// An absent pair is a valid empty set in a sealed schedule: declarations may
// name a writer stage that emitted no request for this execution group.
func (schedule Schedule) StageWriters(base identity.ContentID, stage schema.Key) ([]schema.Key, bool) {
	if !schedule.sealed || !base.Available() || !stage.Available() {
		return nil, false
	}
	return append([]schema.Key(nil), schedule.stageWrites[base][stage]...), true
}

type nodeKey struct {
	stage schema.Key
	point identity.ContentID
}

// BuildSchedule constructs stage identities, closes declared predecessors,
// orders repeating stages by their declared node/dependency parameters, and
// resolves every input from its sealed generic selection policy.
// RouteSource answers the point one declared route departs from. The host owns
// the environment, so it owns this mapping; the schedule consults it and never
// substitutes a chain position when it has no answer.
type RouteSource func(route identity.ContentID) (identity.ContentID, bool)

func BuildSchedule(format uint64, plan schemaissuance.Plan, requests []Request, routeSource RouteSource) (Schedule, bool) {
	if format == 0 {
		return Schedule{}, false
	}
	byBase := make(map[identity.ContentID]map[nodeKey]*Node)
	requestNodes := make([]*Node, len(requests))
	for index := range requests {
		request := requests[index]
		node, ok := nodeForRequest(format, request)
		if !ok {
			return Schedule{}, false
		}
		bucket := byBase[node.base]
		if bucket == nil {
			bucket = make(map[nodeKey]*Node)
			byBase[node.base] = bucket
		}
		key := nodeKey{stage: node.stage.Key(), point: node.point}
		if existing := bucket[key]; existing != nil {
			if !sameArguments(existing.args, node.args) {
				return Schedule{}, false
			}
			requestNodes[index] = existing
			continue
		}
		copyOf := node
		bucket[key] = &copyOf
		requestNodes[index] = &copyOf
	}
	if !closePredecessors(format, plan.Table(), byBase) {
		return Schedule{}, false
	}
	bases := make([]identity.ContentID, 0, len(byBase))
	for base := range byBase {
		bases = append(bases, base)
	}
	sort.Slice(bases, func(left, right int) bool { return contentIDLess(bases[left], bases[right]) })
	orderedByBase := make(map[identity.ContentID][]*Node, len(bases))
	schedule := Schedule{
		pointWrites: make(map[identity.ContentID][]schema.Key),
		stageWrites: make(map[identity.ContentID]map[schema.Key][]schema.Key),
	}
	pointSets := make(map[identity.ContentID]map[schema.Key]struct{})
	stageSets := make(map[identity.ContentID]map[schema.Key]map[schema.Key]struct{})
	for _, base := range bases {
		ordered, ok := orderBucket(byBase[base])
		if !ok {
			return Schedule{}, false
		}
		orderedByBase[base] = ordered
		for _, node := range ordered {
			schedule.nodes = append(schedule.nodes, *node)
		}
	}
	for index, request := range requests {
		node := requestNodes[index]
		subscription := request.Subscription()
		stage := request.Stage()
		inputCount := request.InputCount()
		if stage != nil && inputCount != int(stage.InputCount()) {
			return Schedule{}, false
		}
		if inputCount > len(Emission{}.inputs) {
			return Schedule{}, false
		}
		var inputs [6]identity.ContentID
		for inputIndex := 0; inputIndex < inputCount; inputIndex++ {
			input, hasInput, inputOK := resolveInput(request, requestInput(request, inputIndex), node, orderedByBase[request.base], orderedByBase, routeSource)
			if !inputOK || !hasInput || !input.Available() {
				return Schedule{}, false
			}
			inputs[inputIndex] = input
		}
		if node == nil || !node.point.Available() || !request.base.Available() ||
			stage == nil || stage.Kind() != schemaissuance.KindStage || !stage.Key().Available() ||
			!subscription.Available() || !subscription.Writes().Available() {
			return Schedule{}, false
		}
		schedule.emissions = append(schedule.emissions, Emission{request: request, point: node.point, inputs: inputs, inputCount: uint8(inputCount), native: stage.Native()})
		pointSet := pointSets[node.point]
		if pointSet == nil {
			pointSet = make(map[schema.Key]struct{})
			pointSets[node.point] = pointSet
		}
		pointSet[subscription.Writes()] = struct{}{}
		byStage := stageSets[request.base]
		if byStage == nil {
			byStage = make(map[schema.Key]map[schema.Key]struct{})
			stageSets[request.base] = byStage
		}
		stageSet := byStage[stage.Key()]
		if stageSet == nil {
			stageSet = make(map[schema.Key]struct{})
			byStage[stage.Key()] = stageSet
		}
		stageSet[subscription.Writes()] = struct{}{}
	}
	for point, set := range pointSets {
		schedule.pointWrites[point] = sortedSchemaKeys(set)
	}
	for base, byStage := range stageSets {
		schedule.stageWrites[base] = make(map[schema.Key][]schema.Key, len(byStage))
		for stage, set := range byStage {
			schedule.stageWrites[base][stage] = sortedSchemaKeys(set)
		}
	}
	schedule.sealed = true
	return schedule, true
}

func sortedSchemaKeys(set map[schema.Key]struct{}) []schema.Key {
	keys := make([]schema.Key, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left] < keys[right] })
	return keys
}

func nodeForRequest(format uint64, request Request) (Node, bool) {
	if request.stage == nil || request.stage.Kind() != schemaissuance.KindStage || !request.base.Available() || len(request.parameters) != len(request.stage.Parameters()) {
		return Node{}, false
	}
	point, ok := stagePoint(format, request.stage, request.parameters)
	return Node{stage: request.stage, base: request.base, point: point, args: cloneValues(request.parameters)}, ok && point.Available()
}

func closePredecessors(format uint64, table schemaissuance.Table, byBase map[identity.ContentID]map[nodeKey]*Node) bool {
	for changed := true; changed; {
		changed = false
		for base, bucket := range byBase {
			nodes := nodePointers(bucket)
			for _, node := range nodes {
				for _, predecessorKey := range node.stage.Predecessors() {
					if stagePresent(bucket, predecessorKey) {
						continue
					}
					stage, ok := table.Entry(predecessorKey, schemaissuance.KindStage)
					parameters := stage.Parameters()
					if !ok || len(parameters) != 1 || stage.BaseParameter() != 1 ||
						parameters[0].Value != schemaissuance.ValuePointRange || parameters[0].Name != schemaissuance.TypePoint ||
						(parameters[0].Cardinality != schemaissuance.CardinalityOne && parameters[0].Cardinality != schemaissuance.CardinalityMany) {
						return false
					}
					args := []value{{typ: parameters[0], present: true, points: []identity.ContentID{base}}}
					point, pointOK := stagePoint(format, stage, args)
					key := nodeKey{stage: stage.Key(), point: point}
					if !pointOK || bucket[key] != nil {
						return false
					}
					bucket[key] = &Node{stage: stage, base: base, point: point, args: args}
					changed = true
				}
			}
		}
	}
	return true
}

func stagePoint(format uint64, stage *schemaissuance.Entry, args []value) (identity.ContentID, bool) {
	if stage == nil || len(args) != len(stage.Parameters()) {
		return identity.ContentID{}, false
	}
	baseIndex := int(stage.BaseParameter()) - 1
	if baseIndex < 0 || baseIndex >= len(args) || len(args[baseIndex].points) != 1 {
		return identity.ContentID{}, false
	}
	base := args[baseIndex].points[0]
	if !base.Available() {
		return identity.ContentID{}, false
	}
	if stage.Constructor() == schemaissuance.StageConstructorPassthrough {
		return base, true
	}
	fields := make([]artifactdigest.Field, 0, len(stage.IdentityParameters()))
	for _, parameter := range stage.IdentityParameters() {
		field, ok := identityField(args[parameter-1])
		if !ok {
			return identity.ContentID{}, false
		}
		fields = append(fields, field)
	}
	point := artifactdigest.Digest(stage.Framing(), format, fields...)
	return point, point.Available() && point != base
}

func identityField(argument value) (artifactdigest.Field, bool) {
	if !argument.present {
		return artifactdigest.Field{}, false
	}
	switch argument.typ.Value {
	case schemaissuance.ValuePointRange:
		if len(argument.points) != 1 || !argument.points[0].Available() {
			return artifactdigest.Field{}, false
		}
		return artifactdigest.ContentID(argument.points[0]), true
	case schemaissuance.ValueIdentity:
		if argument.typ.Name == schemaissuance.TypeRuleKey || argument.typ.Name == schemaissuance.TypeAxisKey {
			return artifactdigest.Key(argument.key), argument.key.Available()
		}
		return artifactdigest.ContentID(argument.identity), argument.identity.Available()
	case schemaissuance.ValueUint:
		return artifactdigest.Uint(argument.unsigned), true
	case schemaissuance.ValueBool:
		return artifactdigest.Bool(argument.boolean), true
	default:
		return artifactdigest.Field{}, false
	}
}

func orderBucket(bucket map[nodeKey]*Node) ([]*Node, bool) {
	byOrder := make(map[uint16][]*Node)
	var orders []uint16
	seenOrder := make(map[uint16]schema.Key)
	seenPoint := make(map[identity.ContentID]schema.Key)
	for _, node := range bucket {
		if prior, duplicate := seenPoint[node.point]; duplicate && prior != node.stage.Key() {
			return nil, false
		}
		seenPoint[node.point] = node.stage.Key()
		order := node.stage.Order()
		if prior, present := seenOrder[order]; present && prior != node.stage.Key() {
			return nil, false
		}
		if _, present := seenOrder[order]; !present {
			orders = append(orders, order)
			seenOrder[order] = node.stage.Key()
		}
		byOrder[order] = append(byOrder[order], node)
	}
	sort.Slice(orders, func(left, right int) bool { return orders[left] < orders[right] })
	ordered := make([]*Node, 0, len(bucket))
	for _, order := range orders {
		rows, ok := orderStage(byOrder[order])
		if !ok {
			return nil, false
		}
		ordered = append(ordered, rows...)
	}
	return ordered, true
}

func orderStage(nodes []*Node) ([]*Node, bool) {
	if len(nodes) < 2 || nodes[0].stage.NodeParameter() == 0 {
		sort.Slice(nodes, func(left, right int) bool { return contentIDLess(nodes[left].point, nodes[right].point) })
		return nodes, true
	}
	nodeParameter := nodes[0].stage.NodeParameter()
	dependencies := nodes[0].stage.DependencyParameters()
	byIdentity := make(map[identity.ContentID]*Node, len(nodes))
	for _, node := range nodes {
		id, ok := argumentIdentity(node.args[nodeParameter-1])
		if !ok || byIdentity[id] != nil {
			return nil, false
		}
		byIdentity[id] = node
	}
	indegree := make(map[*Node]int, len(nodes))
	edges := make(map[*Node][]*Node, len(nodes))
	for _, node := range nodes {
		for _, parameter := range dependencies {
			id, ok := argumentIdentity(node.args[parameter-1])
			if !ok {
				return nil, false
			}
			if dependency := byIdentity[id]; dependency != nil {
				edges[dependency] = append(edges[dependency], node)
				indegree[node]++
			}
		}
	}
	ready := make([]*Node, 0, len(nodes))
	for _, node := range nodes {
		if indegree[node] == 0 {
			ready = append(ready, node)
		}
	}
	var ordered []*Node
	for len(ready) != 0 {
		sort.Slice(ready, func(left, right int) bool { return contentIDLess(ready[left].point, ready[right].point) })
		node := ready[0]
		ready = ready[1:]
		ordered = append(ordered, node)
		for _, successor := range edges[node] {
			indegree[successor]--
			if indegree[successor] == 0 {
				ready = append(ready, successor)
			}
		}
	}
	return ordered, len(ordered) == len(nodes)
}

func requestInput(request Request, index int) Input {
	input, _ := request.InputAt(index)
	return input
}

// nodeRoute answers the route a stage carries in its own identity. A stage
// without one cannot take a routed input, which is the schedule's own statement
// of the same law the seal states over the declaration.
func nodeRoute(node *Node) (identity.ContentID, bool) {
	if node == nil || node.stage == nil {
		return identity.ContentID{}, false
	}
	for _, parameter := range node.stage.IdentityParameters() {
		if int(parameter) < 1 || int(parameter) > len(node.args) {
			return identity.ContentID{}, false
		}
		argument := node.args[parameter-1]
		if argument.typ == schemaissuance.IdentityType(schemaissuance.TypeRouteIdentity) {
			return argument.identity, argument.present && argument.identity.Available()
		}
	}
	return identity.ContentID{}, false
}

// terminalLinearPoint answers where a base's own chain ends. Routed nodes do
// not stand in that chain - they stand on the routes that reach the point - so
// the state leaving a base is the last node that is not routed, and the base
// itself when it has no chain at all.
func terminalLinearPoint(base identity.ContentID, ordered []*Node) identity.ContentID {
	terminal := base
	for _, candidate := range ordered {
		if _, routed := nodeRoute(candidate); routed {
			continue
		}
		if candidate.point == base {
			continue
		}
		terminal = candidate.point
	}
	return terminal
}

func resolveInput(request Request, input Input, node *Node, ordered []*Node, orderedByBase map[identity.ContentID][]*Node, routeSource RouteSource) (identity.ContentID, bool, bool) {
	declaration := input.declaration
	if declaration == nil || node == nil {
		return identity.ContentID{}, false, false
	}
	switch declaration.InputSelection() {
	case schemaissuance.InputSelectionNone:
		return identity.ContentID{}, false, len(input.points) == 0
	case schemaissuance.InputSelectionDriver:
		if request.driverIndex < 0 || request.driverIndex >= len(input.points) {
			return identity.ContentID{}, false, false
		}
		return input.points[request.driverIndex], true, input.points[request.driverIndex].Available()
	case schemaissuance.InputSelectionOnly:
		if len(input.points) != 1 {
			return identity.ContentID{}, false, false
		}
		return input.points[0], true, input.points[0].Available()
	case schemaissuance.InputSelectionStage:
		var selected identity.ContentID
		for _, candidate := range ordered {
			if candidate.stage.Key() != declaration.Source() {
				continue
			}
			if selected.Available() {
				return identity.ContentID{}, false, false
			}
			selected = candidate.point
		}
		return selected, true, selected.Available()
	case schemaissuance.InputSelectionPrevious:
		// Routed stages stand on their routes, not in this chain, so the stage
		// before this one is the last unrouted one. Counting them here would
		// name a point the chain never transfers from.
		previous := request.base
		for _, candidate := range ordered {
			if candidate == node {
				return previous, true, previous.Available()
			}
			if _, routed := nodeRoute(candidate); routed {
				continue
			}
			previous = candidate.point
		}
		return identity.ContentID{}, false, false
	case schemaissuance.InputSelectionRoute:
		// The stage stands on its route, so its input is the state that route
		// carries. A stage with no route in its identity, or a route the host
		// cannot place, is refused here rather than quietly reading the linear
		// chain - which is the very point this stage was moved off.
		route, routeOK := nodeRoute(node)
		if !routeOK || routeSource == nil {
			return identity.ContentID{}, false, false
		}
		source, sourceOK := routeSource(route)
		if !sourceOK || !source.Available() {
			return identity.ContentID{}, false, false
		}
		// The route carries the state its source finished with, so the input is
		// that base's terminal point, not the base before its own chain ran.
		terminal := terminalLinearPoint(source, orderedByBase[source])
		return terminal, true, terminal.Available()
	default:
		return identity.ContentID{}, false, false
	}
}

func nodePointers(bucket map[nodeKey]*Node) []*Node {
	result := make([]*Node, 0, len(bucket))
	for _, node := range bucket {
		result = append(result, node)
	}
	return result
}

func stagePresent(bucket map[nodeKey]*Node, stage schema.Key) bool {
	for key := range bucket {
		if key.stage == stage {
			return true
		}
	}
	return false
}

func argumentIdentity(argument value) (identity.ContentID, bool) {
	return argument.identity, argument.present && argument.typ.Value == schemaissuance.ValueIdentity && argument.identity.Available()
}

func cloneValues(values []value) []value {
	cloned := append([]value(nil), values...)
	for index := range cloned {
		cloned[index].rows = append([]programissuance.Row(nil), cloned[index].rows...)
		cloned[index].points = append([]identity.ContentID(nil), cloned[index].points...)
		cloned[index].requests = append([]Request(nil), cloned[index].requests...)
	}
	return cloned
}

func cloneInputs(inputs []Input) []Input {
	cloned := make([]Input, len(inputs))
	for index, input := range inputs {
		cloned[index] = Input{declaration: input.declaration, points: append([]identity.ContentID(nil), input.points...)}
	}
	return cloned
}

func sameArguments(left, right []value) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].typ != right[index].typ || left[index].present != right[index].present ||
			left[index].boolean != right[index].boolean || left[index].unsigned != right[index].unsigned ||
			left[index].identity != right[index].identity || left[index].key != right[index].key ||
			len(left[index].points) != len(right[index].points) {
			return false
		}
		for pointIndex := range left[index].points {
			if left[index].points[pointIndex] != right[index].points[pointIndex] {
				return false
			}
		}
	}
	return true
}

func contentIDLess(left, right identity.ContentID) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return false
}
