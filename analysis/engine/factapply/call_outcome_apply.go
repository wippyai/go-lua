package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func applyCallOutcomeFacts(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	site factflow.CallSiteView,
	outcome CallOutcome,
) state.State {
	bindings := callPlaceholderBindings(facts, site)
	paramBindings := callArgumentPlaceholderBindings(facts, site)
	normalReturnFacts := outcome.NormalReturnFacts
	for id, object := range outcome.HeapTableObjects {
		out = out.WriteHeapTableObject(ctx.Registry, id, object)
	}
	for _, fact := range normalReturnFacts.PathRefinements {
		targetPath, ok := fact.Path.Substitute(bindings)
		if !ok {
			continue
		}
		out = applyValueRefinementAt(ctx.Registry, resolver, projectPath, ctx.Point, out, targetPath, factflow.NewValueConstraint(fact.Value))
	}
	for _, fact := range outcome.ParamPathRefinements {
		targetPath, ok := fact.Path.Substitute(paramBindings)
		if !ok {
			continue
		}
		out = applyValueRefinementAt(ctx.Registry, resolver, projectPath, ctx.Point, out, targetPath, factflow.NewValueConstraint(fact.Value))
	}
	for _, fact := range outcome.ParamPathInvalidations {
		targetPath, ok := fact.Path.Substitute(paramBindings)
		if !ok {
			continue
		}
		out = applyPathDescendantInvalidation(ctx, resolver, out, factflow.NewPathDescendantInvalidation(targetPath))
	}
	for _, condition := range outcome.ParamConditions {
		out = applyCallParamCondition(ctx, facts, resolver, projectPath, out, site, condition)
	}
	for _, relation := range outcome.ParamPathRelations {
		out = applyCallParamPathRelation(ctx, resolver, projectPath, out, paramBindings, relation)
	}
	for _, fact := range normalReturnFacts.PathStaticMembers {
		targetKey, ok := callOutcomePathKeyAt(resolver, ctx.Point, bindings, fact.Path)
		if !ok {
			continue
		}
		out = out.WritePathStaticMember(targetKey, fact.Value)
	}
	for _, fact := range normalReturnFacts.DynamicIndexFacts {
		tableKey, ok := callOutcomePathKeyAt(resolver, ctx.Point, bindings, fact.Table)
		if !ok {
			continue
		}
		out = out.WriteDynamicIndexFact(ctx.Registry, dynamicindex.Key{
			Table: tableKey,
			Site:  fact.Site,
		}, fact.Value)
	}
	for _, proof := range normalReturnFacts.BranchProofs {
		stateProof, ok := callBranchProofAt(resolver, ctx.Point, bindings, proof)
		if !ok {
			continue
		}
		out = out.AddBranchProof(stateProof)
	}
	for _, event := range normalReturnFacts.ChannelSelects {
		fact, ok := callChannelSelectFactAt(resolver, ctx.Point, bindings, event)
		if !ok {
			continue
		}
		out = out.AddChannelSelectFact(fact)
	}
	for _, fact := range normalReturnFacts.FrozenTables {
		targetPath, ok := fact.Target.Substitute(bindings)
		if !ok {
			continue
		}
		if targetKey := factPathKeyAt(resolver, ctx.Point, targetPath); targetKey != "" {
			out = out.WriteEffectDelta(effectdelta.Key{
				Target: targetKey,
				Site:   callboundary.FrozenTableEffectSite(),
				Kind:   effectdelta.Freeze,
			}, effectdelta.Top())
		}
		out = applyFrozenTableFact(ctx.Registry, resolver, projectPath, ctx.Point, out, targetPath)
	}
	for _, delta := range normalReturnFacts.EffectDeltas {
		targetKey, ok := callOutcomePathKeyAt(resolver, ctx.Point, bindings, delta.Target)
		if !ok {
			continue
		}
		out = out.WriteEffectDelta(effectdelta.Key{
			Target: targetKey,
			Site:   delta.Site,
			Kind:   delta.Kind,
		}, delta.Value)
	}
	for _, event := range normalReturnFacts.EscapeEvents {
		targetPath, ok := event.Target.Substitute(bindings)
		if !ok {
			continue
		}
		targetKey := factPathKeyAt(resolver, ctx.Point, targetPath)
		if targetKey == "" {
			continue
		}
		out = out.WriteEffectDelta(effectdelta.Key{
			Target: targetKey,
			Site:   callboundary.EscapeEventEffectSite(event.Kind, event.Recursive),
			Kind:   effectdelta.Escape,
		}, effectdelta.Top())
		out = applyEscapeEventPlacement(ctx.Registry, resolver, projectPath, ctx.Point, out, targetPath, event)
	}
	return out
}

func applyFrozenTableFact(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
) state.State {
	target, ok := resolvePathValueAt(reg, resolver, point, out, targetPath, projectPath)
	if !ok {
		return out
	}
	id, ok := product.Get(reg, target.value, identity.Key).ID()
	if !ok {
		return out
	}
	return out.FreezeTable(id)
}

func applyEscapeEventPlacement(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	event callboundary.EscapeEventFact,
) state.State {
	value, ok := escapeEventPlacement(event.Kind)
	if !ok {
		return out
	}
	target, ok := resolvePathValueAt(reg, resolver, point, out, targetPath, projectPath)
	if !ok {
		return out
	}
	id, ok := product.Get(reg, target.value, identity.Key).ID()
	if !ok {
		return out
	}
	if !event.Recursive {
		return writeJoinedPlacement(out, id, value)
	}
	return markReachableHeapPlacement(reg, out, id, value, map[identity.ID]struct{}{})
}

func escapeEventPlacement(kind callboundary.EscapeEventKind) (placement.Value, bool) {
	switch kind {
	case callboundary.EscapeEventSend, callboundary.EscapeEventExport, callboundary.EscapeEventOpaque:
		return placement.SharedHeap, true
	case callboundary.EscapeEventStore, callboundary.EscapeEventRetain:
		return placement.OwnedHeap, true
	default:
		return placement.Bottom, false
	}
}

func markReachableHeapPlacement(
	reg *axis.Registry,
	out state.State,
	id identity.ID,
	value placement.Value,
	seen map[identity.ID]struct{},
) state.State {
	if id == (identity.ID{}) {
		return out
	}
	if _, ok := seen[id]; ok {
		return out
	}
	seen[id] = struct{}{}
	out = writeJoinedPlacement(out, id, value)
	object := out.ReadHeapTableObject(reg, id)
	objectDomain := heapidentity.ObjectDomain(reg)
	if objectDomain.Equal(object, objectDomain.Bottom()) {
		return out
	}
	out = markReachableHeapValuePlacement(reg, out, object.Root(), value, seen)
	for _, member := range object.StaticMembers() {
		out = markReachableHeapValuePlacement(reg, out, member, value, seen)
	}
	for _, fact := range object.DynamicIndexFacts() {
		out = markReachableHeapValuePlacement(reg, out, fact.KeyValue, value, seen)
		out = markReachableHeapValuePlacement(reg, out, fact.Value, value, seen)
	}
	return out
}

func markReachableHeapValuePlacement(
	reg *axis.Registry,
	out state.State,
	value product.Value,
	target placement.Value,
	seen map[identity.ID]struct{},
) state.State {
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok {
		return out
	}
	return markReachableHeapPlacement(reg, out, id, target, seen)
}

func writeJoinedPlacement(out state.State, id identity.ID, value placement.Value) state.State {
	if id == (identity.ID{}) {
		return out
	}
	return out.WritePlacement(id, placement.Join(out.ReadPlacement(id), value))
}

func callOutcomePathKeyAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	bindings []pathdom.Path,
	path pathdom.Path,
) (pathdom.PathKey, bool) {
	targetPath, ok := path.Substitute(bindings)
	if !ok {
		return "", false
	}
	targetKey := factPathKeyAt(resolver, point, targetPath)
	if targetKey == "" {
		return "", false
	}
	return targetKey, true
}

func applyCallParamCondition(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	site factflow.CallSiteView,
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
		out = applyValueRefinementAt(ctx.Registry, resolver, projectPath, ctx.Point, out, refinement.TargetPath(), refinement.Value())
	}
	for _, relation := range expressionCondition.PathRelationsForValue(condition.Value) {
		out = applyPostconditionPathRelation(ctx, resolver, projectPath, out, relation)
	}
	return out
}

func applyCallParamPathRelation(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
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
		return applyPathEqualityAt(ctx.Registry, resolver, projectPath, ctx.Point, out, left, right)
	default:
		return out
	}
}

func callPlaceholderBindings(facts factflow.Facts, site factflow.CallSiteView) []pathdom.Path {
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

func callArgumentPlaceholderBindings(facts factflow.Facts, site factflow.CallSiteView) []pathdom.Path {
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

func callBranchProofAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	bindings []pathdom.Path,
	proof callboundary.BranchProof,
) (pathevidence.BranchProof, bool) {
	pathKey, ok := callOutcomePathKeyAt(resolver, point, bindings, proof.Path)
	if !ok {
		return pathevidence.BranchProof{}, false
	}
	switch proof.Kind {
	case pathevidence.BranchProofPathPresence:
		return pathevidence.BranchProof{
			Kind:     pathevidence.BranchProofPathPresence,
			Path:     pathKey,
			Presence: proof.Presence,
		}, true
	case pathevidence.BranchProofPathEqual, pathevidence.BranchProofPathNotEqual:
		otherKey, ok := callOutcomePathKeyAt(resolver, point, bindings, proof.Other)
		if !ok {
			return pathevidence.BranchProof{}, false
		}
		return pathevidence.BranchProof{
			Kind:  proof.Kind,
			Path:  pathKey,
			Other: otherKey,
		}, true
	default:
		return pathevidence.BranchProof{}, false
	}
}

func callChannelSelectFactAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	bindings []pathdom.Path,
	event callboundary.ChannelSelectFact,
) (channelselectfact.Fact, bool) {
	switch event.Kind {
	case channelselectfact.FactSelect, channelselectfact.FactReceive, channelselectfact.FactCase:
	default:
		return channelselectfact.Fact{}, false
	}
	fact := channelselectfact.Fact{
		Select:     event.Select,
		Kind:       event.Kind,
		Index:      event.Index,
		HasDefault: event.HasDefault,
	}
	if !event.Result.IsEmpty() {
		resultKey, ok := callOutcomePathKeyAt(resolver, point, bindings, event.Result)
		if !ok {
			return channelselectfact.Fact{}, false
		}
		fact.Result = resultKey
	}
	if !event.Case.IsEmpty() {
		caseKey, ok := callOutcomePathKeyAt(resolver, point, bindings, event.Case)
		if !ok {
			return channelselectfact.Fact{}, false
		}
		fact.Case = caseKey
	}
	return fact, true
}
