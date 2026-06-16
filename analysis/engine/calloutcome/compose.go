package calloutcome

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// WithSupplemental composes two call outcome providers. Result slots and
// post-return state facts are accumulated only until a provider declares
// post-return authority for the call. Pre-call diagnostic obligations are
// accumulated.
func WithSupplemental(primary, supplemental factapply.CallOutcomeProvider) factapply.CallOutcomeProvider {
	if primary == nil {
		return supplemental
	}
	if supplemental == nil {
		return primary
	}
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) factapply.CallOutcome {
		out := primary(ctx, site, in, read)
		second := supplemental(ctx, site, in, read)
		out = withSupplementalResultSlots(ctx.Registry, out, second.Results)
		return withSupplementalFacts(out, second)
	}
}

func withSupplementalResultSlots(reg *axis.Registry, out factapply.CallOutcome, results []factapply.CallResult) factapply.CallOutcome {
	if len(results) == 0 {
		return out
	}
	if out.PostReturnAuthority {
		return out
	}
	if len(out.Results) == 0 {
		out.Results = append(out.Results, results...)
		return out
	}
	position := make(map[int]int, len(out.Results))
	for i, result := range out.Results {
		position[result.Index] = i
	}
	for _, result := range results {
		pos, ok := position[result.Index]
		if !ok {
			position[result.Index] = len(out.Results)
			out.Results = append(out.Results, result)
			continue
		}
		if resultSlotLacksSpecificTypeEvidence(reg, out.Results[pos].Value) && !resultSlotLacksSpecificTypeEvidence(reg, result.Value) {
			out.Results[pos].Value = product.Meet(reg, out.Results[pos].Value, result.Value)
		}
	}
	return out
}

func resultSlotLacksSpecificTypeEvidence(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok {
		return true
	}
	return typ.IsAny(t) || typ.IsUnknown(t)
}

// HasPostReturnEvidence reports whether outcome carries useful return or
// post-return state evidence. Weak top/any/unknown result slots are not
// authority; they remain fallback evidence that stronger providers may refine.
func HasPostReturnEvidence(reg *axis.Registry, outcome factapply.CallOutcome) bool {
	return outcomeHasAuthoritativeResult(reg, outcome.Results) ||
		len(outcome.NormalReturnFacts.PathRefinements) != 0 ||
		len(outcome.NormalReturnFacts.PathStaticMembers) != 0 ||
		len(outcome.NormalReturnFacts.DynamicIndexFacts) != 0 ||
		len(outcome.NormalReturnFacts.BranchProofs) != 0 ||
		len(outcome.NormalReturnFacts.ChannelSelects) != 0 ||
		len(outcome.NormalReturnFacts.EffectDeltas) != 0 ||
		len(outcome.NormalReturnFacts.EscapeEvents) != 0 ||
		len(outcome.ParamPathRefinements) != 0 ||
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

func withSupplementalFacts(out, second factapply.CallOutcome) factapply.CallOutcome {
	out.ParamObligations = append(out.ParamObligations, second.ParamObligations...)
	if out.PostReturnAuthority {
		return out
	}
	out.NormalReturnFacts.PathRefinements = append(out.NormalReturnFacts.PathRefinements, second.NormalReturnFacts.PathRefinements...)
	out.NormalReturnFacts.PathStaticMembers = append(out.NormalReturnFacts.PathStaticMembers, second.NormalReturnFacts.PathStaticMembers...)
	out.NormalReturnFacts.DynamicIndexFacts = append(out.NormalReturnFacts.DynamicIndexFacts, second.NormalReturnFacts.DynamicIndexFacts...)
	out.NormalReturnFacts.BranchProofs = append(out.NormalReturnFacts.BranchProofs, second.NormalReturnFacts.BranchProofs...)
	out.NormalReturnFacts.ChannelSelects = append(out.NormalReturnFacts.ChannelSelects, second.NormalReturnFacts.ChannelSelects...)
	out.NormalReturnFacts.EffectDeltas = append(out.NormalReturnFacts.EffectDeltas, second.NormalReturnFacts.EffectDeltas...)
	out.NormalReturnFacts.EscapeEvents = append(out.NormalReturnFacts.EscapeEvents, second.NormalReturnFacts.EscapeEvents...)
	out.ParamPathRefinements = append(out.ParamPathRefinements, second.ParamPathRefinements...)
	out.ParamPathInvalidations = append(out.ParamPathInvalidations, second.ParamPathInvalidations...)
	out.ParamConditions = append(out.ParamConditions, second.ParamConditions...)
	out.ParamPathRelations = append(out.ParamPathRelations, second.ParamPathRelations...)
	out.ReturnConditionRefinements = append(out.ReturnConditionRefinements, second.ReturnConditionRefinements...)
	out.ReturnPresenceRelations = append(out.ReturnPresenceRelations, second.ReturnPresenceRelations...)
	out.PostReturnAuthority = second.PostReturnAuthority
	return out
}
