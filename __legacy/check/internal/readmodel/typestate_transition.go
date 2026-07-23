package readmodel

import "github.com/wippyai/go-lua/analysis/check/body"

// ForEachTypestateInvalidTransition visits every solved lifecycle transition
// whose declared source state is proven false at its call site.
func (r Reader) ForEachTypestateInvalidTransition(visit func(TypestateInvalidTransition) bool) bool {
	if r.result == nil || visit == nil {
		return false
	}
	proofs := r.result.TypestateInvalidTransitionProofs()
	for _, proof := range proofs {
		if !visit(typestateInvalidTransitionFromBody(proof)) {
			return true
		}
	}
	return len(proofs) > 0
}

func typestateInvalidTransitionFromBody(proof body.TypestateInvalidTransitionProof) TypestateInvalidTransition {
	return TypestateInvalidTransition{
		Point:    proof.Point,
		Span:     sourceSpanFromBody(proof.Span),
		Resource: proof.Resource,
		Protocol: proof.Protocol,
		Expected: proof.Expected,
		Found:    proof.Found,
		Target:   proof.Target,
	}
}
