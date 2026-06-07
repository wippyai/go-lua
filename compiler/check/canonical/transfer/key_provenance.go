package transfer

import "github.com/wippyai/go-lua/types/flow"

func (t *Transfer) applyKeyProvenancePathProof(out *flow.PointState, proof flow.KeyProvenancePathProof) bool {
	result, changed := flow.ApplyKeyProvenancePathProof(out, proof)
	if t.applyKeyProvenanceResult(out, result) {
		return true
	}
	return changed
}

func (t *Transfer) applyKeyProvenanceResult(out *flow.PointState, result flow.KeyProvenanceResult) bool {
	if result.KeyRefinementValue.IsZero() || result.KeyRefinementPath.Symbol == 0 {
		return false
	}
	t.applyRefinementEffect(out, RefinementEffect{
		Place: Place{Root: result.KeyRefinementPath.Symbol},
		Kind:  RefinementSetValue,
		Value: result.KeyRefinementValue,
	})
	return true
}
