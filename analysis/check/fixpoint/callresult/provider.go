// Package callresult adapts fixpoint summaries into factflow call outcomes.
package callresult

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// KeyFunc maps one call producer in context to an exact summary key.
type KeyFunc func(ctx transfer.NodeContext, call factflow.CallProducer) (summary.SummaryKey, bool)

// OutcomeProvider returns a generic call-boundary outcome provider backed by
// exact summary reads.
func OutcomeProvider(summaries summary.Reader, keyFor KeyFunc) factapply.CallOutcomeProvider {
	return func(ctx transfer.NodeContext, site factflow.CallSite, _ state.State, _ func(cfg.Point) state.State) factapply.CallOutcome {
		if summaries == nil || keyFor == nil {
			return factapply.CallOutcome{}
		}
		key, ok := keyFor(ctx, factflow.CallProducerFromSite(site))
		if !ok {
			return factapply.CallOutcome{}
		}
		got, ok := summaries.Read(key)
		if !ok {
			return factapply.CallOutcome{}
		}
		return outcomeFromSummary(got, func(index int) bool {
			if index < 0 || index >= len(got.NormalReturnParams) {
				return false
			}
			return summary.UsefulNormalReturnParam(ctx.Registry, got.NormalReturnParams[index])
		})
	}
}

func outcomeFromSummary(got summary.Summary, usefulNormalReturnParam func(int) bool) factapply.CallOutcome {
	out := factapply.CallOutcome{}
	if len(got.Returns) != 0 {
		out.Results = make([]factapply.CallResult, len(got.Returns))
		for i, value := range got.Returns {
			out.Results[i] = factapply.CallResult{Index: i, Value: value}
		}
	}
	for i, value := range got.NormalReturnParams {
		if usefulNormalReturnParam == nil || !usefulNormalReturnParam(i) {
			continue
		}
		out.ParamPathRefinements = append(out.ParamPathRefinements, factapply.CallParamPathRefinement{
			Path:  pathdom.NewPlaceholder(i),
			Value: value,
		})
	}
	for i, condition := range got.NormalReturnParamConditions {
		value, ok := paramConditionValue(condition)
		if !ok {
			continue
		}
		out.ParamConditions = append(out.ParamConditions, factapply.CallParamCondition{
			ParamIndex: i,
			Value:      value,
		})
	}
	for _, equality := range got.NormalReturnParamEqualities {
		if equality.Left < 0 || equality.Right < 0 || equality.Left == equality.Right {
			continue
		}
		out.ParamPathRelations = append(out.ParamPathRelations, factapply.CallParamPathRelation{
			Kind:  factapply.CallPathRelationEqual,
			Left:  pathdom.NewPlaceholder(equality.Left),
			Right: pathdom.NewPlaceholder(equality.Right),
		})
	}
	facts := got.NormalReturnFacts
	if len(facts.PathRefinements) != 0 {
		for _, fact := range facts.PathRefinements {
			out.PathRefinements = append(out.PathRefinements, factapply.CallPathRefinement{
				Path:  copyPath(fact.Path),
				Value: fact.Value,
			})
		}
	}
	if len(facts.PathStaticMembers) != 0 {
		out.PathStaticMembers = make([]factapply.CallPathStaticMember, len(facts.PathStaticMembers))
		for i, fact := range facts.PathStaticMembers {
			out.PathStaticMembers[i] = factapply.CallPathStaticMember{
				Path:  copyPath(fact.Path),
				Value: fact.Value,
			}
		}
	}
	if len(facts.DynamicIndexFacts) != 0 {
		out.DynamicIndexFacts = make([]factapply.CallDynamicIndexFact, len(facts.DynamicIndexFacts))
		for i, fact := range facts.DynamicIndexFacts {
			out.DynamicIndexFacts[i] = factapply.CallDynamicIndexFact{
				Table:       copyPath(fact.Table),
				Site:        fact.Site,
				KeyPresence: fact.KeyPresence,
				KeyValue:    fact.KeyValue,
				Value:       fact.Value,
				Admission:   dynamicIndexAdmission(fact.Admission),
			}
		}
	}
	if len(facts.BranchProofs) != 0 {
		out.BranchProofs = make([]factapply.CallBranchProof, len(facts.BranchProofs))
		for i, proof := range facts.BranchProofs {
			out.BranchProofs[i] = factapply.CallBranchProof{
				Kind:     branchProofKind(proof.Kind),
				Path:     copyPath(proof.Path),
				Presence: proof.Presence,
				Other:    copyPath(proof.Other),
			}
		}
	}
	if len(facts.ChannelSelects) != 0 {
		out.ChannelSelects = make([]factapply.CallChannelSelectFact, len(facts.ChannelSelects))
		for i, fact := range facts.ChannelSelects {
			out.ChannelSelects[i] = factapply.CallChannelSelectFact{
				Select: fact.Select,
				Kind:   channelSelectKind(fact.Kind),
				Result: copyPath(fact.Result),
				Case:   copyPath(fact.Case),
				Index:  fact.Index,
			}
		}
	}
	if len(facts.EffectDeltas) != 0 {
		out.EffectDeltas = make([]factapply.CallEffectDelta, len(facts.EffectDeltas))
		for i, delta := range facts.EffectDeltas {
			out.EffectDeltas[i] = factapply.CallEffectDelta{
				Target: copyPath(delta.Target),
				Site:   delta.Site,
				Kind:   effectDeltaKind(delta.Kind),
				Before: delta.Before,
				After:  delta.After,
				Change: effectDeltaChange(delta.Change),
			}
		}
	}
	if len(got.ReturnConditionParamRefinements) != 0 {
		out.ReturnConditionRefinements = make([]factapply.CallReturnConditionRefinement, len(got.ReturnConditionParamRefinements))
		for i, refinement := range got.ReturnConditionParamRefinements {
			out.ReturnConditionRefinements[i] = factapply.CallReturnConditionRefinement{
				ReturnIndex: refinement.ReturnIndex,
				ReturnValue: refinement.ReturnValue,
				Target:      copyPath(refinement.Target),
				Value:       refinement.Value,
			}
		}
	}
	if len(got.ReturnPresenceRelations) != 0 {
		out.ReturnPresenceRelations = make([]factapply.CallReturnPresenceRelation, len(got.ReturnPresenceRelations))
		for i, relation := range got.ReturnPresenceRelations {
			out.ReturnPresenceRelations[i] = factapply.CallReturnPresenceRelation{
				TriggerIndex:    relation.TriggerIndex,
				TriggerPresence: relation.TriggerPresence,
				TargetIndex:     relation.TargetIndex,
				TargetPresence:  relation.TargetPresence,
			}
		}
	}
	return out
}

func paramConditionValue(condition summary.ParamCondition) (bool, bool) {
	switch condition {
	case summary.ParamConditionTruthy:
		return true, true
	case summary.ParamConditionFalsy:
		return false, true
	default:
		return false, false
	}
}

// WithSupplementalResults composes two call outcome providers. Result slots are
// primary-by-index; all non-slot side facts are accumulated.
func WithSupplementalResults(primary, supplemental factapply.CallOutcomeProvider) factapply.CallOutcomeProvider {
	if primary == nil {
		return supplemental
	}
	if supplemental == nil {
		return primary
	}
	return func(ctx transfer.NodeContext, site factflow.CallSite, in state.State, read func(cfg.Point) state.State) factapply.CallOutcome {
		out := primary(ctx, site, in, read)
		second := supplemental(ctx, site, in, read)
		out = withSupplementalResultSlots(out, second.Results)
		return withSupplementalOutcomeFacts(out, second)
	}
}

func withSupplementalResultSlots(out factapply.CallOutcome, results []factapply.CallResult) factapply.CallOutcome {
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

func withSupplementalOutcomeFacts(out, second factapply.CallOutcome) factapply.CallOutcome {
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

func dynamicIndexAdmission(admission summary.DynamicIndexAdmission) factapply.CallDynamicIndexAdmission {
	switch admission {
	case summary.DynamicIndexAdmissionAdmitted:
		return factapply.CallDynamicIndexAdmissionAdmitted
	case summary.DynamicIndexAdmissionRejected:
		return factapply.CallDynamicIndexAdmissionRejected
	case summary.DynamicIndexAdmissionUnknown:
		return factapply.CallDynamicIndexAdmissionUnknown
	default:
		return factapply.CallDynamicIndexAdmissionBottom
	}
}

func branchProofKind(kind summary.BranchProofKind) factapply.CallBranchProofKind {
	switch kind {
	case summary.BranchProofPathPresence:
		return factapply.CallBranchProofPathPresence
	case summary.BranchProofPathEqual:
		return factapply.CallBranchProofPathEqual
	case summary.BranchProofPathNotEqual:
		return factapply.CallBranchProofPathNotEqual
	default:
		return 0
	}
}

func channelSelectKind(kind summary.ChannelSelectFactKind) factapply.CallChannelSelectFactKind {
	switch kind {
	case summary.ChannelSelectFactSelect:
		return factapply.CallChannelSelectFactSelect
	case summary.ChannelSelectFactReceive:
		return factapply.CallChannelSelectFactReceive
	case summary.ChannelSelectFactCase:
		return factapply.CallChannelSelectFactCase
	default:
		return 0
	}
}

func effectDeltaKind(kind summary.EffectDeltaKind) factapply.CallEffectDeltaKind {
	switch kind {
	case summary.EffectDeltaMutation:
		return factapply.CallEffectDeltaMutation
	case summary.EffectDeltaEscape:
		return factapply.CallEffectDeltaEscape
	case summary.EffectDeltaCall:
		return factapply.CallEffectDeltaCall
	default:
		return 0
	}
}

func effectDeltaChange(change summary.EffectDeltaChange) factapply.CallEffectDeltaChange {
	switch change {
	case summary.EffectDeltaChangeNone:
		return factapply.CallEffectDeltaChangeNone
	case summary.EffectDeltaChangeChanged:
		return factapply.CallEffectDeltaChangeChanged
	case summary.EffectDeltaChangeUnknown:
		return factapply.CallEffectDeltaChangeUnknown
	default:
		return factapply.CallEffectDeltaChangeBottom
	}
}

func copyPath(p pathdom.Path) pathdom.Path {
	if len(p.Segments) == 0 {
		return p
	}
	out := p
	out.Segments = append(p.Segments[:0:0], p.Segments...)
	return out
}

// ByCalleeSymbol maps callee symbol IDs to exact summary keys.
func ByCalleeSymbol(keys map[symbol.ID]summary.SummaryKey) KeyFunc {
	cloned := make(map[symbol.ID]summary.SummaryKey, len(keys))
	for id, key := range keys {
		cloned[id] = key
	}
	return func(_ transfer.NodeContext, call factflow.CallProducer) (summary.SummaryKey, bool) {
		key, ok := cloned[call.CalleeSymbol()]
		return key, ok
	}
}

// ByCalleeIdentity maps direct callee symbols and exact callee access paths to
// summary keys. Symbol keys are checked first because direct locals are the
// narrowest identity for function values.
func ByCalleeIdentity(symbolKeys map[symbol.ID]summary.SummaryKey, pathKeys map[pathdom.PathKey]summary.SummaryKey) KeyFunc {
	clonedSymbols := make(map[symbol.ID]summary.SummaryKey, len(symbolKeys))
	for id, key := range symbolKeys {
		clonedSymbols[id] = key
	}
	clonedPaths := make(map[pathdom.PathKey]summary.SummaryKey, len(pathKeys))
	for pathKey, key := range pathKeys {
		clonedPaths[pathKey] = key
	}
	return func(_ transfer.NodeContext, call factflow.CallProducer) (summary.SummaryKey, bool) {
		if key, ok := clonedSymbols[call.CalleeSymbol()]; ok {
			return key, true
		}
		calleePath := call.CalleePath()
		if calleePath.IsEmpty() {
			return summary.SummaryKey{}, false
		}
		key, ok := clonedPaths[calleePath.Key()]
		return key, ok
	}
}
