package compiler

import (
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
	stageFor := make(map[identity.ContentID][]identity.ContentID, len(byBase))
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
			for _, edge := range node.Stage().Edges() {
				from, sourceOK := scheduleEdgeSource(edge, nodeIndex, base, nodes)
				full, writes, transportOK := compiler.scheduleTransport(
					edge, node.Base(), node.Point(), compiler.issuanceSchedule,
				)
				if !sourceOK || !transportOK {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, nodeIndex, CompileReasonOccurrenceUnavailable)
				}
				if !full && len(writes) == 0 {
					continue
				}
				if fault := compiler.localTransfer.Append(edge.Framing, from, node.Point(), full, writes...); fault.Available() {
					return CompileFailure{construction: fault}
				}
			}
		}
		if len(stageFor[base]) == 0 {
			delete(stageFor, base)
		}
	}
	for index := range compiler.environment {
		edge := &compiler.environment[index]
		if edge.from == edge.to && !edge.hasMu && !edge.hasReset {
			continue
		}
		if stages := stageFor[edge.from]; len(stages) != 0 {
			edge.from = stages[len(stages)-1]
			if !edge.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
			}
		}
	}

	stageCount := 0
	for _, stages := range stageFor {
		stageCount += len(stages)
	}
	events := make([]wtoEventDraft, 0, len(compiler.events)+stageCount)
	seenPost := make(map[identity.ContentID]struct{}, len(stageFor))
	for _, event := range compiler.events {
		events = append(events, event)
		if event.kind != wtoEventPoint {
			continue
		}
		stages, staged := stageFor[event.point]
		if !staged {
			continue
		}
		if _, duplicate := seenPost[event.point]; duplicate {
			return compileFailure(CompileStageOccurrences, CompileRowWTOEvent, -1, -1, CompileReasonEventPointRepeated)
		}
		seenPost[event.point] = struct{}{}
		for _, stage := range stages {
			events = append(events, wtoEventDraft{kind: wtoEventPoint, point: stage})
		}
	}
	if len(seenPost) != len(stageFor) {
		return compileFailure(CompileStageOccurrences, CompileRowWTOEvent, -1, -1, CompileReasonEventReference)
	}
	compiler.events = events

	regionMembership := make(map[identity.ContentID]int, len(stageFor))
	for regionIndex := range compiler.regions {
		rewritten, injected, ok := rewriteRegionMembers(compiler.regions[regionIndex].members, stageFor)
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

func rewriteRegionMembers(members []identity.ContentID, stageFor map[identity.ContentID][]identity.ContentID) ([]identity.ContentID, []identity.ContentID, bool) {
	additional := 0
	for _, member := range members {
		if stages := stageFor[member]; len(stages) != 0 {
			if len(stages) > int(^uint(0)>>1)-additional {
				return nil, nil, false
			}
			additional += len(stages)
		}
	}
	if additional > int(^uint(0)>>1)-len(members) {
		return nil, nil, false
	}
	rewritten := make([]identity.ContentID, 0, len(members)+additional)
	var injected []identity.ContentID
	for _, member := range members {
		rewritten = append(rewritten, member)
		if stages := stageFor[member]; len(stages) != 0 {
			rewritten = append(rewritten, stages...)
			injected = append(injected, member)
		}
	}
	return rewritten, injected, true
}
