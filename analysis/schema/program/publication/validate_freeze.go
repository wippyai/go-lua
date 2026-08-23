package publication

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
	"github.com/wippyai/go-lua/analysis/schema/program/heapindex"
)

// environmentEdgeForRoute reads the canonical Frozen EnvironmentEdge
// column directly. Duplicate routes are rejected at the only consumer that
// needs route identity, so seal validation retains no inverse route index.
func (validator *validator) environmentEdgeForRoute(route identity.ContentID) (programschema.EnvironmentEdge, bool, bool) {
	if validator == nil || !route.Available() {
		return programschema.EnvironmentEdge{}, false, false
	}
	edgeCount, published := programschema.EnvironmentEdgeFamily().Count(&validator.frozen, validator.catalog)
	if !published {
		return programschema.EnvironmentEdge{}, false, false
	}
	var found programschema.EnvironmentEdge
	for index := 0; index < edgeCount; index++ {
		edge, held := programschema.EnvironmentEdgeFamily().At(&validator.frozen, validator.catalog, index)
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

func (validator *validator) validateSealFreeze(state *validationState) bool {
	if validator == nil || state == nil {
		return false
	}
	valuesCount, valuesPublished := programschema.ValuesFamily().Count(&validator.frozen, validator.catalog)
	if !valuesPublished {
		return false
	}
	state.valuesRows = make(map[identity.ContentID]struct{}, valuesCount)
	for index := 0; index < valuesCount; index++ {
		row, held := programschema.ValuesFamily().At(&validator.frozen, validator.catalog, index)
		if !held {
			return false
		}
		if !row.Available() {
			return false
		}
		state.valuesRows[row.ID()] = struct{}{}
	}
	allocationCount, allocationsPublished := heapallocation.AllocationFamily().Count(&validator.frozen, validator.catalog)
	if !allocationsPublished {
		return false
	}
	seenHeapAllocations := make(map[identity.ContentID]struct{}, allocationCount)
	for index := 0; index < allocationCount; index++ {
		allocation, held := heapallocation.AllocationFamily().At(&validator.frozen, validator.catalog, index)
		offset, fields, spanOK := allocation.FieldSpan()
		if !held || !spanOK {
			return false
		}
		if _, exists := state.occurrenceRows[programschema.OccurrenceAllocation][allocation.ID()]; !exists {
			return false
		}
		if _, duplicate := seenHeapAllocations[allocation.ID()]; duplicate {
			return false
		}
		seenHeapAllocations[allocation.ID()] = struct{}{}
		for position := uint32(0); position < fields; position++ {
			field, fieldHeld := heapallocation.FieldFamily().At(&validator.frozen, validator.catalog, int(offset+position))
			if !fieldHeld {
				return false
			}
			if _, exists := state.occurrenceRows[programschema.OccurrenceAllocationField][field.ID()]; !exists {
				return false
			}
			if _, exists := state.valuesRows[field.ValuesID()]; !exists {
				return false
			}
		}
	}
	indexCount, indexesPublished := heapindex.Family().Count(&validator.frozen, validator.catalog)
	if !indexesPublished {
		return false
	}
	seenHeapIndexes := make(map[identity.ContentID]struct{}, indexCount)
	for index := 0; index < indexCount; index++ {
		access, held := heapindex.Family().At(&validator.frozen, validator.catalog, index)
		if !held {
			return false
		}
		kind := programschema.OccurrenceIndexWrite
		if access.Read() {
			kind = programschema.OccurrenceIndexRead
		}
		if _, exists := state.occurrenceRows[kind][access.ID()]; !exists {
			return false
		}
		if _, duplicate := seenHeapIndexes[access.ID()]; duplicate {
			return false
		}
		if !access.Read() {
			if _, exists := state.valuesRows[access.ValuesID()]; !exists {
				return false
			}
		}
		seenHeapIndexes[access.ID()] = struct{}{}
	}
	program := validator.program
	ruleCount, rulesPublished := program.RuleOccurrenceCount()
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	if !rulesPublished || !occurrencesPublished {
		return false
	}
	for index := 0; index < ruleCount; index++ {
		occurrence, occurrenceOK := program.RuleOccurrenceAt(index)
		parent, parentOK := occurrence.Occurrence()
		if !occurrenceOK || !parentOK || uint64(parent) >= uint64(occurrenceCount) {
			return false
		}
		if _, exists := state.pointRows[occurrence.PointID()]; !exists {
			return false
		}
		inputCount := occurrence.InputPointCount()
		for inputIndex := 0; inputIndex < inputCount; inputIndex++ {
			input, inputOK := occurrence.InputPointAt(inputIndex)
			if !inputOK {
				return false
			}
			if _, exists := state.pointRows[input]; !exists {
				return false
			}
		}
		if route, routeOK := occurrence.PredecessorRouteID(); routeOK {
			input, inputOK := occurrence.InputPointAt(0)
			edge, found, duplicate := validator.environmentEdgeForRoute(route)
			if duplicate {
				return false
			}
			if !inputOK || !found || edge.To() != input {
				return false
			}
		}
	}
	return true
}
