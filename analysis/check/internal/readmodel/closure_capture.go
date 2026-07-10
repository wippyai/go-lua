package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
)

// ForEachClosureCapture visits solved closure-site capture exports.
func (r Reader) ForEachClosureCapture(visit func(ClosureCapture) bool) bool {
	if r.result == nil || visit == nil {
		return false
	}
	visited := false
	r.result.ForEachClosureCaptureFact(func(fact body.ClosureCaptureFact) bool {
		visited = true
		return visit(readmodelClosureCaptureFromBody(fact))
	})
	return visited
}

func readmodelClosureCaptureFromBody(fact body.ClosureCaptureFact) ClosureCapture {
	return ClosureCapture{
		SchemaVersion:   readapi.ClosureCaptureSchemaVersion,
		Point:           fact.Point,
		Function:        uint64(fact.Function),
		CaptureIndex:    fact.CaptureIndex,
		Symbol:          uint64(fact.Symbol),
		Name:            fact.Name,
		Path:            fact.Path.Clone(),
		Policy:          fact.Policy.String(),
		Value:           fact.Value,
		Type:            fact.Type,
		HasType:         fact.HasType,
		Shape:           fact.Shape,
		HasShape:        fact.HasShape,
		StableShape:     fact.StableShape,
		ShapeTier:       fact.ShapeTier.String(),
		Nilable:         fact.Nilable,
		NilabilityKnown: fact.NilabilityKnown,
		Placement:       fact.Placement,
		HasPlacement:    fact.HasPlacement,
		Identity:        fact.Identity,
		HasIdentity:     fact.HasIdentity,
	}
}
