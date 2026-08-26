package compiler

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	issuanceexecutor "github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/issuance"
	"github.com/wippyai/go-lua/analysis/schema"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
)

// installLocalStagesFailure materializes the already-sealed generic schedule.
// Transport sources, factor sets, and identity framings are declaration data;
// this function contains no call/local/computation stage cases.
func (compiler *compiler) installLocalStagesFailure() CompileFailure {
	if compiler == nil || compiler.localTransfer == nil {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	byBase := make(map[identity.ContentID][]issuanceexecutor.Node)
	for index := 0; index < compiler.issuanceSchedule.NodeCount(); index++ {
		node, ok := compiler.issuanceSchedule.NodeAt(index)
		if !ok || node.Stage() == nil || !node.Base().Available() || !node.Point().Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		byBase[node.Base()] = append(byBase[node.Base()], node)
	}
	// A routed stage does not stand in its base's linear chain: it stands on
	// the route that reaches it. So the chain is built from the linear nodes
	// alone, and the routed nodes are indexed by their route instead. Both
	// still become points, join the base's region, and take a WTO event.
	stageFor := make(map[identity.ContentID][]identity.ContentID, len(byBase))
	linearFor := make(map[identity.ContentID][]identity.ContentID, len(byBase))
	linearNodes := make(map[identity.ContentID][]issuanceexecutor.Node, len(byBase))
	routeStage := make(map[identity.ContentID][]routedStagePlacement)
	preStageFor := make(map[identity.ContentID][]identity.ContentID, len(byBase))
	for base, nodes := range byBase {
		geometry, baseOK := compiler.pointGeometry[base]
		if !baseOK || !geometry.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowPoint, -1, -1, CompileReasonPointUnavailable)
		}
		for nodeIndex, node := range nodes {
			if node.Point() != base {
				if _, duplicate := compiler.pointGeometry[node.Point()]; duplicate {
					return compileFailure(CompileStageOccurrences, CompileRowPoint, -1, nodeIndex, CompileReasonPointUnavailable)
				}
				compiler.pointGeometry[node.Point()] = pointDraft{id: node.Point(), decisionScope: geometry.decisionScope}
				stageFor[base] = append(stageFor[base], node.Point())
			}
			route, routed := routedStageRoute(node)
			if !routed {
				linearNodes[base] = append(linearNodes[base], node)
				if node.Point() != base {
					linearFor[base] = append(linearFor[base], node.Point())
				}
				continue
			}
			if !route.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, nodeIndex, CompileReasonOccurrenceUnavailable)
			}
			routeStage[route] = append(routeStage[route], routedStagePlacement{order: node.Stage().Order(), point: node.Point()})
			if node.Point() != base {
				preStageFor[base] = append(preStageFor[base], node.Point())
			}
		}
		if len(stageFor[base]) == 0 {
			delete(stageFor, base)
		}
	}
	// One route can carry a routed stage for more than one axis. They compose
	// along the route in a stable order - each narrows its own axis, so the
	// state a route delivers is every routed stage standing on it, applied
	// once, and the route leaves from the last of them.
	for route := range routeStage {
		chain := routeStage[route]
		sort.Slice(chain, func(left, right int) bool {
			if chain[left].order != chain[right].order {
				return chain[left].order < chain[right].order
			}
			return contentIDBefore(chain[left].point, chain[right].point)
		})
		for index := 1; index < len(chain); index++ {
			if chain[index-1].point == chain[index].point {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, index, CompileReasonOccurrenceUnavailable)
			}
		}
		routeStage[route] = chain
	}
	// terminal answers where a base's own linear chain ends, which is what any
	// transfer leaving that base departs from.
	terminal := func(base identity.ContentID) identity.ContentID {
		if chain := linearFor[base]; len(chain) != 0 {
			return chain[len(chain)-1]
		}
		return base
	}
	for base, nodes := range byBase {
		for _, node := range nodes {
			route, routed := routedStageRoute(node)
			for _, edge := range node.Stage().Edges() {
				var from identity.ContentID
				var sourceOK bool
				if edge.Source == schemaissuance.StageEdgeSourceRoute {
					if !routed {
						return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
					}
					source, resolved := compiler.routeSourcePoint(route)
					predecessor, chained := routedStagePredecessor(routeStage[route], node.Point())
					if !resolved || !source.Available() || !chained {
						return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
					}
					from, sourceOK = terminal(source), true
					if predecessor.Available() {
						from = predecessor
					}
				} else {
					position := linearNodeIndex(linearNodes[base], node)
					if position < 0 {
						return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
					}
					from, sourceOK = scheduleEdgeSource(edge, position, base, linearNodes[base])
				}
				full, writes, transportOK := compiler.scheduleTransport(
					edge, node.Base(), node.Point(), compiler.issuanceSchedule,
				)
				if !sourceOK || !transportOK || !from.Available() {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
				}
				if !full && len(writes) == 0 {
					continue
				}
				if fault := compiler.localTransfer.Append(edge.Framing, from, node.Point(), full, writes...); fault.Available() {
					return CompileFailure{construction: fault}
				}
			}
		}
	}
	for index := range compiler.environment {
		edge := &compiler.environment[index]
		if edge.from == edge.to && !edge.hasMu && !edge.hasReset {
			continue
		}
		if staged, routed := routeStage[edge.route]; routed && len(staged) != 0 {
			edge.departure = staged[len(staged)-1].point
		} else if chain := linearFor[edge.from]; len(chain) != 0 {
			edge.departure = chain[len(chain)-1]
		}
	}

	stageCount := 0
	for _, stages := range stageFor {
		stageCount += len(stages)
	}
	events := make([]wtoEventDraft, 0, len(compiler.events)+stageCount)
	seenPost := make(map[identity.ContentID]struct{}, len(stageFor))
	for _, event := range compiler.events {
		if event.kind == wtoEventPoint {
			// A routed stage feeds its base, so it is visited before it. The
			// linear chain hangs off the base and is visited after, exactly as
			// an unrouted point's stages always were.
			for _, stage := range preStageFor[event.point] {
				events = append(events, wtoEventDraft{kind: wtoEventPoint, point: stage})
			}
		}
		events = append(events, event)
		if event.kind != wtoEventPoint {
			continue
		}
		if _, staged := stageFor[event.point]; !staged {
			continue
		}
		if _, duplicate := seenPost[event.point]; duplicate {
			return compileFailure(CompileStageOccurrences, CompileRowWTOEvent, -1, -1, CompileReasonEventPointRepeated)
		}
		seenPost[event.point] = struct{}{}
		for _, stage := range linearFor[event.point] {
			events = append(events, wtoEventDraft{kind: wtoEventPoint, point: stage})
		}
	}
	if len(seenPost) != len(stageFor) {
		return compileFailure(CompileStageOccurrences, CompileRowWTOEvent, -1, -1, CompileReasonEventReference)
	}
	compiler.events = events

	regionMembership := make(map[identity.ContentID]int, len(stageFor))
	for regionIndex := range compiler.regions {
		rewritten, injected, ok := rewriteRegionMembers(compiler.regions[regionIndex].members, preStageFor, linearFor)
		if !ok {
			return compileFailure(CompileStageOccurrences, CompileRowRegion, regionIndex, -1, CompileReasonRegionReference)
		}
		for _, member := range injected {
			regionMembership[member]++
		}
		compiler.regions[regionIndex].members = rewritten
	}
	for base, count := range regionMembership {
		if count > 1 || !base.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowRegion, -1, -1, CompileReasonRegionReference)
		}
	}
	return CompileFailure{}
}

func scheduleEdgeSource(edge schemaissuance.StageEdge, target int, base identity.ContentID, nodes []issuanceexecutor.Node) (identity.ContentID, bool) {
	switch edge.Source {
	case schemaissuance.StageEdgeSourceBase:
		return base, base.Available()
	case schemaissuance.StageEdgeSourcePrevious:
		if target == 0 {
			return identity.ContentID{}, false
		}
		return nodes[target-1].Point(), nodes[target-1].Point().Available()
	case schemaissuance.StageEdgeSourceStage:
		return uniqueStagePoint(edge.Stage, nodes)
	case schemaissuance.StageEdgeSourceBeforeStage:
		for index, node := range nodes {
			if node.Stage().Key() != edge.Stage {
				continue
			}
			if index == 0 {
				return base, base.Available()
			}
			return nodes[index-1].Point(), nodes[index-1].Point().Available()
		}
	}
	return identity.ContentID{}, false
}

func uniqueStagePoint(stage schema.Key, nodes []issuanceexecutor.Node) (identity.ContentID, bool) {
	var point identity.ContentID
	for _, node := range nodes {
		if node.Stage().Key() != stage {
			continue
		}
		if point.Available() {
			return identity.ContentID{}, false
		}
		point = node.Point()
	}
	return point, point.Available()
}

func (compiler *compiler) scheduleTransport(
	edge schemaissuance.StageEdge,
	targetBase identity.ContentID,
	targetPoint identity.ContentID,
	schedule issuanceexecutor.Schedule,
) (bool, []schema.Key, bool) {
	switch edge.Transport {
	case schemaissuance.StageTransportAll:
		return true, nil, true
	case schemaissuance.StageTransportAllExceptTargetWrites:
		excluded, found := schedule.PointWriters(targetPoint)
		if !found || len(excluded) == 0 {
			return false, nil, false
		}
		excludedSet := make(map[schema.Key]struct{}, len(excluded))
		for _, axis := range excluded {
			excludedSet[axis] = struct{}{}
		}
		var writes []schema.Key
		for _, axis := range compiler.issuance.Axes() {
			if _, skip := excludedSet[axis]; !skip {
				writes = append(writes, axis)
			}
		}
		return false, writes, true
	case schemaissuance.StageTransportAllExceptWritesOfStages:
		if !targetBase.Available() {
			return false, nil, false
		}
		excludedSet := make(map[schema.Key]struct{})
		for _, stage := range edge.WriterStages {
			writers, ok := schedule.StageWriters(targetBase, stage)
			if !ok {
				return false, nil, false
			}
			for _, axis := range writers {
				excludedSet[axis] = struct{}{}
			}
		}
		var writes []schema.Key
		for _, axis := range compiler.issuance.Axes() {
			if _, skip := excludedSet[axis]; !skip {
				writes = append(writes, axis)
			}
		}
		return false, writes, true
	case schemaissuance.StageTransportWritesOfStages:
		if !targetBase.Available() || !targetPoint.Available() {
			return false, nil, false
		}
		set := make(map[schema.Key]struct{})
		for _, stage := range edge.WriterStages {
			writers, ok := schedule.StageWriters(targetBase, stage)
			if !ok {
				return false, nil, false
			}
			for _, axis := range writers {
				set[axis] = struct{}{}
			}
		}
		return false, orderedAxes(compiler.issuance.Axes(), set), true
	default:
		return false, nil, false
	}
}

func orderedAxes(order []schema.Key, selected map[schema.Key]struct{}) []schema.Key {
	result := make([]schema.Key, 0, len(selected))
	for _, axis := range order {
		if _, ok := selected[axis]; ok {
			result = append(result, axis)
		}
	}
	return result
}

// rewriteRegionMembers injects a point's stages around it in the exact order
// the schedule visits them: the stages that feed the point come before it, and
// the stages that hang off it come after. A region's member order and its
// event order are the same statement, so both are produced here from one walk.
func rewriteRegionMembers(members []identity.ContentID, preStageFor, stageFor map[identity.ContentID][]identity.ContentID) ([]identity.ContentID, []identity.ContentID, bool) {
	additional := 0
	for _, member := range members {
		count := len(preStageFor[member]) + len(stageFor[member])
		if count > int(^uint(0)>>1)-additional {
			return nil, nil, false
		}
		additional += count
	}
	if additional > int(^uint(0)>>1)-len(members) {
		return nil, nil, false
	}
	rewritten := make([]identity.ContentID, 0, len(members)+additional)
	var injected []identity.ContentID
	for _, member := range members {
		pre, post := preStageFor[member], stageFor[member]
		rewritten = append(rewritten, pre...)
		rewritten = append(rewritten, member)
		rewritten = append(rewritten, post...)
		if len(pre) != 0 || len(post) != 0 {
			injected = append(injected, member)
		}
	}
	return rewritten, injected, true
}

// routedStageRoute answers the route a scheduled stage stands on, and whether
// it stands on one at all. A routed stage is identified by its own declaration
// - it carries a route in its identity - never by its key or its axis.
func routedStageRoute(node issuanceexecutor.Node) (identity.ContentID, bool) {
	stage := node.Stage()
	if stage == nil {
		return identity.ContentID{}, false
	}
	routed := false
	for _, edge := range stage.Edges() {
		if edge.Source == schemaissuance.StageEdgeSourceRoute {
			routed = true
			break
		}
	}
	if !routed {
		return identity.ContentID{}, false
	}
	return node.Route()
}

// linearNodeIndex is the node's position in its base's linear chain, which is
// what a Previous edge counts against once routed stages have left the chain.
func linearNodeIndex(nodes []issuanceexecutor.Node, node issuanceexecutor.Node) int {
	for index := range nodes {
		if nodes[index].Point() == node.Point() {
			return index
		}
	}
	return -1
}

// routedStagePlacement is one routed stage's position on its route.
type routedStagePlacement struct {
	order uint16
	point identity.ContentID
}

// routedStagePredecessor answers the routed stage standing immediately before
// this one on the same route. The first stage on a route has none, and takes
// the route's source instead.
func routedStagePredecessor(chain []routedStagePlacement, point identity.ContentID) (identity.ContentID, bool) {
	for index := range chain {
		if chain[index].point != point {
			continue
		}
		if index == 0 {
			return identity.ContentID{}, true
		}
		return chain[index-1].point, true
	}
	return identity.ContentID{}, false
}

// routeDeparture answers the point a route's transfer leaves from.
//
// A route whose destination carries routed stages leaves from the last of
// them: those stages exist to prove something about this route in particular,
// so the state it delivers is the state they proved, and a departure that
// skipped them would carry the unproved state onto the destination while the
// stages sat beside the transfer proving nothing anyone reads. Every other
// route leaves from the end of its source's own chain, which is where that
// point finished. A source with no chain departs from itself.
func routeDeparture(route, source identity.ContentID, routeStage map[identity.ContentID][]routedStagePlacement, linearFor map[identity.ContentID][]identity.ContentID) identity.ContentID {
	if staged := routeStage[route]; len(staged) != 0 {
		return staged[len(staged)-1].point
	}
	if chain := linearFor[source]; len(chain) != 0 {
		return chain[len(chain)-1]
	}
	return identity.ContentID{}
}
