package readmodel

import "github.com/wippyai/go-lua/analysis/check/body"

func (r Reader) ForEachTypestateRequirement(visit func(TypestateRequirement) bool) bool {
	if r.result == nil || visit == nil {
		return false
	}
	proofs := r.result.TypestateRequirementProofs()
	for _, proof := range proofs {
		if !visit(TypestateRequirement{
			Point: proof.Point, Span: sourceSpanFromBody(proof.Span), Resource: proof.Resource,
			Protocol: proof.Protocol, Expected: proof.Expected, Found: proof.Found, Target: proof.Target,
			Refuted: proof.Status == body.TypestateRequirementRefuted,
		}) {
			return true
		}
	}
	return len(proofs) > 0
}
