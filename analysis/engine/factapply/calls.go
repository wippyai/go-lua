package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// CallResult is one indexed abstract result produced by a call.
type CallResult struct {
	Index int
	Value product.Value
}

// CallResultProvider resolves generic call-producer facts into indexed return
// slots. Call result targets remain metadata for downstream facts; providers
// produce only ReturnSlot(index) values.
type CallResultProvider func(ctx transfer.NodeContext, call factflow.CallProducer, in state.State, read func(cfg.Point) state.State) []CallResult

// CallOutcomeProvider resolves rich call-site evidence into one generic call
// outcome payload.
type CallOutcomeProvider func(ctx transfer.NodeContext, site factflow.CallSite, in state.State, read func(cfg.Point) state.State) CallOutcome

func callResultReader(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	provider CallResultProvider,
	outcomeProvider CallOutcomeProvider,
	resolver *visibility.Resolver,
) (func(cfg.Point) state.State, func(cfg.Point, state.State) state.State) {
	rawRead := ctx.Read
	if rawRead == nil {
		rawRead = emptyStateRead
	}

	cache := make(map[cfg.Point]state.State)
	active := make(map[cfg.Point]bool)
	activeBase := make(map[cfg.Point]state.State)
	var read func(cfg.Point) state.State
	materialize := func(point cfg.Point, base state.State) state.State {
		if out, ok := cache[point]; ok {
			return out
		}
		if active[point] {
			return activeBase[point]
		}
		active[point] = true
		activeBase[point] = base
		out := materializeCallResults(callContextAt(ctx, point, read), facts, provider, outcomeProvider, resolver, read, base, base)
		delete(active, point)
		delete(activeBase, point)
		cache[point] = out
		return out
	}
	read = func(point cfg.Point) state.State {
		return materialize(point, rawRead(point))
	}
	return read, materialize
}

func callContextAt(ctx transfer.NodeContext, point cfg.Point, read func(cfg.Point) state.State) transfer.NodeContext {
	ctx.Point = point
	ctx.Read = read
	if ctx.Graph != nil {
		ctx.Node = ctx.Graph.Node(point)
	} else {
		ctx.Node = nil
	}
	return ctx
}

func materializeCallResults(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	provider CallResultProvider,
	outcomeProvider CallOutcomeProvider,
	resolver *visibility.Resolver,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
) state.State {
	site, ok := facts.CallSite(ctx.Point)
	if !ok {
		return out
	}
	call, hasProducer := facts.Call(ctx.Point)
	written := make(map[int]struct{})
	if outcomeProvider != nil {
		outcome := outcomeProvider(ctx, site, in, read)
		if hasProducer {
			for _, result := range outcome.Results {
				if result.Index < 0 {
					continue
				}
				out = out.WriteReturnSlot(ctx.Registry, result.Index, result.Value)
				written[result.Index] = struct{}{}
			}
		}
		out = applyCallOutcomeFacts(ctx, facts, resolver, out, site, outcome)
	}
	if provider != nil && hasProducer {
		for _, result := range provider(ctx, call, in, read) {
			if result.Index < 0 {
				continue
			}
			if _, ok := written[result.Index]; ok {
				continue
			}
			out = out.WriteReturnSlot(ctx.Registry, result.Index, result.Value)
		}
	}
	if hasProducer {
		for _, result := range facts.CallResultValues(ctx.Point) {
			out = constrainReturnSlot(ctx, out, result)
		}
	}
	return out
}

func applyCallOutcomeFacts(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	out state.State,
	site factflow.CallSite,
	outcome CallOutcome,
) state.State {
	bindings := callPlaceholderBindings(facts, site)
	for _, fact := range outcome.PathRefinements {
		targetPath, ok := fact.Path.Substitute(bindings)
		if !ok {
			continue
		}
		out = applyValueRefinementAt(ctx.Registry, resolver, ctx.Point, out, targetPath, factflow.NewValueConstraint(fact.Value))
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
		kind, ok := callEffectDeltaKind(delta.Kind)
		if !ok {
			continue
		}
		out = out.WriteEffectDelta(ctx.Registry, state.EffectDeltaKey{
			Target: targetKey,
			Site:   state.EffectSite(delta.Site),
			Kind:   kind,
		}, state.EffectDelta{
			Before: delta.Before,
			After:  delta.After,
			Change: callEffectDeltaChange(delta.Change),
		})
	}
	return out
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

func callEffectDeltaKind(kind CallEffectDeltaKind) (state.EffectDeltaKind, bool) {
	switch kind {
	case CallEffectDeltaMutation:
		return state.EffectDeltaMutation, true
	case CallEffectDeltaEscape:
		return state.EffectDeltaEscape, true
	case CallEffectDeltaCall:
		return state.EffectDeltaCall, true
	default:
		return 0, false
	}
}

func callEffectDeltaChange(change CallEffectDeltaChange) state.EffectDeltaChange {
	switch change {
	case CallEffectDeltaChangeNone:
		return state.EffectDeltaChangeNone
	case CallEffectDeltaChangeChanged:
		return state.EffectDeltaChangeChanged
	case CallEffectDeltaChangeUnknown:
		return state.EffectDeltaChangeUnknown
	default:
		return state.EffectDeltaChangeBottom
	}
}

func constrainReturnSlot(ctx transfer.NodeContext, out state.State, fact factflow.CallResultValue) state.State {
	if fact.Index() < 0 {
		return out
	}
	value := fact.Value()
	current := out.ReadReturnSlot(ctx.Registry, fact.Index())
	if product.Equal(ctx.Registry, current, product.Bottom(ctx.Registry)) {
		return out.WriteReturnSlot(ctx.Registry, fact.Index(), value)
	}
	return out.WriteReturnSlot(ctx.Registry, fact.Index(), product.Meet(ctx.Registry, current, value))
}

func applyReturn(
	ctx transfer.NodeContext,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact factflow.Return,
) state.State {
	for i, source := range fact.Sources() {
		value, ok := sources.ValueOfSource(ctx.Point, source, in, read)
		if !ok {
			continue
		}
		out = out.WriteReturnSlot(ctx.Registry, i, value)
	}
	return out
}

func emptyStateRead(cfg.Point) state.State {
	return state.State{}
}
