package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// CallResult is one indexed abstract result produced by a call.
type CallResult struct {
	Index int
	Value product.Value
}

// CallOutcomeProvider resolves rich call-site evidence into one generic call
// outcome payload.
type CallOutcomeProvider func(ctx transfer.NodeContext, site factflow.CallSite, in state.State, read func(cfg.Point) state.State) CallOutcome

// WithSupplementalCallOutcome composes two call outcome providers. Result
// slots and post-return state facts are accumulated only until a provider
// declares post-return authority for the call. Pre-call diagnostic obligations
// are accumulated.
func WithSupplementalCallOutcome(primary, supplemental CallOutcomeProvider) CallOutcomeProvider {
	if primary == nil {
		return supplemental
	}
	if supplemental == nil {
		return primary
	}
	return func(ctx transfer.NodeContext, site factflow.CallSite, in state.State, read func(cfg.Point) state.State) CallOutcome {
		out := primary(ctx, site, in, read)
		second := supplemental(ctx, site, in, read)
		out = withSupplementalResultSlots(ctx.Registry, out, second.Results)
		return withSupplementalOutcomeFacts(out, second)
	}
}

func withSupplementalResultSlots(reg *axis.Registry, out CallOutcome, results []CallResult) CallOutcome {
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
		// Quality-aware merge: a concrete supplemental slot refines a primary
		// slot that lacks specific type evidence. Concrete-vs-concrete keeps the
		// primary (provider order). The meet preserves non-type axes already
		// carried by the primary result.
		if resultSlotLacksSpecificTypeEvidence(reg, out.Results[pos].Value) && !resultSlotLacksSpecificTypeEvidence(reg, result.Value) {
			out.Results[pos].Value = product.Meet(reg, out.Results[pos].Value, result.Value)
		}
	}
	return out
}

// resultSlotLacksSpecificTypeEvidence reports whether a result slot carries no
// usable type evidence. `any` and `unknown` are weak evidence here; they are not
// trusted claims.
func resultSlotLacksSpecificTypeEvidence(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok {
		return true
	}
	return typ.IsAny(t) || typ.IsUnknown(t)
}

// OutcomeHasPostReturnEvidence reports whether outcome carries useful return or
// post-return state evidence. Weak top/any/unknown result slots are not
// authority; they remain fallback evidence that stronger providers may refine.
func OutcomeHasPostReturnEvidence(reg *axis.Registry, outcome CallOutcome) bool {
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

func outcomeHasAuthoritativeResult(reg *axis.Registry, results []CallResult) bool {
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

func withSupplementalOutcomeFacts(out, second CallOutcome) CallOutcome {
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

// CallOutcome is the generic payload produced at a call boundary. It carries
// return-slot values plus normal-return facts expressed over placeholder paths
// such as $0 and $1. Fact application rebases those paths at the caller.
type CallOutcome struct {
	Results []CallResult

	// PostReturnAuthority means this outcome matched a semantic provider that
	// owns result slots and normal-return state facts for the call. Supplemental
	// providers may still add diagnostic ParamObligations, but must not publish
	// weaker return or post-return facts through this call.
	PostReturnAuthority bool

	NormalReturnFacts          callboundary.NormalReturnFacts
	ParamObligations           []CallParamObligation
	ParamPathRefinements       []CallParamPathRefinement
	ParamPathInvalidations     []CallParamPathInvalidation
	ParamConditions            []CallParamCondition
	ParamPathRelations         []CallParamPathRelation
	ReturnConditionRefinements []CallReturnConditionRefinement
	ReturnPresenceRelations    []CallReturnPresenceRelation
}

// CallParamObligation records a pre-call value constraint for one explicit
// argument. It is diagnostic evidence only and is not applied as a normal-return
// refinement to caller state.
type CallParamObligation struct {
	ParamIndex int
	Value      product.Value
}

// CallParamPathRefinement records a normal-return value constraint for a
// parameter placeholder path. Parameter placeholders are indexed by explicit
// argument position and do not include the receiver slot.
type CallParamPathRefinement struct {
	Path  pathdom.Path
	Value product.Value
}

// CallParamPathInvalidation records that descendants below a parameter
// placeholder path were invalidated by a normal-returning call.
type CallParamPathInvalidation struct {
	Path pathdom.Path
}

// CallParamCondition records that a normal return selects the truthiness facts
// for one call argument expression.
type CallParamCondition struct {
	ParamIndex int
	Value      bool
}

// CallPathRelationKind classifies a normal-return relation over placeholder
// paths.
type CallPathRelationKind uint8

const (
	CallPathRelationEqual CallPathRelationKind = iota + 1
)

// CallParamPathRelation records a normal-return relation over parameter
// placeholder paths. Parameter placeholders are indexed by explicit argument
// position and do not include the receiver slot.
type CallParamPathRelation struct {
	Kind  CallPathRelationKind
	Left  pathdom.Path
	Right pathdom.Path
}

// CallReturnConditionRefinement records a parameter-relative value refinement
// selected by the boolean value of one call return slot.
type CallReturnConditionRefinement struct {
	ReturnIndex int
	ReturnValue bool
	Target      pathdom.Path
	Value       product.Value
}

// CallReturnPresenceRelation records a must implication between two call
// return slots.
type CallReturnPresenceRelation struct {
	TriggerIndex    int
	TriggerPresence presence.Value
	TargetIndex     int
	TargetPresence  presence.Value
}
