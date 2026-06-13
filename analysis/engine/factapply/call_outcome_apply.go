package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func applyCallOutcomeFacts(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	out state.State,
	site factflow.CallSite,
	outcome CallOutcome,
) state.State {
	bindings := callPlaceholderBindings(facts, site)
	paramBindings := callArgumentPlaceholderBindings(facts, site)
	for _, fact := range outcome.PathRefinements {
		targetPath, ok := fact.Path.Substitute(bindings)
		if !ok {
			continue
		}
		out = applyValueRefinementAt(ctx.Registry, resolver, ctx.Point, out, targetPath, factflow.NewValueConstraint(fact.Value))
	}
	for _, fact := range outcome.ParamPathRefinements {
		targetPath, ok := fact.Path.Substitute(paramBindings)
		if !ok {
			continue
		}
		out = applyValueRefinementAt(ctx.Registry, resolver, ctx.Point, out, targetPath, factflow.NewValueConstraint(fact.Value))
	}
	for _, fact := range outcome.ParamPathInvalidations {
		targetPath, ok := fact.Path.Substitute(paramBindings)
		if !ok {
			continue
		}
		out = applyPathDescendantInvalidation(ctx, resolver, out, factflow.NewPathDescendantInvalidation(targetPath))
	}
	for _, condition := range outcome.ParamConditions {
		out = applyCallParamCondition(ctx, facts, resolver, out, site, condition)
	}
	for _, relation := range outcome.ParamPathRelations {
		out = applyCallParamPathRelation(ctx, resolver, out, paramBindings, relation)
	}
	for _, fact := range outcome.PathStaticMembers {
		targetPath, ok := fact.Path.Substitute(bindings)
		if !ok {
			continue
		}
		targetKey := factPathKeyAt(resolver, ctx.Point, targetPath)
		if targetKey == "" {
			continue
		}
		out = out.WritePathStaticMember(targetKey, fact.Value)
	}
	for _, fact := range outcome.DynamicIndexFacts {
		tablePath, ok := fact.Table.Substitute(bindings)
		if !ok {
			continue
		}
		tableKey := factPathKeyAt(resolver, ctx.Point, tablePath)
		if tableKey == "" {
			continue
		}
		out = out.WriteDynamicIndexFact(ctx.Registry, state.DynamicIndexKey{
			Table: tableKey,
			Site:  state.DynamicIndexSite(fact.Site),
		}, state.DynamicIndexFact{
			KeyPresence: fact.KeyPresence,
			KeyValue:    fact.KeyValue,
			Value:       fact.Value,
			Admission:   callDynamicIndexAdmission(fact.Admission),
		})
	}
	for _, proof := range outcome.BranchProofs {
		stateProof, ok := callBranchProofAt(resolver, ctx.Point, bindings, proof)
		if !ok {
			continue
		}
		out = out.AddBranchProof(stateProof)
	}
	for _, event := range outcome.ChannelSelects {
		fact, ok := callChannelSelectFactAt(resolver, ctx.Point, bindings, event)
		if !ok {
			continue
		}
		out = out.AddChannelSelectFact(fact)
	}
	for _, delta := range outcome.EffectDeltas {
		targetPath, ok := delta.Target.Substitute(bindings)
		if !ok {
			continue
		}
		targetKey := factPathKeyAt(resolver, ctx.Point, targetPath)
		if targetKey == "" {
			continue
		}
		out = out.WriteEffectDelta(effectdelta.Key{
			Target: targetKey,
			Site:   delta.Site,
			Kind:   delta.Kind,
		}, delta.Value)
	}
	return out
}

func applyCallParamCondition(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	out state.State,
	site factflow.CallSite,
	condition CallParamCondition,
) state.State {
	args := site.ArgumentSources()
	if condition.ParamIndex < 0 || condition.ParamIndex >= len(args) {
		return out
	}
	arg := args[condition.ParamIndex]
	if arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr {
		return out
	}
	expressionCondition, ok := facts.ExpressionCondition(arg.ExprRef)
	if !ok {
		return out
	}
	for _, refinement := range expressionCondition.RefinementsForValue(condition.Value) {
		out = applyPostconditionRefinement(ctx, resolver, out, refinement)
	}
	for _, relation := range expressionCondition.PathRelationsForValue(condition.Value) {
		out = applyPostconditionPathRelation(ctx, resolver, out, relation)
	}
	return out
}

func applyCallParamPathRelation(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	bindings []pathdom.Path,
	relation CallParamPathRelation,
) state.State {
	switch relation.Kind {
	case CallPathRelationEqual:
		left, ok := relation.Left.Substitute(bindings)
		if !ok {
			return out
		}
		right, ok := relation.Right.Substitute(bindings)
		if !ok || left.Equal(right) {
			return out
		}
		return applyPathEqualityAt(ctx.Registry, resolver, ctx.Point, out, left, right)
	default:
		return out
	}
}

func callPlaceholderBindings(facts factflow.Facts, site factflow.CallSite) []pathdom.Path {
	var bindings []pathdom.Path
	offset := 0
	if receiverPath, ok := site.ReceiverPath(); ok {
		bindings = bindPlaceholderPath(bindings, 0, receiverPath)
		offset = 1
	}
	for i, source := range site.ArgumentSources() {
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			continue
		}
		sourcePath, ok := facts.ExpressionPath(source.ExprRef)
		if !ok || sourcePath.IsEmpty() {
			continue
		}
		bindings = bindPlaceholderPath(bindings, i+offset, sourcePath)
	}
	return bindings
}

func callArgumentPlaceholderBindings(facts factflow.Facts, site factflow.CallSite) []pathdom.Path {
	var bindings []pathdom.Path
	for i, source := range site.ArgumentSources() {
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			continue
		}
		sourcePath, ok := facts.ExpressionPath(source.ExprRef)
		if !ok || sourcePath.IsEmpty() {
			continue
		}
		bindings = bindPlaceholderPath(bindings, i, sourcePath)
	}
	return bindings
}

func bindPlaceholderPath(bindings []pathdom.Path, index int, p pathdom.Path) []pathdom.Path {
	if index < 0 || p.IsEmpty() {
		return bindings
	}
	for len(bindings) <= index {
		bindings = append(bindings, pathdom.Path{})
	}
	bindings[index] = p
	return bindings
}

func callDynamicIndexAdmission(admission CallDynamicIndexAdmission) state.DynamicIndexAdmission {
	switch admission {
	case CallDynamicIndexAdmissionAdmitted:
		return state.DynamicIndexAdmissionAdmitted
	case CallDynamicIndexAdmissionRejected:
		return state.DynamicIndexAdmissionRejected
	case CallDynamicIndexAdmissionUnknown:
		return state.DynamicIndexAdmissionUnknown
	default:
		return state.DynamicIndexAdmissionBottom
	}
}

func callBranchProofAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	bindings []pathdom.Path,
	proof CallBranchProof,
) (state.BranchProof, bool) {
	targetPath, ok := proof.Path.Substitute(bindings)
	if !ok {
		return state.BranchProof{}, false
	}
	pathKey := factPathKeyAt(resolver, point, targetPath)
	if pathKey == "" {
		return state.BranchProof{}, false
	}
	switch proof.Kind {
	case CallBranchProofPathPresence:
		return state.BranchProof{
			Kind:     state.BranchProofPathPresence,
			Path:     pathKey,
			Presence: proof.Presence,
		}, true
	case CallBranchProofPathEqual, CallBranchProofPathNotEqual:
		otherPath, ok := proof.Other.Substitute(bindings)
		if !ok {
			return state.BranchProof{}, false
		}
		otherKey := factPathKeyAt(resolver, point, otherPath)
		if otherKey == "" {
			return state.BranchProof{}, false
		}
		return state.BranchProof{
			Kind:  callBranchProofKind(proof.Kind),
			Path:  pathKey,
			Other: otherKey,
		}, true
	default:
		return state.BranchProof{}, false
	}
}

func callBranchProofKind(kind CallBranchProofKind) state.BranchProofKind {
	switch kind {
	case CallBranchProofPathEqual:
		return state.BranchProofPathEqual
	case CallBranchProofPathNotEqual:
		return state.BranchProofPathNotEqual
	default:
		return state.BranchProofPathPresence
	}
}

func callChannelSelectFactAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	bindings []pathdom.Path,
	event CallChannelSelectFact,
) (state.ChannelSelectFact, bool) {
	kind, ok := callChannelSelectKind(event.Kind)
	if !ok {
		return state.ChannelSelectFact{}, false
	}
	fact := state.ChannelSelectFact{
		Select: state.ChannelSelectID(event.Select),
		Kind:   kind,
		Index:  event.Index,
	}
	if !event.Result.IsEmpty() {
		resultPath, ok := event.Result.Substitute(bindings)
		if !ok {
			return state.ChannelSelectFact{}, false
		}
		fact.Result = factPathKeyAt(resolver, point, resultPath)
		if fact.Result == "" {
			return state.ChannelSelectFact{}, false
		}
	}
	if !event.Case.IsEmpty() {
		casePath, ok := event.Case.Substitute(bindings)
		if !ok {
			return state.ChannelSelectFact{}, false
		}
		fact.Case = factPathKeyAt(resolver, point, casePath)
		if fact.Case == "" {
			return state.ChannelSelectFact{}, false
		}
	}
	return fact, true
}

func callChannelSelectKind(kind CallChannelSelectFactKind) (state.ChannelSelectFactKind, bool) {
	switch kind {
	case CallChannelSelectFactSelect:
		return state.ChannelSelectFactSelect, true
	case CallChannelSelectFactReceive:
		return state.ChannelSelectFactReceive, true
	case CallChannelSelectFactCase:
		return state.ChannelSelectFactCase, true
	default:
		return 0, false
	}
}
