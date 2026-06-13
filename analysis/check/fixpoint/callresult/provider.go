// Package callresult adapts fixpoint summaries into factflow call outcomes.
package callresult

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/discriminant"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// KeyFunc maps one call producer in context to an exact summary key.
type KeyFunc func(ctx transfer.NodeContext, call factflow.CallProducer) (summary.SummaryKey, bool)

// ProviderConfig configures summary-backed call outcomes.
type ProviderConfig struct {
	Summaries     summary.Reader
	KeyFor        KeyFunc
	FunctionTypes map[summary.SummaryKey]*typ.Function
	Sources       sourcevalue.SourceValues
}

// OutcomeProvider returns a generic call-boundary outcome provider backed by
// exact summary reads.
func OutcomeProvider(config ProviderConfig) factapply.CallOutcomeProvider {
	summaries := config.Summaries
	keyFor := config.KeyFor
	functionTypes := cloneFunctionTypes(config.FunctionTypes)
	sources := config.Sources
	return func(ctx transfer.NodeContext, site factflow.CallSite, in state.State, read func(cfg.Point) state.State) factapply.CallOutcome {
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
		got = specializeGenericSummary(ctx, site, got, functionTypes[key], sources, in, read)
		return outcomeFromSummary(got, func(index int) bool {
			if index < 0 || index >= len(got.NormalReturnParams) {
				return false
			}
			return summary.UsefulNormalReturnParam(ctx.Registry, got.NormalReturnParams[index])
		})
	}
}

func specializeGenericSummary(
	ctx transfer.NodeContext,
	site factflow.CallSite,
	got summary.Summary,
	fn *typ.Function,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) summary.Summary {
	if ctx.Registry == nil || fn == nil || len(fn.TypeParams) == 0 || sources == nil {
		return got
	}
	args := callArgumentTypes(ctx, site, sources, in, read)
	if len(args) == 0 {
		return got
	}
	instantiated, violations := typecall.InstantiateGenericCall(fn, args)
	if len(violations) != 0 || instantiated == nil || instantiated == fn {
		return got
	}
	return specializeSummaryReturns(ctx.Registry, got, instantiated.Returns)
}

func callArgumentTypes(
	ctx transfer.NodeContext,
	site factflow.CallSite,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) []typ.Type {
	if sources == nil {
		return nil
	}
	argSources := site.ArgumentSources()
	if len(argSources) == 0 {
		return nil
	}
	args := make([]typ.Type, len(argSources))
	seen := false
	for i, source := range argSources {
		value, ok := sources.ValueOfSource(ctx.Point, source, in, read)
		if !ok {
			continue
		}
		t, ok := typeFromValue(ctx.Registry, value)
		if !ok {
			continue
		}
		args[i] = t
		seen = true
	}
	if !seen {
		return nil
	}
	return args
}

func typeFromValue(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	if reg == nil {
		return nil, false
	}
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		if t, ok := witness.Type(); ok {
			return t, true
		}
	}
	origin := product.Get(reg, value, variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		return nil, false
	}
	return discriminant.TypeFromOrigin(origin.Family(), origin.Cases())
}

func specializeSummaryReturns(reg *axis.Registry, got summary.Summary, returns []typ.Type) summary.Summary {
	if reg == nil || len(got.Returns) == 0 || len(returns) == 0 {
		return got
	}
	returns = callResultReturnTypes(got, returns)
	nextReturns := make([]product.Value, len(got.Returns))
	copy(nextReturns, got.Returns)
	changed := false
	for i := range nextReturns {
		if i >= len(returns) {
			break
		}
		ret := returns[i]
		if ret == nil || typ.IsAny(ret) || typ.IsUnknown(ret) || refinement.ContainsFreeTypeParam(ret) {
			continue
		}
		declared := typevalue.WithWitness(reg, typevalue.FromType(reg, ret), ret)
		next := joinInstantiatedReturnValue(reg, nextReturns[i], declared)
		if product.Equal(reg, nextReturns[i], next) {
			continue
		}
		nextReturns[i] = next
		changed = true
	}
	if !changed {
		return got
	}
	out := got.Clone()
	out.Returns = nextReturns
	return summary.Normalize(reg, out)
}

func callResultReturnTypes(got summary.Summary, returns []typ.Type) []typ.Type {
	if len(returns) == 1 && len(got.Returns) > 1 {
		if tuple, ok := returns[0].(*typ.Tuple); ok {
			return append([]typ.Type(nil), tuple.Elements...)
		}
	}
	return returns
}

func joinInstantiatedReturnValue(reg *axis.Registry, value product.Value, declared product.Value) product.Value {
	joined := product.Join(reg, value, declared)
	declaredWitness := product.Get(reg, declared, typewitness.Key)
	if !declaredWitness.IsBottom() && !declaredWitness.IsTop() {
		joinedWitness := product.Get(reg, joined, typewitness.Key)
		if joinedWitness.IsTop() {
			joined = product.Set(reg, joined, typewitness.Key, declaredWitness)
		}
	}
	declaredOrigin := product.Get(reg, declared, variantorigin.Key)
	if declaredOrigin.IsBottom() || declaredOrigin.IsTop() {
		return joined
	}
	joinedOrigin := product.Get(reg, joined, variantorigin.Key)
	if !joinedOrigin.IsTop() && !originContainsFreeTypeParam(joinedOrigin) {
		return joined
	}
	return product.Set(reg, joined, variantorigin.Key, declaredOrigin)
}

func originContainsFreeTypeParam(origin variantorigin.Value) bool {
	if origin.IsBottom() || origin.IsTop() {
		return false
	}
	t, ok := discriminant.TypeFromOrigin(origin.Family(), origin.Cases())
	return ok && refinement.ContainsFreeTypeParam(t)
}

func cloneFunctionTypes(in map[summary.SummaryKey]*typ.Function) map[summary.SummaryKey]*typ.Function {
	if len(in) == 0 {
		return nil
	}
	out := make(map[summary.SummaryKey]*typ.Function, len(in))
	for key, fn := range in {
		out[key] = fn
	}
	return out
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
