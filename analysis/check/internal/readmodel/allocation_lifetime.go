package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
)

// ForEachAllocationLifetime visits solved allocation lifetime exports.
func (r Reader) ForEachAllocationLifetime(visit func(AllocationLifetime) bool) bool {
	if r.result == nil || visit == nil {
		return false
	}
	visited := false
	for _, fact := range r.result.AllocationLifetimeFacts() {
		visited = true
		if !visit(readmodelAllocationLifetimeFromBody(fact)) {
			return true
		}
	}
	return visited
}

func readmodelAllocationLifetimeFromBody(fact body.AllocationLifetimeFact) AllocationLifetime {
	return AllocationLifetime{
		SchemaVersion:        readapi.AllocationLifetimeSchemaVersion,
		ID:                   fact.ID,
		BirthPoint:           fact.BirthPoint,
		BirthSpan:            sourceSpanFromBody(fact.BirthSpan),
		HasBirthSpan:         fact.HasBirthSpan,
		DiesBeforeSuspension: fact.DiesBeforeSuspension,
	}
}
