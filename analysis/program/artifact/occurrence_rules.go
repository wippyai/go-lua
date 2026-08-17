package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
)

func (compiler *compiler) deriveRuleOccurrencesFailure() CompileFailure {
	compiler.ruleOccurrences = make(map[RuleRole][]RuleOccurrence)
	for index, row := range compiler.occurrences {
		if uint64(index) > uint64(^uint32(0)) {
			compiler.ruleOccurrences = nil
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		ordinal := uint32(index)
		geometry := compiler.occurrenceSpans[occurrenceLookup{kind: row.kind, id: row.id}]
		finish := geometry.finish
		if len(finish) == 0 {
			finish = row.points
		}
		appendBase := func(role RuleRole, inputKind RuleInputKind, input []identity.ContentID) bool {
			if len(finish) == 0 || inputKind != RuleInputNone && len(input) != 1 {
				return false
			}
			for _, point := range finish {
				placement := RuleOccurrence{role: role, occurrence: ordinal, point: point, stage: RuleStageBase, inputKind: inputKind}
				if inputKind != RuleInputNone {
					placement.input = input[0]
				}
				if !placement.Available() {
					return false
				}
				compiler.ruleOccurrences[role] = append(compiler.ruleOccurrences[role], placement)
			}
			return true
		}
		appendLocal := func(role RuleRole, inputKind RuleInputKind, inputs []identity.ContentID) bool {
			if len(finish) == 0 || inputKind == RuleInputNone || inputKind == RuleInputPredecessor || inputKind == RuleInputEntry && len(inputs) != 1 {
				return false
			}
			for _, base := range finish {
				stage, stageOK := compiler.localStage(base)
				if !stageOK {
					return false
				}
				input := base
				if inputKind == RuleInputEntry {
					input = inputs[0]
				}
				placement := RuleOccurrence{role: role, occurrence: ordinal, point: stage, input: input, stage: RuleStageLocal, inputKind: inputKind}
				if !placement.Available() {
					return false
				}
				compiler.ruleOccurrences[role] = append(compiler.ruleOccurrences[role], placement)
			}
			return true
		}
		appendComputation := func(role RuleRole) bool {
			if len(finish) == 0 || len(row.inputs) < 2 {
				return false
			}
			for _, base := range finish {
				stage, stageOK := compiler.localComputationStage(base, role, row.id, row.inputs[0], row.inputs[1])
				placement := RuleOccurrence{role: role, occurrence: ordinal, point: stage, input: base, stage: RuleStageLocal, inputKind: RuleInputFinish}
				if !stageOK || !placement.Available() {
					return false
				}
				compiler.ruleOccurrences[role] = append(compiler.ruleOccurrences[role], placement)
			}
			return true
		}
		appendLocalPredecessor := func(role RuleRole) bool {
			if !geometry.route.Available() {
				return false
			}
			if _, duplicate := compiler.environmentRouteDuplicates[geometry.route]; duplicate {
				return false
			}
			predecessor, found := compiler.environmentByRoute[geometry.route]
			if !found || !predecessor.Available() {
				return false
			}
			finishMember := false
			for _, point := range finish {
				if point == predecessor.to {
					finishMember = true
					break
				}
			}
			stage, stageOK := compiler.localStage(predecessor.to)
			placement := RuleOccurrence{role: role, occurrence: ordinal, point: stage, input: predecessor.from, stage: RuleStageLocal, inputKind: RuleInputPredecessor, route: geometry.route}
			if !finishMember || !stageOK || !placement.Available() {
				return false
			}
			compiler.ruleOccurrences[role] = append(compiler.ruleOccurrences[role], placement)
			return true
		}
		appendCallStage := func(role RuleRole, stage RuleStage) bool {
			if len(finish) == 0 || stage < RuleStageCallDispatch || stage > RuleStageCallEffect {
				return false
			}
			for _, base := range finish {
				stages, stagesOK := compiler.callStage(base)
				if !stagesOK {
					return false
				}
				point, input := stages.dispatch, base
				switch stage {
				case RuleStageCallSummary:
					point, input = stages.summary, stages.dispatch
				case RuleStageCallEffect:
					point, input = stages.effect, stages.summary
				}
				placement := RuleOccurrence{role: role, occurrence: ordinal, point: point, input: input, stage: stage, inputKind: RuleInputFinish}
				if !placement.Available() {
					return false
				}
				compiler.ruleOccurrences[role] = append(compiler.ruleOccurrences[role], placement)
			}
			return true
		}
		ok := true
		switch row.kind {
		case OccurrenceValueSource:
			ok = appendBase(RuleRoleValueSource, RuleInputNone, nil)
		case OccurrenceValues:
			if len(row.points) != 0 {
				ok = appendBase(RuleRolePackSource, RuleInputNone, nil)
			}
		case OccurrenceStorageRead, OccurrenceStorageBindTransfer:
			// Storage reads and fixed bind transfers read the exact pre-result
			// Entry environment, then write at a separate post-Finish Local cut.
			// The entry witness is issued by Program and retained only in the
			// sealed artifact geometry; Link never reconstructs it.
			ok = appendLocal(RuleRoleValueStorageTransfer, RuleInputEntry, geometry.entry)
		case OccurrenceStorageWrite:
			// A storage write reads its exact reverse assignment-commit
			// predecessor, including the parent route's guard/reset proof, and
			// writes at the Local cut after that route's Finish attachment.
			ok = appendLocalPredecessor(RuleRoleValueStorageTransfer)
		case OccurrenceIndexRead:
			ok = appendLocal(RuleRoleRawGet, RuleInputEntry, geometry.entry)
		case OccurrenceIndexWrite:
			ok = appendLocalPredecessor(RuleRoleRawSet)
		case OccurrenceBinaryEquality:
			// A computation consumes the environment after its operands have
			// finished. ProgramArtifact gives every primitive its own stable local
			// cut; installLocalStagesFailure orders those cuts by exact semantic
			// operand dependencies before any Link mount exists.
			ok = appendComputation(RuleRoleValueBinaryEquality)
		case OccurrenceBinaryArithmetic:
			ok = appendComputation(RuleRoleValueBinaryArithmetic)
		case OccurrenceBinaryOrder:
			ok = appendComputation(RuleRoleValueBinaryOrder)
		case OccurrenceBinaryPresenceRefinement:
			// The generic refinement consumes its exact guarded predecessor and
			// writes at the ordinary local cut.  It never guesses a later
			// consumer through a shared base point.
			ok = appendLocalPredecessor(RuleRoleValuePresenceRefinement)
		case OccurrenceCall:
			ok = appendCallStage(RuleRoleCallDispatch, RuleStageCallDispatch) &&
				appendBase(RuleRolePackSource, RuleInputNone, nil) &&
				appendCallStage(RuleRoleEffectSelected, RuleStageCallEffect) &&
				appendCallStage(RuleRoleEffectOpaque, RuleStageCallEffect) &&
				appendCallStage(RuleRoleEffectBody, RuleStageCallEffect)
		case OccurrenceCallActivation:
			ok = appendCallStage(RuleRoleCallActivation, RuleStageCallSummary)
		case OccurrenceAllocation:
			ok = appendBase(RuleRoleHeapIngress, RuleInputNone, nil) &&
				appendLocal(RuleRoleValueAllocation, RuleInputEntry, geometry.entry)
			if ok && row.code == uint64(flow.AllocationFormEmpty) {
				ok = appendLocal(RuleRoleHeapEmpty, RuleInputFinish, finish)
			}
			if ok && row.code == uint64(flow.AllocationFormClosed) {
				ok = appendLocal(RuleRoleHeapClosed, RuleInputFinish, finish)
			}
		}
		if !ok {
			return compileFailure(CompileStageOccurrences, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	return CompileFailure{}
}
