package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

type computationStage struct {
	base       identity.ContentID
	point      identity.ContentID
	occurrence identity.ContentID
	key        schema.Key
	left       identity.ContentID
	right      identity.ContentID
}

func (stage computationStage) available() bool {
	return stage.base.Available() && stage.point.Available() && stage.point != stage.base && stage.occurrence.Available() &&
		stage.key.Available() && stage.left.Available() && stage.right.Available()
}

func (compiler *compiler) localComputationStage(base identity.ContentID, key schema.Key, occurrence, left, right identity.ContentID) (identity.ContentID, bool) {
	if compiler == nil || compiler.computationStages == nil || !base.Available() || !occurrence.Available() ||
		!key.Available() || !left.Available() || !right.Available() {
		return identity.ContentID{}, false
	}
	framing, framingOK := compiler.issuance.formFraming(IssuanceFormComputation)
	if !framingOK {
		return identity.ContentID{}, false
	}
	point := digest(framing, artifactFormat(), bytesField(base), keyField(key), bytesField(occurrence))
	stage := computationStage{base: base, point: point, occurrence: occurrence, key: key, left: left, right: right}
	if !stage.available() {
		return identity.ContentID{}, false
	}
	for _, prior := range compiler.computationStages[base] {
		if prior.point == point || prior.occurrence == occurrence {
			return identity.ContentID{}, false
		}
	}
	compiler.computationStages[base] = append(compiler.computationStages[base], stage)
	return point, true
}

// orderedLocalComputations closes the Program-local primitive dependency
// graph without consulting Link coordinates. Binary operands and binary
// results share the same Program semantic span identity, so nested
// computations induce exact edges. Unrelated ready rows use their stable
// stage identity only as a canonical serialization order.
func (compiler *compiler) orderedLocalComputations(base identity.ContentID) ([]computationStage, bool) {
	if compiler == nil || !base.Available() {
		return nil, false
	}
	rows := compiler.computationStages[base]
	if len(rows) == 0 {
		return nil, true
	}
	producer := make(map[identity.ContentID]int, len(rows))
	pointIndex := make(map[identity.ContentID]int, len(rows))
	for index, row := range rows {
		if !row.available() || row.base != base {
			return nil, false
		}
		if _, duplicate := producer[row.occurrence]; duplicate {
			return nil, false
		}
		if _, duplicate := pointIndex[row.point]; duplicate {
			return nil, false
		}
		producer[row.occurrence] = index
		pointIndex[row.point] = index
	}
	ordered := make([]computationStage, 0, len(rows))
	placed := make([]bool, len(rows))
	for len(ordered) < len(rows) {
		ready := make([]identity.ContentID, 0, len(rows)-len(ordered))
		for index, row := range rows {
			if placed[index] {
				continue
			}
			blocked := false
			for _, input := range [...]identity.ContentID{row.left, row.right} {
				if dependency, found := producer[input]; found && !placed[dependency] {
					blocked = true
					break
				}
			}
			if !blocked {
				ready = append(ready, row.point)
			}
		}
		if len(ready) == 0 {
			return nil, false
		}
		identity.SortContentIDs(ready)
		index := pointIndex[ready[0]]
		placed[index] = true
		ordered = append(ordered, rows[index])
	}
	return ordered, true
}

type callStageSet struct {
	dispatch identity.ContentID
	summary  identity.ContentID
	effect   identity.ContentID
}

func (stages callStageSet) available(base identity.ContentID) bool {
	return base.Available() && stages.dispatch.Available() && stages.summary.Available() && stages.effect.Available() &&
		stages.dispatch != base && stages.summary != base && stages.effect != base &&
		stages.dispatch != stages.summary && stages.dispatch != stages.effect && stages.summary != stages.effect
}

func (compiler *compiler) callStage(base identity.ContentID) (callStageSet, bool) {
	if compiler == nil || compiler.callStages == nil || !base.Available() {
		return callStageSet{}, false
	}
	if stages := compiler.callStages[base]; stages.available(base) {
		return stages, true
	}
	if _, known := compiler.pointGeometry[base]; !known {
		return callStageSet{}, false
	}
	dispatch, dispatchOK := compiler.issuance.stageFraming(programschema.RuleStageCallDispatch)
	summary, summaryOK := compiler.issuance.stageFraming(programschema.RuleStageCallSummary)
	effect, effectOK := compiler.issuance.stageFraming(programschema.RuleStageCallEffect)
	if !dispatchOK || !summaryOK || !effectOK {
		return callStageSet{}, false
	}
	stages := callStageSet{
		dispatch: digest(dispatch, artifactFormat(), bytesField(base)),
		summary:  digest(summary, artifactFormat(), bytesField(base)),
		effect:   digest(effect, artifactFormat(), bytesField(base)),
	}
	if !stages.available(base) {
		return callStageSet{}, false
	}
	compiler.callStages[base] = stages
	return stages, true
}

// reusableStage raises the memoized cut one placement form names over a base.
// The local-family cuts differ only in the memo they occupy and the framing
// their form declares, so they are one constructor.
func (compiler *compiler) reusableStage(memo map[identity.ContentID]identity.ContentID, form IssuanceForm, base identity.ContentID) (identity.ContentID, bool) {
	if compiler == nil || memo == nil || !base.Available() {
		return identity.ContentID{}, false
	}
	if stage := memo[base]; stage.Available() {
		return stage, true
	}
	framing, framingOK := compiler.issuance.formFraming(form)
	if _, known := compiler.pointGeometry[base]; !known || !framingOK {
		return identity.ContentID{}, false
	}
	stage := digest(framing, artifactFormat(), bytesField(base))
	if !stage.Available() || stage == base {
		return identity.ContentID{}, false
	}
	memo[base] = stage
	return stage, true
}

func (compiler *compiler) localStage(base identity.ContentID) (identity.ContentID, bool) {
	if compiler == nil {
		return identity.ContentID{}, false
	}
	return compiler.reusableStage(compiler.localStages, IssuanceFormLocal, base)
}

// predecessorStage is the Program-owned execution cut for a routed strong
// write. It is distinct from the ordinary local cut because rules at one point
// all read one immutable incoming state.
func (compiler *compiler) predecessorStage(base identity.ContentID) (identity.ContentID, bool) {
	if compiler == nil {
		return identity.ContentID{}, false
	}
	return compiler.reusableStage(compiler.predecessorStages, IssuanceFormLocalPredecessor, base)
}

// installLocalStagesFailure splices every reusable synthetic execution cut
// into the exact Program WTO stream. A route-specific entry refinement is a
// forward overlay: its guarded ingress targets the stage and Program-issued
// successor continuations depart that stage. It never merges back into the
// base, because base→stage→base would fabricate a recurrence.
func (compiler *compiler) installLocalStagesFailure() CompileFailure {
	if len(compiler.predecessorStages) == 0 && len(compiler.localStages) == 0 && len(compiler.computationStages) == 0 && len(compiler.callStages) == 0 {
		return CompileFailure{}
	}
	baseSet := make(map[identity.ContentID]struct{}, len(compiler.predecessorStages)+len(compiler.localStages)+len(compiler.computationStages)+len(compiler.callStages))
	for base := range compiler.predecessorStages {
		baseSet[base] = struct{}{}
	}
	for base := range compiler.localStages {
		baseSet[base] = struct{}{}
	}
	for base := range compiler.computationStages {
		baseSet[base] = struct{}{}
	}
	for base := range compiler.callStages {
		baseSet[base] = struct{}{}
	}
	bases := make([]identity.ContentID, 0, len(baseSet))
	for base := range baseSet {
		bases = append(bases, base)
	}
	identity.SortContentIDs(bases)
	var callTransport callStageTransport
	if len(compiler.callStages) != 0 {
		plan, planOK := compiler.issuance.callStageTransport()
		if !planOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
		}
		callTransport = plan
	}
	stageFor := make(map[identity.ContentID][]identity.ContentID, len(bases))
	computationInput := make(map[identity.ContentID]identity.ContentID)
	callInput := make(map[identity.ContentID]identity.ContentID)
	localPredecessorInput := make(map[identity.ContentID]identity.ContentID)
	appendTransfer := func(domain string, from, to identity.ContentID, full bool, writes ...schema.Key) bool {
		ordered, orderedOK := orderedWrites(writes)
		if !orderedOK {
			return false
		}
		fields := []field{bytesField(from), bytesField(to), boolField(full), uintField(uint64(len(ordered)))}
		for _, write := range ordered {
			fields = append(fields, keyField(write))
		}
		edge := localTransferDraft{id: digest(domain, artifactFormat(), fields...), from: from, to: to, full: full, writes: ordered}
		if !edge.Available() {
			return false
		}
		compiler.localTransfers = append(compiler.localTransfers, edge)
		return true
	}
	for index, base := range bases {
		geometry, baseOK := compiler.pointGeometry[base]
		if !baseOK || !geometry.Available() {
			return compileFailure(CompileStageOccurrences, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
		}
		sequence := make([]identity.ContentID, 0, 6)
		predecessor := base
		if stage := compiler.predecessorStages[base]; stage.Available() {
			written := make(map[schema.Key]struct{})
			for _, placement := range compiler.ruleOccurrences {
				if placement.PointID() != stage || placement.InputKind() != programschema.RuleInputPredecessor {
					continue
				}
				axis, axisOK := compiler.issuance.writesFor(placement.Key())
				if !axisOK {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
				}
				written[axis] = struct{}{}
			}
			transport, transportOK := compiler.issuance.transportKeysExcept(written)
			if len(written) == 0 || !transportOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			sequence = append(sequence, stage)
			if len(transport) != 0 && !appendTransfer("analysis/program-artifact/local-predecessor-transfer", base, stage, false, transport...) {
				return compileFailure(CompileStageOccurrences, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
			}
			predecessor = stage
		}
		if local := compiler.localStages[base]; local.Available() {
			sequence = append(sequence, local)
			if !appendTransfer("analysis/program-artifact/local-transfer", predecessor, local, true) {
				return compileFailure(CompileStageOccurrences, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
			}
			if predecessor != base {
				localPredecessorInput[local] = predecessor
			}
			predecessor = local
		}
		computations, computationsOK := compiler.orderedLocalComputations(base)
		if !computationsOK {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		for _, computation := range computations {
			sequence = append(sequence, computation.point)
			if !appendTransfer("analysis/program-artifact/local-computation-transfer", predecessor, computation.point, true) {
				return compileFailure(CompileStageOccurrences, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
			}
			computationInput[computation.point] = predecessor
			predecessor = computation.point
		}
		callBase := predecessor
		if stages := compiler.callStages[base]; stages.available(base) {
			sequence = append(sequence, stages.dispatch, stages.summary, stages.effect)
			if !appendTransfer("analysis/program-artifact/call-base-dispatch-transfer", callBase, stages.dispatch, false, callTransport.dispatchEntry...) ||
				!appendTransfer("analysis/program-artifact/call-base-summary-transfer", callBase, stages.summary, false, callTransport.effectBypass...) ||
				!appendTransfer("analysis/program-artifact/call-dispatch-summary-transfer", stages.dispatch, stages.summary, false, callTransport.dispatchForward...) ||
				!appendTransfer("analysis/program-artifact/call-base-effect-transfer", callBase, stages.effect, true) ||
				!appendTransfer("analysis/program-artifact/call-dispatch-effect-transfer", stages.dispatch, stages.effect, false, callTransport.dispatchForward...) {
				return compileFailure(CompileStageOccurrences, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
			}
			callInput[stages.dispatch] = callBase
		}
		for _, stage := range sequence {
			if !stage.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
			}
			if _, duplicate := compiler.pointGeometry[stage]; duplicate {
				return compileFailure(CompileStageOccurrences, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
			}
			compiler.pointGeometry[stage] = pointDraft{id: stage, decisions: append([]identity.ContentID(nil), geometry.decisions...)}
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
		compiler.environmentByRoute = make(map[identity.ContentID]environmentEdgeDraft, len(compiler.environment))
	} else {
		clear(compiler.environmentByRoute)
	}
	if compiler.environmentRouteDuplicates == nil {
		compiler.environmentRouteDuplicates = make(map[identity.ContentID]struct{}, len(compiler.environment))
	} else {
		clear(compiler.environmentRouteDuplicates)
	}
	for _, edge := range compiler.environment {
		if _, duplicate := compiler.environmentByRoute[edge.route]; duplicate {
			compiler.environmentRouteDuplicates[edge.route] = struct{}{}
		} else {
			compiler.environmentByRoute[edge.route] = edge
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
			edge, found := compiler.environmentByRoute[route]
			if !routeOK || !found || !edge.to.Available() || edge.to == point {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			if !compiler.replaceRuleOccurrenceInput(index, edge.to) {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			continue
		}
		if input, computation := computationInput[point]; computation {
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
		local := compiler.localStages[base]
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
		rewritten := make([]identity.ContentID, 0, len(members)+len(stageFor))
		for _, member := range members {
			rewritten = append(rewritten, member)
			injected := false
			if stages, staged := stageFor[member]; staged {
				rewritten = append(rewritten, stages...)
				injected = true
			}
			if injected {
				regionMembership[member]++
			}
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
