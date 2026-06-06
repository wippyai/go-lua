package transfer

import "github.com/wippyai/go-lua/types/flow"

func (t *Transfer) applyKeyProvenancePathProof(out *flow.PointState, proof flow.KeyProvenancePathProof) bool {
	result, changed := flow.ApplyKeyProvenancePathProof(out, proof)
	if result.KeyRefinementValue.IsZero() || result.KeyRefinementPath.Symbol == 0 {
		return changed
	}
	t.applyRefinementEffect(out, RefinementEffect{
		Place: Place{Root: result.KeyRefinementPath.Symbol},
		Kind:  RefinementSetValue,
		Value: result.KeyRefinementValue,
	})
	return true
}
