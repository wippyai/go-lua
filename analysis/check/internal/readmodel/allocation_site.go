package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
)

// ForEachAllocationSite visits solved table-allocation exports.
func (r Reader) ForEachAllocationSite(visit func(AllocationSite) bool) bool {
	if r.result == nil || visit == nil {
		return false
	}
	visited := false
	r.result.ForEachAllocationSiteFact(func(fact body.AllocationSiteFact) bool {
		visited = true
		return visit(readmodelAllocationSiteFromBody(fact))
	})
	return visited
}

func readmodelAllocationSiteFromBody(fact body.AllocationSiteFact) AllocationSite {
	fields := make([]readapi.AllocationField, len(fact.Fields))
	for i, field := range fact.Fields {
		fields[i] = readapi.AllocationField{Name: field.Name, Type: field.Type}
	}
	return AllocationSite{
		SchemaVersion: readapi.AllocationSiteSchemaVersion,
		Point:         fact.Point,
		ExpressionID:  uint64(fact.ExpressionID),
		ExprRef:       uint64(fact.ExprRef),
		Identity:      fact.Identity,
		Placement:     fact.Placement,
		HasPlacement:  fact.HasPlacement,
		Shape:         fact.Shape,
		Fields:        fields,
		StableShape:   fact.StableShape,
		Decomposable:  fact.Decomposable,
	}
}
