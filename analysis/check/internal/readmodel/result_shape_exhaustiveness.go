package readmodel

import "github.com/wippyai/go-lua/analysis/check/body"

// ForEachResultShapeExhaustiveness visits case-specific field reads on
// discriminated unions where solved state has not proved the required case.
func (r Reader) ForEachResultShapeExhaustiveness(visit func(ResultShapeExhaustiveness) bool) bool {
	if r.result == nil || visit == nil {
		return false
	}
	visited := false
	return r.result.ForEachResultShapeExhaustivenessProof(func(proof body.ResultShapeExhaustivenessProof) bool {
		visited = true
		return visit(resultShapeExhaustivenessFromBody(proof))
	}) || visited
}

func resultShapeExhaustivenessFromBody(proof body.ResultShapeExhaustivenessProof) ResultShapeExhaustiveness {
	return ResultShapeExhaustiveness{
		Point:         proof.Point,
		ReceiverLabel: proof.ReceiverLabel,
		ReadLabel:     proof.ReadLabel,
		Discriminant:  proof.Discriminant,
		RequiredCase:  proof.RequiredCase,
		Span:          sourceSpanFromBody(proof.Span),
	}
}
