package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
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
	widen CovariantWiden,
	out state.State,
	site factflow.CallSiteView,
	outcome callpayload.CallOutcome,
) state.State {
	bindings := callPlaceholderBindings(facts, site)
	paramBindings := callArgumentPlaceholderBindings(facts, site)
	out = applyCallParamExposures(ctx, resolver, widen, out, paramBindings, outcome.ParamExposures)
	normalReturnFacts := outcome.NormalReturnFacts
	lengthFloors := resolveCallParamLengthFloors(resolver, ctx.Point, out, paramBindings, outcome.ParamLengthFloors)
	for id, object := range outcome.HeapTableObjects {
		out = out.WriteHeapTableObject(ctx.Registry, id, object)
	}
	for id, value := range outcome.Placements {
		out = writeJoinedPlacement(out, id, value)
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
		out = writePathInvalidationMarker(resolver, ctx.Point, out, targetPath)
		out = applyPathDescendantInvalidation(ctx, resolver, out, factflow.NewPathDescendantInvalidation(targetPath), true)
	}
	for _, fact := range normalReturnFacts.PathInvalidations {
		targetPath, ok := fact.Path.Substitute(bindings)
		if !ok {
			continue
		}
		out = writePathInvalidationMarker(resolver, ctx.Point, out, targetPath)
		out = applyPathDescendantInvalidation(ctx, resolver, out, factflow.NewPathDescendantInvalidation(targetPath), true)
		out = invalidateMutatedFieldSlot(ctx, resolver, out, targetPath)
	}
	for _, fact := range lengthFloors {
		out = applyCallParamLengthFloor(resolver, ctx.Point, out, fact.Path, fact.Floor)
	}
	for _, condition := range outcome.ParamConditions {
		out = applyCallParamCondition(ctx, facts, resolver, projectPath, out, site, condition)
	}
	for _, relation := range outcome.ParamPathRelations {
		out = applyCallParamPathRelation(ctx, resolver, projectPath, out, paramBindings, relation)
	}
	for _, fact := range normalReturnFacts.PathStaticMembers {
		targetPathKey, ok := callOutcomePathKeyAt(resolver, ctx.Point, bindings, fact.Path)
		if !ok {
			continue
		}
		out = out.WritePathStaticMember(resolver.KeySpace(), targetPathKey, fact.Value)
	}
	for _, fact := range normalReturnFacts.DynamicIndexFacts {
		tableKey, ok := callOutcomeKeyspaceKeyAt(resolver, ctx.Point, bindings, fact.Table)
		if !ok {
			continue
		}
		tablePath, ok := fact.Table.Substitute(bindings)
		if !ok {
			continue
		}
		key := dynamicindex.Key{
			Table: tableKey,
			Site:  fact.Site,
		}
		out = out.WriteDynamicIndexFact(ctx.Registry, key, fact.Value)
		out = writeHeapTableDynamicIndexFact(ctx, resolver, out, tablePath, key, fact.Value)
	}
	for _, proof := range normalReturnFacts.BranchProofs {
		stateProof, ok := callBranchProofAt(resolver, ctx.Point, bindings, proof)
		if !ok {
			continue
		}
		out = out.AddBranchProof(stateProof)
	}
	for _, fact := range normalReturnFacts.NumFloors {
		out = applyNormalReturnNumFloor(resolver, ctx.Point, bindings, out, fact)
	}
	for _, fact := range normalReturnFacts.RelConstraints {
		out = applyNormalReturnRelConstraint(resolver, ctx.Point, bindings, out, fact)
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
		if targetKey, ok := factKeyspaceKeyAt(resolver, ctx.Point, targetPath); ok {
			out = out.WriteEffectDelta(effectdelta.Key{
				Target: targetKey,
				Site:   callboundary.FrozenTableEffectSite(),
				Kind:   effectdelta.Freeze,
			}, effectdelta.Top())
		}
		out = applyFrozenTableFact(ctx.Registry, resolver, projectPath, ctx.Point, out, targetPath)
	}
	for _, delta := range normalReturnFacts.EffectDeltas {
		targetKey, ok := callOutcomeKeyspaceKeyAt(resolver, ctx.Point, bindings, delta.Target)
		if !ok {
			continue
		}
		out = out.WriteEffectDelta(effectdelta.Key{
			Target: targetKey,
			Site:   delta.Site,
			Kind:   delta.Kind,
		}, delta.Value)
	}
	for _, relation := range normalReturnFacts.StoreRelations {
		sourceStateKey, ok := callOutcomeStateKeyAt(resolver, ctx.Point, bindings, relation.Source)
		if !ok {
			continue
		}
		intoStateKey, ok := callOutcomeStateKeyAt(resolver, ctx.Point, bindings, relation.Into)
		if !ok {
			continue
		}
		out = out.AddStoreRelation(state.StoreRelation{
			Source: sourceStateKey,
			Into:   intoStateKey,
		})
	}
	for _, fact := range normalReturnFacts.LifecycleFacts {
		targetStateKey, ok := callOutcomeStateKeyAt(resolver, ctx.Point, bindings, fact.Target)
		if !ok || fact.Protocol == "" {
			continue
		}
		resource := out.CanonicalTypestateResource(resolver.KeySpace(), targetStateKey, fact.Protocol)
		switch fact.Kind {
		case callboundary.LifecycleAcquire:
			out = out.AcquireTypestate(resource, fact.To, fact.Obligation)
		case callboundary.LifecycleTransition:
			out = out.TransitionTypestate(resource, fact.From, fact.To)
		case callboundary.LifecycleEscape:
			out = out.EscapeTypestate(resource)
		}
	}
	for _, event := range normalReturnFacts.EscapeEvents {
		targetPath, ok := event.Target.Substitute(bindings)
		if !ok {
			continue
		}
		targetStateKey, ok := factStateKeyAt(resolver, ctx.Point, targetPath)
		if !ok {
			continue
		}
		out = out.AddEscapeEvent(state.EscapeEvent{
			Target:    targetStateKey,
			Kind:      event.Kind,
			Recursive: event.Recursive,
		})
		out = applyEscapeEventPlacement(ctx.Registry, resolver, projectPath, ctx.Point, out, targetPath, event)
	}
	return out
}

func applyNormalReturnNumFloor(
	resolver *visibility.Resolver,
	point cfg.Point,
	bindings []pathdom.Path,
	out state.State,
	fact callboundary.NumFloorFact,
) state.State {
	targetPath, ok := fact.Path.Substitute(bindings)
	if !ok || targetPath.Symbol == 0 {
		return out
	}
	pathKey, ok := visibility.RootOrVisibleStateKeyAt(resolver, point, targetPath)
	if !ok {
		return out
	}
	return out.WriteNumFloor(resolver.KeySpace(), pathKey, fact.Floor)
}

func applyNormalReturnRelConstraint(
	resolver *visibility.Resolver,
	point cfg.Point,
	bindings []pathdom.Path,
	out state.State,
	fact callboundary.RelConstraintFact,
) state.State {
	aKey, ok := callRelationGraphKeyAt(resolver, point, bindings, fact.A)
	if !ok {
		return out
	}
	cKey, ok := callRelationGraphKeyAt(resolver, point, bindings, fact.C)
	if !ok {
		return out
	}
	var bKey state.RelOperand
	coB := fact.CoB
	if coB != 0 && !fact.B.Path.IsEmpty() {
		bKey, ok = callRelationGraphKeyAt(resolver, point, bindings, fact.B)
		if !ok {
			return out
		}
	} else {
		coB = 0
	}
	return out.WriteScaledConstraint(fact.CoA, aKey, coB, bKey, cKey, fact.K)
}

func callRelationGraphKeyAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	bindings []pathdom.Path,
	operand callboundary.RelOperand,
) (state.RelOperand, bool) {
	targetPath, ok := operand.Path.Substitute(bindings)
	if !ok || targetPath.Symbol == 0 {
		return state.RelOperand{}, false
	}
	return relationGraphKeyAt(resolver, point, targetPath, operand.IsLength)
}

// applyCallParamExposures eager-widens each argument object the callee exposes
// through a wider mutable view. Each exposure declares that the callee aliases the
// argument (or a member sub-path of it), at the wider mutable contract carried by
// Contract, into a slot the callee returns, stores into another argument, or
// retains in a captured sink; a write through that wider view can launder a wider
// value back into the argument object, so a later narrow read of the argument is
// no longer trustworthy. It reuses the single covariant widening routine
// (applyCovariantExposure) by rebasing the exposure's callee-relative Source
// placeholder onto the concrete argument path, so the widen and per-field fact
// invalidation match every other mutable-exposure site. The widen no-ops when the
// argument's current type is not strictly narrower than the contract.
func applyCallParamExposures(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	widen CovariantWiden,
	out state.State,
	paramBindings []pathdom.Path,
	exposures []callpayload.CallParamExposure,
) state.State {
	if widen == nil || len(exposures) == 0 {
		return out
	}
	for _, exposure := range exposures {
		argPath, ok := exposure.Source.Substitute(paramBindings)
		if !ok || argPath.Symbol == 0 {
			continue
		}
		out = applyCovariantExposure(ctx, resolver, widen, out, factflow.NewCovariantExposure(argPath, exposure.Contract, exposure.Kind))
	}
	return out
}

type resolvedCallParamLengthFloor struct {
	Path  pathdom.Path
	Floor int64
}

func resolveCallParamLengthFloors(
	resolver *visibility.Resolver,
	point cfg.Point,
	in state.State,
	paramBindings []pathdom.Path,
	facts []callpayload.CallParamLengthFloor,
) []resolvedCallParamLengthFloor {
	if len(facts) == 0 {
		return nil
	}
	out := make([]resolvedCallParamLengthFloor, 0, len(facts))
	for _, fact := range facts {
		targetPath, ok := fact.Path.Substitute(paramBindings)
		if !ok {
			continue
		}
		floor := fact.Floor
		// Current param length floors are lowered from positive LengthChange
		// effects, so Floor is the minimum delta contributed by the call.
		// Resolve it against the incoming floor before mutation invalidations
		// clear stale descendants, then publish the post-call lower bound.
		if existing, ok := readCallParamLengthFloor(resolver, point, in, targetPath); ok {
			floor += existing
		}
		out = append(out, resolvedCallParamLengthFloor{Path: targetPath, Floor: floor})
	}
	return out
}

func readCallParamLengthFloor(
	resolver *visibility.Resolver,
	point cfg.Point,
	in state.State,
	targetPath pathdom.Path,
) (int64, bool) {
	if resolver == nil || targetPath.Symbol == 0 {
		return 0, false
	}
	pathKey, ok := resolver.StateKeyAt(point, targetPath)
	if !ok {
		return 0, false
	}
	return in.ReadLenFloor(resolver.KeySpace(), pathKey)
}

func applyCallParamLengthFloor(
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	floor int64,
) state.State {
	if resolver == nil || targetPath.Symbol == 0 || floor <= 0 {
		return out
	}
	pathKey, ok := resolver.StateKeyAt(point, targetPath)
	if !ok {
		return out
	}
	return out.WriteLenFloor(resolver.KeySpace(), pathKey, floor)
}

func applyFrozenTableFact(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
) state.State {
	target, ok := resolvePlacementTargetValueAt(reg, resolver, point, out, targetPath, projectPath)
	if !ok {
		return out
	}
	id, ok := product.Get(reg, target.value, identity.Key).ID()
	if !ok {
		return out
	}
	return out.FreezeTable(id)
}

// invalidateMutatedFieldSlot drops the caller's stored value for a field slot a
// callee wrote through. A field-level path invalidation (segments > 0) records
// that the callee assigned the slot to a value of its own, wider parameter field
// type, so the slot's confined caller value (a fresh literal's heap static member
// or path-key refinement) is no longer trustworthy. Descendant invalidation alone
// preserves the slot's own value, which would launder the covariant write-through;
// clearing the slot itself makes the later read fall back to structural
// projection, matching how an opaque parameter argument already reflects the
// mutation. Root-targeted invalidations (segments == 0) keep their container value
// and are handled by the descendant pass above.
func invalidateMutatedFieldSlot(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	targetPath pathdom.Path,
) state.State {
	if len(targetPath.Segments) == 0 {
		return out
	}
	if withHeap, ok := invalidateHeapStaticMemberSubtreeAt(ctx.Registry, out, resolver, ctx.Point, targetPath); ok {
		out = withHeap
	}
	if cleared, ok := invalidatePathSubtreeAt(out, resolver, ctx.Point, targetPath); ok {
		out = cleared
	}
	return out
}

func writePathInvalidationMarker(
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
) state.State {
	if targetKey, ok := factKeyspaceKeyAt(resolver, point, targetPath); ok {
		return out.WriteEffectDelta(effectdelta.Key{
			Target: targetKey,
			Site:   callboundary.PathInvalidationEffectSite(),
			Kind:   effectdelta.Mutation,
		}, effectdelta.Top())
	}
	return out
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
	target, ok := resolvePlacementTargetValueAt(reg, resolver, point, out, targetPath, projectPath)
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

func resolvePlacementTargetValueAt(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	projectPath PathTypeProjector,
) (pathValue, bool) {
	target, ok := resolvePathValueAt(reg, resolver, point, out, targetPath, projectPath)
	if ok {
		if _, hasID := product.Get(reg, target.value, identity.Key).ID(); hasID {
			return target, true
		}
	}
	if len(targetPath.Segments) == 0 {
		return target, ok
	}
	if projected, projectedOK := projectPathDynamicIndexValue(reg, resolver, point, out, targetPath); projectedOK {
		if recovered, recoveredOK := mergePlacementIdentityProjection(reg, target, ok, projected); recoveredOK {
			return recovered, true
		}
	}
	if projected, projectedOK := projectPathHeapStaticMemberValue(reg, resolver, point, out, targetPath); projectedOK {
		if recovered, recoveredOK := mergePlacementIdentityProjection(reg, target, ok, projected); recoveredOK {
			return recovered, true
		}
	}
	if projected, projectedOK := projectPathOriginValue(reg, out, targetPath); projectedOK {
		if recovered, recoveredOK := mergePlacementIdentityProjection(reg, target, ok, projected); recoveredOK {
			return recovered, true
		}
	}
	return target, ok
}

func mergePlacementIdentityProjection(
	reg *axis.Registry,
	target pathValue,
	hasTarget bool,
	projected product.Value,
) (pathValue, bool) {
	if _, hasID := product.Get(reg, projected, identity.Key).ID(); !hasID {
		return pathValue{}, false
	}
	if hasTarget {
		if merged := product.Meet(reg, target.value, projected); !product.Equal(reg, merged, product.Bottom(reg)) {
			target.value = merged
			return target, true
		}
	}
	return pathValue{value: projected}, true
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
	condition callpayload.CallParamCondition,
) state.State {
	arg, ok := site.ArgumentSourceAt(condition.ParamIndex)
	if !ok {
		return out
	}
	if arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr {
		return out
	}
	expressionCondition, ok := facts.ExpressionCondition(arg.ExprRef)
	if !ok {
		return out
	}
	selectedFacts := expressionCondition.FactsForValue(condition.Value)
	for _, refinement := range selectedFacts.Refinements() {
		out = applyValueRefinementAt(ctx.Registry, resolver, projectPath, ctx.Point, out, refinement.TargetPath(), refinement.Value())
	}
	for _, relation := range selectedFacts.PathRelations() {
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
	relation callpayload.CallParamPathRelation,
) state.State {
	switch relation.Kind {
	case callpayload.CallPathRelationEqual:
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
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return true
		}
		sourcePath, ok := facts.ExpressionPath(source.ExprRef)
		if !ok || sourcePath.IsEmpty() {
			return true
		}
		bindings = bindPlaceholderPath(bindings, i+offset, sourcePath)
		return true
	})
	return bindings
}

func callArgumentPlaceholderBindings(facts factflow.Facts, site factflow.CallSiteView) []pathdom.Path {
	var bindings []pathdom.Path
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return true
		}
		sourcePath, ok := facts.ExpressionPath(source.ExprRef)
		if !ok || sourcePath.IsEmpty() {
			return true
		}
		bindings = bindPlaceholderPath(bindings, i, sourcePath)
		return true
	})
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
	path, ok := callOutcomeKeyspaceKeyAt(resolver, point, bindings, proof.Path)
	if !ok {
		return pathevidence.BranchProof{}, false
	}
	switch proof.Kind {
	case pathevidence.BranchProofPathPresence:
		return pathevidence.BranchProof{
			Kind:     pathevidence.BranchProofPathPresence,
			Path:     path,
			Presence: proof.Presence,
		}, true
	case pathevidence.BranchProofPathEqual, pathevidence.BranchProofPathNotEqual, pathevidence.BranchProofIndexInRange:
		other, ok := callOutcomeKeyspaceKeyAt(resolver, point, bindings, proof.Other)
		if !ok {
			return pathevidence.BranchProof{}, false
		}
		return pathevidence.BranchProof{
			Kind:  proof.Kind,
			Path:  path,
			Other: other,
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
		resultStateKey, ok := callOutcomeStateKeyAt(resolver, point, bindings, event.Result)
		if !ok {
			return channelselectfact.Fact{}, false
		}
		fact.Result = resultStateKey
	}
	if !event.Case.IsEmpty() {
		caseStateKey, ok := callOutcomeStateKeyAt(resolver, point, bindings, event.Case)
		if !ok {
			return channelselectfact.Fact{}, false
		}
		fact.Case = caseStateKey
	}
	return fact, true
}

func callOutcomeStateKeyAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	bindings []pathdom.Path,
	path pathdom.Path,
) (pathaddr.StateKey, bool) {
	targetPath, ok := path.Substitute(bindings)
	if !ok {
		return "", false
	}
	return factStateKeyAt(resolver, point, targetPath)
}
