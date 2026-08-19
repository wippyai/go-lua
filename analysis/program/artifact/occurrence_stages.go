package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
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
	point := digest("analysis/program-artifact/local-computation-stage", artifactFormat,
		bytesField(base), keyField(key), bytesField(occurrence))
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
	stages := callStageSet{
		dispatch: digest("analysis/program-artifact/call-dispatch-stage", artifactFormat, bytesField(base)),
		summary:  digest("analysis/program-artifact/call-summary-stage", artifactFormat, bytesField(base)),
		effect:   digest("analysis/program-artifact/call-effect-stage", artifactFormat, bytesField(base)),
	}
	if !stages.available(base) {
		return callStageSet{}, false
	}
	compiler.callStages[base] = stages
	return stages, true
}

func (compiler *compiler) localStage(base identity.ContentID) (identity.ContentID, bool) {
	if compiler == nil || compiler.localStages == nil || !base.Available() {
		return identity.ContentID{}, false
	}
	if stage := compiler.localStages[base]; stage.Available() {
		return stage, true
	}
	if _, known := compiler.pointGeometry[base]; !known {
		return identity.ContentID{}, false
	}
	stage := digest("analysis/program-artifact/local-stage", artifactFormat, bytesField(base))
	if !stage.Available() || stage == base {
		return identity.ContentID{}, false
	}
	compiler.localStages[base] = stage
	return stage, true
}

// predecessorStage is the Program-owned execution cut for a routed strong
// write. It is distinct from the ordinary local cut because rules at one point
// all read one immutable incoming state.
func (compiler *compiler) predecessorStage(base identity.ContentID) (identity.ContentID, bool) {
	if compiler == nil || compiler.predecessorStages == nil || !base.Available() {
		return identity.ContentID{}, false
	}
	if stage := compiler.predecessorStages[base]; stage.Available() {
		return stage, true
	}
	if _, known := compiler.pointGeometry[base]; !known {
		return identity.ContentID{}, false
	}
	stage := digest("analysis/program-artifact/local-predecessor-stage", artifactFormat, bytesField(base))
	if !stage.Available() || stage == base {
		return identity.ContentID{}, false
	}
	compiler.predecessorStages[base] = stage
	return stage, true
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
	stageFor := make(map[identity.ContentID][]identity.ContentID, len(bases))
	stageExit := make(map[identity.ContentID]identity.ContentID, len(bases))
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
		edge := LocalTransfer{id: digest(domain, artifactFormat, fields...), from: from, to: to, full: full, writes: ordered}
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
				if placement.point != stage || placement.inputKind != RuleInputPredecessor {
					continue
				}
				axis, axisOK := compiler.issuance.writesFor(placement.key)
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
			stageExit[base] = stage
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
			stageExit[base] = local
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
			stageExit[base] = computation.point
		}
		callBase := predecessor
		if stages := compiler.callStages[base]; stages.available(base) {
			plan, planOK := compiler.issuance.callStageTransport()
			if !planOK {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			sequence = append(sequence, stages.dispatch, stages.summary, stages.effect)
			if !appendTransfer("analysis/program-artifact/call-base-dispatch-transfer", callBase, stages.dispatch, false, plan.dispatchEntry...) ||
				!appendTransfer("analysis/program-artifact/call-base-summary-transfer", callBase, stages.summary, false, plan.effectBypass...) ||
				!appendTransfer("analysis/program-artifact/call-dispatch-summary-transfer", stages.dispatch, stages.summary, false, plan.dispatchForward...) ||
				!appendTransfer("analysis/program-artifact/call-base-effect-transfer", callBase, stages.effect, true) ||
				!appendTransfer("analysis/program-artifact/call-dispatch-effect-transfer", stages.dispatch, stages.effect, false, plan.dispatchForward...) {
				return compileFailure(CompileStageOccurrences, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
			}
			callInput[stages.dispatch] = callBase
			stageExit[base] = stages.effect
		}
		for _, stage := range sequence {
			if !stage.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
			}
			if _, duplicate := compiler.points[stage]; duplicate {
				return compileFailure(CompileStageOccurrences, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
			}
			compiler.points[stage] = struct{}{}
			compiler.pointGeometry[stage] = Point{id: stage, decisions: append([]identity.ContentID(nil), geometry.decisions...)}
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
		if exit := stageExit[edge.from]; exit.Available() {
			edge.from = exit
			if !edge.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
			}
		}
	}
	if compiler.environmentByRoute == nil {
		compiler.environmentByRoute = make(map[identity.ContentID]EnvironmentEdge, len(compiler.environment))
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
		if compiler.ruleOccurrences[index].inputKind == RuleInputNone {
			continue
		}
		if compiler.ruleOccurrences[index].inputKind == RuleInputPredecessor {
			edge, found := compiler.environmentByRoute[compiler.ruleOccurrences[index].route]
			if !found || !edge.to.Available() || edge.to == compiler.ruleOccurrences[index].point {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			compiler.ruleOccurrences[index].input = edge.to
			continue
		}
		if input, computation := computationInput[compiler.ruleOccurrences[index].point]; computation {
			if compiler.ruleOccurrences[index].inputKind != RuleInputFinish || !input.Available() || input == compiler.ruleOccurrences[index].point {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			compiler.ruleOccurrences[index].input = input
			continue
		}
		if input, dispatch := callInput[compiler.ruleOccurrences[index].point]; dispatch {
			if compiler.ruleOccurrences[index].stage != RuleStageCallDispatch || compiler.ruleOccurrences[index].inputKind != RuleInputFinish || !input.Available() || input == compiler.ruleOccurrences[index].point {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			compiler.ruleOccurrences[index].input = input
			continue
		}
		if input, separated := localPredecessorInput[compiler.ruleOccurrences[index].point]; separated {
			if !input.Available() || input == compiler.ruleOccurrences[index].point {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			compiler.ruleOccurrences[index].input = input
			continue
		}

		// A synthetic stage is an execution splice for rule inputs as well as
		// structural routes. A rule producing the base's Local stage must read
		// the original base. Call Dispatch reads that Local result when one
		// exists. Every other consumer of the staged base reads the terminal
		// stage, so no Entry/Finish rule can bypass a prior strong write.
		base := compiler.ruleOccurrences[index].input
		exit := stageExit[base]
		if !exit.Available() {
			continue
		}
		local := compiler.localStages[base]
		if local.Available() && compiler.ruleOccurrences[index].point == local {
			continue
		}
		compiler.ruleOccurrences[index].input = exit
		if !compiler.ruleOccurrences[index].Available() {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	stageCount := 0
	for _, stages := range stageFor {
		stageCount += len(stages)
	}
	events := make([]WTOEvent, 0, len(compiler.events)+stageCount)
	seenPost := make(map[identity.ContentID]struct{}, len(stageFor))
	for _, event := range compiler.events {
		events = append(events, event)
		if event.kind != WTOEventPoint {
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
			events = append(events, WTOEvent{kind: WTOEventPoint, point: stage})
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
