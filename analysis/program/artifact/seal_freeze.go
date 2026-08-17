package artifact

import "github.com/wippyai/go-lua/analysis/identity"

func (artifact *Artifact) validateSealFreeze(state *sealValidationState) CompileFailure {
	state.valuesRows = make(map[identity.ContentID]struct{}, len(artifact.values))
	for _, row := range artifact.values {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowValues, -1, -1, CompileReasonValuesUnavailable)
		}
		state.valuesRows[row.ID()] = struct{}{}
	}
	seenHeapAllocations := make(map[identity.ContentID]struct{}, len(artifact.heapAllocations))
	for index, allocation := range artifact.heapAllocations {
		if !allocation.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAllocation)
		}
		if _, exists := state.occurrenceRows[OccurrenceAllocation][allocation.id]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAllocation)
		}
		if _, duplicate := seenHeapAllocations[allocation.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAllocation)
		}
		seenHeapAllocations[allocation.id] = struct{}{}
		for fieldIndex, field := range allocation.fields {
			if !field.Available() {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, fieldIndex, CompileReasonOccurrenceAllocation)
			}
			if _, exists := state.occurrenceRows[OccurrenceAllocationField][field.id]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, fieldIndex, CompileReasonOccurrenceAllocation)
			}
			if _, exists := state.valuesRows[field.valuesID]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, fieldIndex, CompileReasonOccurrenceAllocation)
			}
		}
	}
	seenHeapIndexes := make(map[identity.ContentID]struct{}, len(artifact.heapIndexes))
	for index, access := range artifact.heapIndexes {
		if !access.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		kind := OccurrenceIndexWrite
		if access.read {
			kind = OccurrenceIndexRead
		}
		if _, exists := state.occurrenceRows[kind][access.id]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		if _, duplicate := seenHeapIndexes[access.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		if !access.read {
			if _, exists := state.valuesRows[access.valuesID]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
			}
		}
		seenHeapIndexes[access.id] = struct{}{}
	}
	if artifact.ruleOccurrences == nil {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	for role, rows := range artifact.ruleOccurrences {
		if !role.valid() || !ruleRoleSupported(role) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
		}
		for index, occurrence := range rows {
			if !occurrence.Available() || occurrence.role != role || int(occurrence.occurrence) >= len(artifact.occurrences) {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			if _, exists := state.pointRows[occurrence.point]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 0, CompileReasonOccurrenceUnavailable)
			}
			if occurrence.input.Available() {
				if _, exists := state.pointRows[occurrence.input]; !exists {
					return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 1, CompileReasonOccurrenceUnavailable)
				}
			}
			if occurrence.inputKind == RuleInputPredecessor {
				if _, duplicate := state.environmentRouteDuplicates[occurrence.route]; duplicate {
					return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 2, CompileReasonOccurrenceUnavailable)
				}
				edge, found := state.environmentByRoute[occurrence.route]
				if !found || edge.from != occurrence.input {
					return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 2, CompileReasonOccurrenceUnavailable)
				}
			}
		}
	}
	return CompileFailure{}
}
