package transformer

type rowStepKind uint8

const (
	rowStepInvalid rowStepKind = iota
	rowStepEffect
	rowStepCall
)

// rowStep is the single ordered transaction stream for the tuple executor.
// It is private until lowering is atomically switched from the legacy flat
// effect slice; no public parallel row vocabulary is introduced.
type rowStep struct {
	kind       rowStepKind
	effect     EffectTerm
	call       callFrameTerm
	guard      Guard
	memberCall boundaryMemberCallDiagnosticTerm
}

func localEffectStep(effect EffectTerm) rowStep   { return rowStep{kind: rowStepEffect, effect: effect} }
func deferredCallStep(call callFrameTerm) rowStep { return rowStep{kind: rowStepCall, call: call} }

func cloneRowSteps(steps []rowStep) []rowStep {
	if len(steps) == 0 {
		return nil
	}
	out := append([]rowStep(nil), steps...)
	for index := range out {
		if out[index].memberCall.site != 0 {
			out[index].memberCall = cloneBoundaryMemberCallDiagnostics([]boundaryMemberCallDiagnosticTerm{out[index].memberCall})[0]
		}
	}
	return out
}

func effectFramesOwned(effects *EffectArena, term EffectTerm, frames map[callFrameTerm]struct{}) bool {
	if effects == nil || effects.terms == nil || term == 0 || int(term) >= len(effects.nodes) {
		return false
	}
	n := effects.nodes[term]
	values := []ValueTerm{n.key, n.value}
	if n.invalidation.Precise != nil {
		values = append(values, n.invalidation.Precise.Key)
	}
	for _, value := range values {
		if value != 0 && !effects.terms.valueFramesOwned(value, frames, make(map[ValueTerm]bool)) {
			return false
		}
	}
	return true
}
