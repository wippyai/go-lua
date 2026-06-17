package calloutcome

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// HasAuthoritativePostReturnEvidence reports whether outcome carries return or
// post-return state evidence strong enough to block supplemental fallback facts.
// Weak top/any/unknown result slots are not authority; they remain fallback
// evidence that stronger providers may refine.
func HasAuthoritativePostReturnEvidence(reg *axis.Registry, outcome factapply.CallOutcome) bool {
	return outcomeHasAuthoritativeResult(reg, outcome.Results) ||
		len(outcome.NormalReturnFacts.PathRefinements) != 0 ||
		len(outcome.NormalReturnFacts.PathStaticMembers) != 0 ||
		len(outcome.NormalReturnFacts.PathInvalidations) != 0 ||
		len(outcome.NormalReturnFacts.DynamicIndexFacts) != 0 ||
		len(outcome.NormalReturnFacts.BranchProofs) != 0 ||
		len(outcome.NormalReturnFacts.ChannelSelects) != 0 ||
		len(outcome.NormalReturnFacts.FrozenTables) != 0 ||
		len(outcome.NormalReturnFacts.EffectDeltas) != 0 ||
		len(outcome.NormalReturnFacts.EscapeEvents) != 0 ||
		len(outcome.NormalReturnFacts.StoreRelations) != 0 ||
		len(outcome.HeapTableObjects) != 0 ||
		len(outcome.Placements) != 0 ||
		len(outcome.ParamPathRefinements) != 0 ||
		len(outcome.ParamLengthFloors) != 0 ||
		len(outcome.ParamPathInvalidations) != 0 ||
		len(outcome.ParamConditions) != 0 ||
		len(outcome.ParamPathRelations) != 0 ||
		len(outcome.ReturnConditionRefinements) != 0 ||
		len(outcome.ReturnPresenceRelations) != 0
}

func outcomeHasAuthoritativeResult(reg *axis.Registry, results []factapply.CallResult) bool {
	for _, result := range results {
		if resultValueHasAuthority(reg, result.Value) {
			return true
		}
	}
	return false
}

func resultValueHasAuthority(reg *axis.Registry, value product.Value) bool {
	if reg == nil ||
		product.Equal(reg, value, product.Bottom(reg)) ||
		product.Equal(reg, value, product.Top()) {
		return false
	}
	ev := product.Get(reg, value, evidence.Key)
	if ev.IsExplicitTop() || ev.IsGradualTop() {
		return false
	}
	if t, ok := typevalue.TypeOf(reg, value); ok {
		return !typ.IsAny(t) && !typ.IsUnknown(t)
	}
	return true
}
