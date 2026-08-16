package artifact

import "github.com/wippyai/go-lua/analysis/identity"

type computationStage struct {
	base       identity.ContentID
	point      identity.ContentID
	occurrence identity.ContentID
	role       RuleRole
	left       identity.ContentID
	right      identity.ContentID
}

func (stage computationStage) available() bool {
	return stage.base.Available() && stage.point.Available() && stage.point != stage.base && stage.occurrence.Available() &&
		(stage.role == RuleRoleValueBinaryArithmetic || stage.role == RuleRoleValueBinaryEquality || stage.role == RuleRoleValueBinaryOrder) &&
		stage.left.Available() && stage.right.Available()
}

func (compiler *compiler) localComputationStage(base identity.ContentID, role RuleRole, occurrence, left, right identity.ContentID) (identity.ContentID, bool) {
	if compiler == nil || compiler.computationStages == nil || !base.Available() || !occurrence.Available() ||
		(role != RuleRoleValueBinaryArithmetic && role != RuleRoleValueBinaryEquality && role != RuleRoleValueBinaryOrder) || !left.Available() || !right.Available() {
		return identity.ContentID{}, false
	}
	point := digest("analysis/program-artifact/local-computation-stage", artifactFormat,
		bytesField(base), uintField(uint64(role)), bytesField(occurrence))
	stage := computationStage{base: base, point: point, occurrence: occurrence, role: role, left: left, right: right}
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

// installLocalStagesFailure splices every reusable synthetic execution cut
// into the exact Program WTO stream. A route-specific entry refinement is a
// forward overlay: its guarded ingress targets the stage and Program-issued
// successor continuations depart that stage. It never merges back into the
// base, because base→stage→base would fabricate a recurrence.
func (compiler *compiler) installLocalStagesFailure() CompileFailure {
	if len(compiler.localStages) == 0 && len(compiler.computationStages) == 0 && len(compiler.callStages) == 0 {
		return CompileFailure{}
	}
	baseSet := make(map[identity.ContentID]struct{}, len(compiler.localStages)+len(compiler.computationStages)+len(compiler.callStages))
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
	appendTransfer := func(domain string, from, to identity.ContentID, full bool, roles ...RuleRole) bool {
		fields := []field{bytesField(from), bytesField(to), boolField(full), uintField(uint64(len(roles)))}
		for _, role := range roles {
			fields = append(fields, uintField(uint64(role)))
		}
		edge := LocalTransfer{id: digest(domain, artifactFormat, fields...), from: from, to: to, full: full, roles: append([]RuleRole(nil), roles...)}
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
		sequence := make([]identity.ContentID, 0, 5)
		predecessor := base
		if local := compiler.localStages[base]; local.Available() {
			sequence = append(sequence, local)
			if !appendTransfer("analysis/program-artifact/local-transfer", base, local, true) {
				return compileFailure(CompileStageOccurrences, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
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
			sequence = append(sequence, stages.dispatch, stages.summary, stages.effect)
			if !appendTransfer("analysis/program-artifact/call-base-dispatch-transfer", callBase, stages.dispatch, false, RuleRoleValueSource, RuleRolePackSource, RuleRoleHeapIngress, RuleRoleCallDispatch) ||
				!appendTransfer("analysis/program-artifact/call-base-summary-transfer", callBase, stages.summary, false, RuleRoleEffectSelected) ||
				!appendTransfer("analysis/program-artifact/call-dispatch-summary-transfer", stages.dispatch, stages.summary, false, RuleRoleCallDispatch) ||
				!appendTransfer("analysis/program-artifact/call-base-effect-transfer", callBase, stages.effect, true) ||
				!appendTransfer("analysis/program-artifact/call-dispatch-effect-transfer", stages.dispatch, stages.effect, false, RuleRoleCallDispatch) {
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

	// The parent issues one commit route per write-shaped occurrence, recorded
	// by recordOccurrencePredecessor while the Program proof is live. That set
	// is the exact predecessor-boundary fact, so a commit route keeps its
	// boundary regardless of the cyclic region it was issued inside.
	commitRoutes := make(map[identity.ContentID]struct{}, len(compiler.occurrenceSpans))
	for _, geometry := range compiler.occurrenceSpans {
		if geometry.route.Available() {
			commitRoutes[geometry.route] = struct{}{}
		}
	}

	// Ordinary Local/Call stages replace their base's outgoing route source.
	// There is no route-local continuation overlay here: an unproved control
	// bridge must never be replayed through a shared base point.
	originalCount := len(compiler.environment)
	for index := 0; index < originalCount; index++ {
		edge := &compiler.environment[index]
		// A same-point route stays base -> base when it is proved not to be a
		// downstream control successor: the parent recorded it as a write
		// commit boundary, or it lies outside every cyclic region, where no
		// route can be a back edge. Either way the rule reads the pre-write
		// environment instead of its own Local output. A Mu/reset witness is a
		// proved recurrence, so such a route keeps the staged source.
		if edge.from == edge.to && !edge.hasMu && !edge.hasReset {
			_, commit := commitRoutes[edge.route]
			if commit || !edge.component.Available() {
				continue
			}
		}
		if exit := stageExit[edge.from]; exit.Available() {
			edge.from = exit
			if !edge.Available() {
				return compileFailure(CompileStageOccurrences, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
			}
		}
	}
	compiler.environmentByRoute = make(map[identity.ContentID]EnvironmentEdge, len(compiler.environment))
	compiler.environmentRouteDuplicates = make(map[identity.ContentID]struct{})
	for _, edge := range compiler.environment {
		if _, duplicate := compiler.environmentByRoute[edge.route]; duplicate {
			compiler.environmentRouteDuplicates[edge.route] = struct{}{}
		} else {
			compiler.environmentByRoute[edge.route] = edge
		}
	}
	for role, rows := range compiler.ruleOccurrences {
		for index := range rows {
			if rows[index].inputKind == RuleInputNone {
				continue
			}
			if rows[index].inputKind == RuleInputPredecessor {
				edge, found := compiler.environmentByRoute[rows[index].route]
				if !found || !edge.from.Available() {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
				}
				rows[index].input = edge.from
				continue
			}
			if input, computation := computationInput[rows[index].point]; computation {
				if rows[index].inputKind != RuleInputFinish || !input.Available() || input == rows[index].point {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
				}
				rows[index].input = input
				continue
			}
			if input, dispatch := callInput[rows[index].point]; dispatch {
				if rows[index].stage != RuleStageCallDispatch || rows[index].inputKind != RuleInputFinish || !input.Available() || input == rows[index].point {
					return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
				}
				rows[index].input = input
				continue
			}

			// A synthetic stage is an execution splice for rule inputs as well as
			// structural routes. A rule producing the base's Local stage must read
			// the original base. Call Dispatch reads that Local result when one
			// exists. Every other consumer of the staged base reads the terminal
			// stage, so no Entry/Finish rule can bypass a prior strong write.
			base := rows[index].input
			exit := stageExit[base]
			if !exit.Available() {
				continue
			}
			local := compiler.localStages[base]
			if local.Available() && rows[index].point == local {
				continue
			}
			rows[index].input = exit
			if !rows[index].Available() {
				return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
		}
		compiler.ruleOccurrences[role] = rows
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
