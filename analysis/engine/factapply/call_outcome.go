package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/state/dynamicindex"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
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
// slots are primary-by-index; all non-slot side facts are accumulated.
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
		out = withSupplementalResultSlots(out, second.Results)
		return withSupplementalOutcomeFacts(out, second)
	}
}

func withSupplementalResultSlots(out CallOutcome, results []CallResult) CallOutcome {
	if len(results) == 0 {
		return out
	}
	if len(out.Results) == 0 {
		out.Results = append(out.Results, results...)
		return out
	}
	seen := make(map[int]struct{}, len(out.Results))
	for _, result := range out.Results {
		seen[result.Index] = struct{}{}
	}
	for _, result := range results {
		if _, ok := seen[result.Index]; ok {
			continue
		}
		out.Results = append(out.Results, result)
	}
	return out
}

func withSupplementalOutcomeFacts(out, second CallOutcome) CallOutcome {
	out.PathRefinements = append(out.PathRefinements, second.PathRefinements...)
	out.ParamPathRefinements = append(out.ParamPathRefinements, second.ParamPathRefinements...)
	out.ParamPathInvalidations = append(out.ParamPathInvalidations, second.ParamPathInvalidations...)
	out.ParamConditions = append(out.ParamConditions, second.ParamConditions...)
	out.ParamPathRelations = append(out.ParamPathRelations, second.ParamPathRelations...)
	out.PathStaticMembers = append(out.PathStaticMembers, second.PathStaticMembers...)
	out.DynamicIndexFacts = append(out.DynamicIndexFacts, second.DynamicIndexFacts...)
	out.BranchProofs = append(out.BranchProofs, second.BranchProofs...)
	out.ChannelSelects = append(out.ChannelSelects, second.ChannelSelects...)
	out.EffectDeltas = append(out.EffectDeltas, second.EffectDeltas...)
	out.ReturnConditionRefinements = append(out.ReturnConditionRefinements, second.ReturnConditionRefinements...)
	out.ReturnPresenceRelations = append(out.ReturnPresenceRelations, second.ReturnPresenceRelations...)
	return out
}

// CallOutcome is the generic payload produced at a call boundary. It carries
// return-slot values plus normal-return facts expressed over placeholder paths
// such as $0 and $1. Fact application rebases those paths at the caller.
type CallOutcome struct {
	Results []CallResult

	PathRefinements            []CallPathRefinement
	ParamPathRefinements       []CallParamPathRefinement
	ParamPathInvalidations     []CallParamPathInvalidation
	ParamConditions            []CallParamCondition
	ParamPathRelations         []CallParamPathRelation
	PathStaticMembers          []CallPathStaticMember
	DynamicIndexFacts          []CallDynamicIndexFact
	BranchProofs               []CallBranchProof
	ChannelSelects             []CallChannelSelectFact
	EffectDeltas               []CallEffectDelta
	ReturnConditionRefinements []CallReturnConditionRefinement
	ReturnPresenceRelations    []CallReturnPresenceRelation
}

// CallPathRefinement records a normal-return value constraint for a
// placeholder-rooted path.
type CallPathRefinement struct {
	Path  pathdom.Path
	Value product.Value
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

// CallPathStaticMember records a normal-return static-member fact for a
// placeholder-rooted path.
type CallPathStaticMember struct {
	Path  pathdom.Path
	Value product.Value
}

// CallDynamicIndexFact records a normal-return dynamic-index fact for a
// placeholder-rooted table path.
type CallDynamicIndexFact struct {
	Table pathdom.Path
	Site  dynamicindex.Site
	Value dynamicindex.Fact
}

// CallBranchProof records a must branch proof over placeholder paths.
type CallBranchProof struct {
	Kind     pathevidence.BranchProofKind
	Path     pathdom.Path
	Presence presence.Value
	Other    pathdom.Path
}

// CallChannelSelectFact records a must channel-select fact over optional
// placeholder paths.
type CallChannelSelectFact struct {
	Select channelselectfact.ID
	Kind   channelselectfact.Kind
	Result pathdom.Path
	Case   pathdom.Path
	Index  int
}

// CallEffectDelta records a normal-return effect delta for a placeholder path.
type CallEffectDelta struct {
	Target pathdom.Path
	Site   effectdelta.Site
	Kind   effectdelta.Kind
	Value  effectdelta.Value
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
