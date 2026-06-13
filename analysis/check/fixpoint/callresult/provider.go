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
				Table: copyPath(fact.Table),
				Site:  fact.Site,
				Value: fact.Value,
			}
		}
	}
	if len(facts.BranchProofs) != 0 {
		out.BranchProofs = make([]factapply.CallBranchProof, len(facts.BranchProofs))
		for i, proof := range facts.BranchProofs {
			out.BranchProofs[i] = factapply.CallBranchProof{
				Kind:     proof.Kind,
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
				Kind:   fact.Kind,
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
				Kind:   delta.Kind,
				Value:  delta.Value,
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

func copyPath(p pathdom.Path) pathdom.Path {
	if len(p.Segments) == 0 {
		return p
	}
	out := p
	out.Segments = append(p.Segments[:0:0], p.Segments...)
	return out
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
