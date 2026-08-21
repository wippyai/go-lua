package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/localtransfer"
	stageplan "github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/stage"
	"github.com/wippyai/go-lua/analysis/program/artifact/issuance"
	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func (compiler *compiler) localComputationStage(base identity.ContentID, key schema.Key, occurrence, left, right identity.ContentID) (identity.ContentID, bool) {
	if compiler == nil || compiler.stages == nil {
		return identity.ContentID{}, false
	}
	framing, framingOK := compiler.issuance.FormFraming(issuance.FormComputation)
	if !framingOK {
		return identity.ContentID{}, false
	}
	return compiler.stages.Computation(base, framing, key, occurrence, left, right)
}

func (compiler *compiler) callStage(base identity.ContentID) (stageplan.Call, bool) {
	if compiler == nil || compiler.stages == nil || !base.Available() {
		return stageplan.Call{}, false
	}
	if _, known := compiler.pointGeometry[base]; !known {
		return stageplan.Call{}, false
	}
	dispatch, dispatchOK := compiler.issuance.StageFraming(programschema.RuleStageCallDispatch)
	summary, summaryOK := compiler.issuance.StageFraming(programschema.RuleStageCallSummary)
	effect, effectOK := compiler.issuance.StageFraming(programschema.RuleStageCallEffect)
	if !dispatchOK || !summaryOK || !effectOK {
		return stageplan.Call{}, false
	}
	return compiler.stages.Call(base, dispatch, summary, effect)
}

func (compiler *compiler) localStage(base identity.ContentID) (identity.ContentID, bool) {
	if compiler == nil {
		return identity.ContentID{}, false
	}
	if compiler.stages == nil || !base.Available() {
		return identity.ContentID{}, false
	}
	framing, framingOK := compiler.issuance.FormFraming(issuance.FormLocal)
	if _, known := compiler.pointGeometry[base]; !known || !framingOK {
		return identity.ContentID{}, false
	}
	return compiler.stages.Local(base, framing)
}

func (compiler *compiler) localSuccessorStage(base identity.ContentID) (identity.ContentID, identity.ContentID, bool) {
	if compiler == nil || compiler.stages == nil || !base.Available() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	localFraming, localFramingOK := compiler.issuance.FormFraming(issuance.FormLocal)
	successorFraming, successorFramingOK := compiler.issuance.FormFraming(issuance.FormLocalSuccessor)
	if _, known := compiler.pointGeometry[base]; !known || !localFramingOK || !successorFramingOK {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	local, localOK := compiler.stages.Local(base, localFraming)
	successor, successorOK := compiler.stages.Successor(base, successorFraming)
	return local, successor, localOK && successorOK && local != successor
}

// predecessorStage is the Program-owned execution cut for a routed strong
// write. It is distinct from the ordinary local cut because rules at one point
// all read one immutable incoming state.
func (compiler *compiler) predecessorStage(base identity.ContentID, writes schema.Key) (identity.ContentID, bool) {
	if compiler == nil || compiler.stages == nil || !base.Available() || !writes.Available() {
		return identity.ContentID{}, false
	}
	framing, framingOK := compiler.issuance.FormFraming(issuance.FormLocalPredecessor)
	if _, known := compiler.pointGeometry[base]; !known || !framingOK {
		return identity.ContentID{}, false
	}
	return compiler.stages.Predecessor(base, framing, writes)
}

// installLocalStagesFailure splices every reusable synthetic execution cut
// into the exact Program WTO stream. A route-specific entry refinement is a
// forward overlay: its guarded ingress targets the stage and Program-issued
// successor continuations depart that stage. It never merges back into the
// base, because base→stage→base would fabricate a recurrence.
func (compiler *compiler) installLocalStagesFailure() CompileFailure {
	if compiler == nil || compiler.stages == nil {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	plan, planFault := compiler.stages.Seal()
	if planFault.Failed() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	if plan.Count() == 0 {
		return CompileFailure{}
	}
	var dispatchEntry, effectEntry, effectBypass, dispatchForward, summaryForward []schema.Key
	for index := 0; index < plan.Count(); index++ {
		placement, placementOK := plan.At(index)
		if !placementOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if _, hasCall := placement.Call(); !hasCall {
			continue
		}
		var planOK bool
		dispatchEntry, effectEntry, effectBypass, dispatchForward, summaryForward, planOK = compiler.issuance.CallStageTransport()
		if !planOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
		}
		break
	}
	stageFor := make(map[identity.ContentID][]identity.ContentID, plan.Count())
	stagedInput := make(map[identity.ContentID]identity.ContentID)
	callInput := make(map[identity.ContentID]identity.ContentID)
	localPredecessorInput := make(map[identity.ContentID]identity.ContentID)
	localStageFor := make(map[identity.ContentID]identity.ContentID)
	if compiler.localTransfer == nil {
		compiler.localTransfer = localtransfer.New(artifactFormat())
	}
	for index := 0; index < plan.Count(); index++ {
		placement, placementOK := plan.At(index)
		if !placementOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		base := placement.Base()
		geometry, baseOK := compiler.pointGeometry[base]
		if !baseOK || !geometry.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
		}
		sequence := make([]identity.ContentID, 0, 6)
		predecessor := base
		if stage, staged := placement.Predecessor(); staged {
			written := make(map[schema.Key]struct{})
			for writeIndex := 0; writeIndex < placement.PredecessorWriteCount(); writeIndex++ {
				axis, axisOK := placement.PredecessorWriteAt(writeIndex)
				if !axisOK || !axis.Available() {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, writeIndex, CompileReasonOccurrenceUnavailable)
				}
				written[axis] = struct{}{}
			}
			transport, transportOK := compiler.issuance.TransportKeysExcept(written)
			if len(written) == 0 || !transportOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			sequence = append(sequence, stage)
			if len(transport) != 0 && !compiler.localTransfer.Append("analysis/program-artifact/local-predecessor-transfer", base, stage, false, transport...) {
				return compileFailure(CompileStageOccurrences, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
			}
			predecessor = stage
		}
		if local, staged := placement.Local(); staged {
			localStageFor[base] = local
			sequence = append(sequence, local)
			if !compiler.localTransfer.Append("analysis/program-artifact/local-transfer", predecessor, local, true) {
				return compileFailure(CompileStageOccurrences, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
			}
			if predecessor != base {
				localPredecessorInput[local] = predecessor
			}
			predecessor = local
		}
		if successor, staged := placement.Successor(); staged {
			if predecessor == base {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			sequence = append(sequence, successor)
			if !compiler.localTransfer.Append("analysis/program-artifact/local-successor-transfer", predecessor, successor, true) {
				return compileFailure(CompileStageOccurrences, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
			}
			stagedInput[successor] = predecessor
			predecessor = successor
		}
		for computationIndex := 0; computationIndex < placement.ComputationCount(); computationIndex++ {
			computation, computationOK := placement.ComputationAt(computationIndex)
			point := computation.Point()
			if !computationOK || !point.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, computationIndex, CompileReasonOccurrenceUnavailable)
			}
			sequence = append(sequence, point)
			if !compiler.localTransfer.Append("analysis/program-artifact/local-computation-transfer", predecessor, point, true) {
				return compileFailure(CompileStageOccurrences, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
			}
			stagedInput[point] = predecessor
			predecessor = point
		}
		callBase := predecessor
		if calls, staged := placement.Call(); staged {
			dispatch, summary, effect := calls.Dispatch(), calls.Summary(), calls.Effect()
			sequence = append(sequence, dispatch, summary, effect)
			if !compiler.localTransfer.Append("analysis/program-artifact/call-base-dispatch-transfer", callBase, dispatch, false, dispatchEntry...) ||
				!compiler.localTransfer.Append("analysis/program-artifact/call-base-summary-transfer", callBase, summary, false, effectBypass...) ||
				!compiler.localTransfer.Append("analysis/program-artifact/call-dispatch-summary-transfer", dispatch, summary, false, dispatchForward...) ||
				!compiler.localTransfer.Append("analysis/program-artifact/call-base-effect-transfer", callBase, effect, false, effectEntry...) ||
				!compiler.localTransfer.Append("analysis/program-artifact/call-dispatch-effect-transfer", dispatch, effect, false, dispatchForward...) ||
				!compiler.localTransfer.Append("analysis/program-artifact/call-summary-effect-transfer", summary, effect, false, summaryForward...) {
				return compileFailure(CompileStageOccurrences, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
			}
			callInput[dispatch] = callBase
		}
		for _, stage := range sequence {
			if !stage.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
			}
			if _, duplicate := compiler.pointGeometry[stage]; duplicate {
				return compileFailure(CompileStageOccurrences, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
			}
			compiler.pointGeometry[stage] = pointDraft{id: stage, decisionScope: geometry.decisionScope}
		}
		stageFor[base] = sequence
	}

	// Ordinary Local/Call stages replace their base's outgoing route source.
	// There is no route-local continuation overlay here: an unproved control
	// bridge must never be replayed through a shared base point.
	originalCount := len(compiler.environment)
	for index := 0; index < originalCount; index++ {
		edge := &compiler.environment[index]
		// A Mu or reset witness is the sole proof that a same-point route is a
		// downstream control successor. Without one the route keeps its base
		// source, so the rule reads the pre-write environment instead of its
		// own Local output; staging the source would merge stage back into
		// base and fabricate a recurrence the parent never issued.
		if edge.from == edge.to && !edge.hasMu && !edge.hasReset {
			continue
		}
		stages := stageFor[edge.from]
		if len(stages) != 0 {
			exit := stages[len(stages)-1]
			edge.from = exit
			if !edge.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
			}
		}
	}
	if compiler.environmentByRoute == nil {
		compiler.environmentByRoute = make(map[identity.ContentID]environmentRouteIndex, len(compiler.environment))
	} else {
		clear(compiler.environmentByRoute)
	}
	for edgeOrdinal := range compiler.environment {
		edge := compiler.environment[edgeOrdinal]
		if !recordEnvironmentRoute(compiler.environmentByRoute, edge.route, edgeOrdinal, len(compiler.environment)) {
			return compileFailure(CompileStageOccurrences, CompileRowEnvironment, edgeOrdinal, -1, CompileReasonEnvironmentUnavailable)
		}
	}
	for index := range compiler.ruleOccurrences {
		placement := compiler.ruleOccurrences[index]
		inputKind := placement.InputKind()
		point := placement.PointID()
		if inputKind == programschema.RuleInputNone {
			continue
		}
		if inputKind == programschema.RuleInputPredecessor {
			route, routeOK := placement.PredecessorRouteID()
			routeIndex, found := compiler.environmentByRoute[route]
			edgeOrdinal, unique := routeIndex.uniqueAt(len(compiler.environment))
			if !routeOK || !found || !unique {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			edge := compiler.environment[edgeOrdinal]
			expectedID := environmentRouteOccurrenceID(compiler.input.ContentID(), route, edge.arm)
			if !edge.Available() || edge.route != route || edge.id != expectedID || !edge.to.Available() || edge.to == point {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			if !compiler.replaceRuleOccurrenceInput(index, edge.to) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			continue
		}
		if input, staged := stagedInput[point]; staged {
			if inputKind != programschema.RuleInputFinish || !input.Available() || input == point {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			if !compiler.replaceRuleOccurrenceInput(index, input) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			continue
		}
		if input, dispatch := callInput[point]; dispatch {
			if placement.Stage() != programschema.RuleStageCallDispatch || inputKind != programschema.RuleInputFinish || !input.Available() || input == point {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			if !compiler.replaceRuleOccurrenceInput(index, input) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			continue
		}
		if input, separated := localPredecessorInput[point]; separated {
			if !input.Available() || input == point {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			if !compiler.replaceRuleOccurrenceInput(index, input) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			continue
		}

		// A synthetic stage is an execution splice for rule inputs as well as
		// structural routes. A rule producing the base's Local stage must read
		// the original base. Call Dispatch reads that Local result when one
		// exists. Every other consumer of the staged base reads the terminal
		// stage, so no Entry/Finish rule can bypass a prior strong write.
		base, baseOK := placement.InputPoint()
		if !baseOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		stages := stageFor[base]
		if len(stages) == 0 {
			continue
		}
		exit := stages[len(stages)-1]
		local := localStageFor[base]
		if local.Available() && point == local {
			continue
		}
		if !compiler.replaceRuleOccurrenceInput(index, exit) || !compiler.ruleOccurrences[index].Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
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
		members := compiler.regions[regionIndex].members
		rewritten, injected, rewriteOK := rewriteRegionMembers(members, stageFor)
		if !rewriteOK {
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

// rewriteRegionMembers inserts only the synthetic stages that belong to one
// region. Its capacity is derived from that region's actual injected stages,
// never from the global stage directory.
func rewriteRegionMembers(members []identity.ContentID, stageFor map[identity.ContentID][]identity.ContentID) ([]identity.ContentID, []identity.ContentID, bool) {
	additional := 0
	for _, member := range members {
		stages, staged := stageFor[member]
		if !staged {
			continue
		}
		if len(stages) > int(^uint(0)>>1)-additional {
			return nil, nil, false
		}
		additional += len(stages)
	}
	if additional > int(^uint(0)>>1)-len(members) {
		return nil, nil, false
	}
	rewritten := make([]identity.ContentID, 0, len(members)+additional)
	injected := make([]identity.ContentID, 0)
	for _, member := range members {
		rewritten = append(rewritten, member)
		if stages, staged := stageFor[member]; staged {
			rewritten = append(rewritten, stages...)
			injected = append(injected, member)
		}
	}
	return rewritten, injected, true
}
