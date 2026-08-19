package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

// environmentEdgeForRoute reads the canonical Frozen EnvironmentEdge
// column directly. Duplicate routes are rejected at the only consumer that
// needs route identity, so seal validation retains no inverse route index.
func (artifact *Artifact) environmentEdgeForRoute(route identity.ContentID) (programschema.EnvironmentEdge, bool, bool) {
	if artifact == nil || !route.Available() {
		return programschema.EnvironmentEdge{}, false, false
	}
	edgeCount, published := coldCount(artifact, programschema.EnvironmentEdgeFamily())
	if !published {
		return programschema.EnvironmentEdge{}, false, false
	}
	var found programschema.EnvironmentEdge
	for index := 0; index < edgeCount; index++ {
		edge, held := coldRow(artifact, programschema.EnvironmentEdgeFamily(), index)
		if !held || !edge.Available() {
			return programschema.EnvironmentEdge{}, false, false
		}
		if edge.RouteID() != route {
			continue
		}
		if found.Available() {
			return found, true, true
		}
		found = edge
	}
	return found, found.Available(), false
}

func (artifact *Artifact) validateSealFreeze(state *sealValidationState) CompileFailure {
	if artifact == nil || state == nil {
		return compileFailure(CompileStageSeal, CompileRowAuthority, -1, -1, CompileReasonArtifactIdentity)
	}
	valuesCount, valuesPublished := coldCount(artifact, programschema.ValuesFamily())
	if !valuesPublished {
		return compileFailure(CompileStageSeal, CompileRowValues, -1, -1, CompileReasonValuesUnavailable)
	}
	state.valuesRows = make(map[identity.ContentID]struct{}, valuesCount)
	for index := 0; index < valuesCount; index++ {
		row, held := coldRow(artifact, programschema.ValuesFamily(), index)
		if !held {
			return compileFailure(CompileStageSeal, CompileRowValues, index, -1, CompileReasonValuesUnavailable)
		}
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowValues, -1, -1, CompileReasonValuesUnavailable)
		}
		state.valuesRows[row.ID()] = struct{}{}
	}
	allocationCount, allocationsPublished := coldCount(artifact, programschema.HeapAllocationFamily())
	if !allocationsPublished {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceAllocation)
	}
	seenHeapAllocations := make(map[identity.ContentID]struct{}, allocationCount)
	for index := 0; index < allocationCount; index++ {
		allocation, held := coldRow(artifact, programschema.HeapAllocationFamily(), index)
		offset, fields, spanOK := allocation.FieldSpan()
		if !held || !spanOK {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAllocation)
		}
		if _, exists := state.occurrenceRows[programschema.OccurrenceAllocation][allocation.ID()]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAllocation)
		}
		if _, duplicate := seenHeapAllocations[allocation.ID()]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAllocation)
		}
		seenHeapAllocations[allocation.ID()] = struct{}{}
		for position := uint32(0); position < fields; position++ {
			field, fieldHeld := coldRow(artifact, programschema.HeapFieldFamily(), int(offset+position))
			if !fieldHeld {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, int(position), CompileReasonOccurrenceAllocation)
			}
			if _, exists := state.occurrenceRows[programschema.OccurrenceAllocationField][field.ID()]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, int(position), CompileReasonOccurrenceAllocation)
			}
			if _, exists := state.valuesRows[field.ValuesID()]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, int(position), CompileReasonOccurrenceAllocation)
			}
		}
	}
	indexCount, indexesPublished := coldCount(artifact, programschema.HeapIndexFamily())
	if !indexesPublished {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceIndexShape)
	}
	seenHeapIndexes := make(map[identity.ContentID]struct{}, indexCount)
	for index := 0; index < indexCount; index++ {
		access, held := coldRow(artifact, programschema.HeapIndexFamily(), index)
		if !held {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		kind := programschema.OccurrenceIndexWrite
		if access.Read() {
			kind = programschema.OccurrenceIndexRead
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
	program := artifact.Program()
	ruleCount, rulesPublished := program.RuleOccurrenceCount()
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	if !rulesPublished || !occurrencesPublished {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	for index := 0; index < ruleCount; index++ {
		occurrence, occurrenceOK := program.RuleOccurrenceAt(index)
		parent, parentOK := occurrence.Occurrence()
		if !occurrenceOK || !parentOK || uint64(parent) >= uint64(occurrenceCount) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if _, exists := state.pointRows[occurrence.PointID()]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 0, CompileReasonOccurrenceUnavailable)
		}
		input, hasInput := occurrence.InputPoint()
		if hasInput {
			if _, exists := state.pointRows[input]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 1, CompileReasonOccurrenceUnavailable)
			}
		}
		if occurrence.InputKind() == programschema.RuleInputPredecessor {
			route, routeOK := occurrence.PredecessorRouteID()
			if !routeOK {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 2, CompileReasonOccurrenceUnavailable)
			}
			edge, found, duplicate := artifact.environmentEdgeForRoute(route)
			if duplicate {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 2, CompileReasonOccurrenceUnavailable)
			}
			if !found || edge.To() != input {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 2, CompileReasonOccurrenceUnavailable)
			}
		}
	}
	return CompileFailure{}
}
