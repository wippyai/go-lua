package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/cold"
)

func (artifact *Artifact) validateSealFreeze(state *sealValidationState) CompileFailure {
	if artifact == nil || state == nil {
		return compileFailure(CompileStageSeal, CompileRowAuthority, -1, -1, CompileReasonArtifactIdentity)
	}
	valuesCount, valuesPublished := coldCount(artifact, cold.ValuesFamily())
	if !valuesPublished {
		return compileFailure(CompileStageSeal, CompileRowValues, -1, -1, CompileReasonValuesUnavailable)
	}
	state.valuesRows = make(map[identity.ContentID]struct{}, valuesCount)
	for index := 0; index < valuesCount; index++ {
		row, held := coldRow(artifact, cold.ValuesFamily(), index)
		if !held {
			return compileFailure(CompileStageSeal, CompileRowValues, index, -1, CompileReasonValuesUnavailable)
		}
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowValues, -1, -1, CompileReasonValuesUnavailable)
		}
		state.valuesRows[row.ID()] = struct{}{}
	}
	allocationCount, allocationsPublished := coldCount(artifact, cold.HeapAllocationFamily())
	if !allocationsPublished {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceAllocation)
	}
	seenHeapAllocations := make(map[identity.ContentID]struct{}, allocationCount)
	for index := 0; index < allocationCount; index++ {
		allocation, held := coldRow(artifact, cold.HeapAllocationFamily(), index)
		offset, fields, spanOK := allocation.FieldSpan()
		if !held || !spanOK {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAllocation)
		}
		if _, exists := state.occurrenceRows[OccurrenceAllocation][allocation.ID()]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAllocation)
		}
		if _, duplicate := seenHeapAllocations[allocation.ID()]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAllocation)
		}
		seenHeapAllocations[allocation.ID()] = struct{}{}
		for position := uint32(0); position < fields; position++ {
			field, fieldHeld := coldRow(artifact, cold.HeapFieldFamily(), int(offset+position))
			if !fieldHeld {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, int(position), CompileReasonOccurrenceAllocation)
			}
			if _, exists := state.occurrenceRows[OccurrenceAllocationField][field.ID()]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, int(position), CompileReasonOccurrenceAllocation)
			}
			if _, exists := state.valuesRows[field.ValuesID()]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, int(position), CompileReasonOccurrenceAllocation)
			}
		}
	}
	indexCount, indexesPublished := coldCount(artifact, cold.HeapIndexFamily())
	if !indexesPublished {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceIndexShape)
	}
	seenHeapIndexes := make(map[identity.ContentID]struct{}, indexCount)
	for index := 0; index < indexCount; index++ {
		access, held := coldRow(artifact, cold.HeapIndexFamily(), index)
		if !held {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		kind := OccurrenceIndexWrite
		if access.Read() {
			kind = OccurrenceIndexRead
		}
		if _, exists := state.occurrenceRows[kind][access.ID()]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		if _, duplicate := seenHeapIndexes[access.ID()]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		if !access.Read() {
			if _, exists := state.valuesRows[access.ValuesID()]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
			}
		}
		seenHeapIndexes[access.ID()] = struct{}{}
	}
	if artifact.ruleOccurrences == nil {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	for index, occurrence := range artifact.ruleOccurrences {
		if !occurrence.Available() || int(occurrence.occurrence) >= len(artifact.occurrences) {
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
			if !found || edge.to != occurrence.input {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 2, CompileReasonOccurrenceUnavailable)
			}
		}
	}
	return CompileFailure{}
}
