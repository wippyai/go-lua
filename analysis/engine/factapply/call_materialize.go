package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/callproducer"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourceprojection"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type callResultMaterializer struct {
	owner   *lazyCallResultReader
	rawRead func(cfg.Point) state.State
	read    func(cfg.Point) state.State

	cache  callResultPointStateCache
	active callResultPointStateCache
}

func newCallResultMaterializer(owner *lazyCallResultReader) *callResultMaterializer {
	rawRead := owner.ctx.Read
	if rawRead == nil {
		rawRead = emptyStateRead
	}
	m := &callResultMaterializer{
		owner:   owner,
		rawRead: rawRead,
	}
	m.read = m.readPoint
	return m
}

func (m *callResultMaterializer) readPoint(point cfg.Point) state.State {
	return m.materialize(point, m.rawRead(point))
}

func (m *callResultMaterializer) materialize(point cfg.Point, base state.State) state.State {
	if out, ok := m.cache.lookup(point); ok {
		return out
	}
	if activeBase, ok := m.active.lookup(point); ok {
		return activeBase
	}
	m.active.store(point, base)
	owner := m.owner
	out := materializeCallOutcome(callContextAt(owner.ctx, point, m.read), owner.facts, owner.sources, owner.outcomeProvider, owner.resolver, owner.projectPath, owner.widen, owner.typeValues, m.read, base, base)
	m.active.remove(point)
	m.cache.store(point, out)
	return out
}

type lazyCallResultReader struct {
	initialized bool

	ctx             transfer.NodeContext
	facts           factflow.Facts
	sources         sourcevalue.SourceValues
	outcomeProvider callpayload.CallOutcomeProvider
	resolver        *visibility.Resolver
	projectPath     PathTypeProjector
	widen           CovariantWiden
	typeValues      *typevalue.Cache

	read         func(cfg.Point) state.State
	materialize  func(cfg.Point, state.State) state.State
	lazyRead     func(cfg.Point) state.State
	materializer *callResultMaterializer
}

func (r *lazyCallResultReader) ensure() {
	if r.initialized {
		return
	}
	r.materializer = newCallResultMaterializer(r)
	r.read = r.materializer.read
	r.materialize = r.materializer.materialize
	r.initialized = true
}

// ReadLazy returns a read function that initializes call-result materialization
// only if the caller actually reads through it. Most non-call value sources never
// read another point, so they should not allocate the materialization cache just
// to carry a same-point read handle through transfer helpers.
func (r *lazyCallResultReader) ReadLazy() func(cfg.Point) state.State {
	if r.lazyRead == nil {
		r.lazyRead = func(point cfg.Point) state.State {
			r.ensure()
			return r.read(point)
		}
	}
	return r.lazyRead
}

func (r *lazyCallResultReader) Materialize(point cfg.Point, base state.State) state.State {
	r.ensure()
	return r.materialize(point, base)
}

func nodeHasCallMaterializationFacts(facts factflow.Facts, point cfg.Point) bool {
	if _, ok := facts.CallSiteView(point); ok {
		return true
	}
	return facts.HasChannelSelects(point)
}

type callResultPointStateCache struct {
	point    cfg.Point
	state    state.State
	valid    bool
	overflow map[cfg.Point]state.State
}

func (c *callResultPointStateCache) lookup(point cfg.Point) (state.State, bool) {
	if c.overflow != nil {
		out, ok := c.overflow[point]
		return out, ok
	}
	if c.valid && c.point == point {
		return c.state, true
	}
	return state.State{}, false
}

func (c *callResultPointStateCache) store(point cfg.Point, out state.State) {
	if c.overflow != nil {
		c.overflow[point] = out
		return
	}
	if !c.valid || c.point == point {
		c.point = point
		c.state = out
		c.valid = true
		return
	}
	c.overflow = make(map[cfg.Point]state.State, 2)
	c.overflow[c.point] = c.state
	c.point = 0
	c.state = state.State{}
	c.valid = false
	c.overflow[point] = out
}

func (c *callResultPointStateCache) remove(point cfg.Point) {
	if c.overflow != nil {
		delete(c.overflow, point)
		return
	}
	if c.valid && c.point == point {
		c.point = 0
		c.state = state.State{}
		c.valid = false
	}
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

func materializeCallOutcome(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	outcomeProvider callpayload.CallOutcomeProvider,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	widen CovariantWiden,
	typeValues *typevalue.Cache,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
) state.State {
	siteView, ok := facts.CallSiteView(ctx.Point)
	if !ok {
		return applyChannelSelectResult(ctx, typeValues, resolver, projectPath, out, facts.ChannelSelects(ctx.Point))
	}
	siteView.ForEachArgumentSource(func(_ int, source factflow.ValueSource) bool {
		out = materializeObjectLiteralHeap(ctx, resolver, facts, sources, read, in, out, source, typeValues)
		return true
	})
	hasProducer := callproducer.Has(facts, ctx.Point)
	var outcome callpayload.CallOutcome
	hasOutcome := false
	if outcomeProvider != nil {
		outcome = outcomeProvider(ctx, siteView, in, read)
		hasOutcome = true
	}
	if hasProducer {
		out = applyCallProducerReturnSlots(ctx, siteView, out, outcome, hasOutcome)
	}
	if hasOutcome {
		out = applyCallOutcomeFacts(ctx, facts, resolver, projectPath, widen, typeValues, out, siteView, outcome)
	}
	out = applyChannelSelectResult(ctx, typeValues, resolver, projectPath, out, facts.ChannelSelects(ctx.Point))
	edit := out.EditValues(ctx.Registry)
	appliedFixedResult := false
	facts.ForEachCallResultValue(ctx.Point, func(result factflow.CallResultValue) bool {
		constrainReturnSlotEdit(ctx, &edit, result)
		appliedFixedResult = true
		return true
	})
	if appliedFixedResult {
		out = edit.Done()
	}
	return out
}

func applyCallProducerReturnSlots(ctx transfer.NodeContext, site factflow.CallSiteView, out state.State, outcome callpayload.CallOutcome, hasOutcome bool) state.State {
	edit := out.EditValues(ctx.Registry)
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		if target.ResultIndex() < 0 {
			return true
		}
		edit.WriteReturnSlot(target.ResultIndex(), product.Bottom(ctx.Registry))
		return true
	})
	if hasOutcome {
		for _, result := range outcome.Results {
			if result.Index < 0 {
				continue
			}
			edit.WriteReturnSlot(result.Index, result.Value)
		}
	}
	return edit.Done()
}

func constrainReturnSlotEdit(ctx transfer.NodeContext, edit *state.ValueEdit, fact factflow.CallResultValue) {
	if fact.Index() < 0 {
		return
	}
	value := fact.Value()
	current := edit.Read(key.ReturnSlot(fact.Index()))
	if product.Equal(ctx.Registry, current, product.Bottom(ctx.Registry)) {
		edit.WriteReturnSlot(fact.Index(), value)
		return
	}
	if returnSlotLacksReadableType(ctx.Registry, current) && returnSlotHasReadableType(ctx.Registry, value) {
		edit.WriteReturnSlot(fact.Index(), value)
		return
	}
	if returnSlotHasTrustedEvidence(ctx.Registry, current) && returnSlotHasUntrustedTopEvidence(ctx.Registry, value) {
		return
	}
	edit.WriteReturnSlot(fact.Index(), product.Meet(ctx.Registry, current, value))
}

func returnSlotHasReadableType(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	return ok && t != nil && !typ.IsAny(t) && !typ.IsUnknown(t) && !typ.IsNever(t)
}

func returnSlotLacksReadableType(reg *axis.Registry, value product.Value) bool {
	return !returnSlotHasReadableType(reg, value)
}

func returnSlotHasTrustedEvidence(reg *axis.Registry, value product.Value) bool {
	ev := product.Get(reg, value, evidence.Key)
	return !ev.IsExplicitTop() && !ev.IsGradualTop()
}

func returnSlotHasUntrustedTopEvidence(reg *axis.Registry, value product.Value) bool {
	ev := product.Get(reg, value, evidence.Key)
	return ev.IsExplicitTop() || ev.IsGradualTop()
}

func applyReturn(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact factflow.Return,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	typeValues *typevalue.Cache,
) state.State {
	var edit state.ValueEdit
	editing := false
	var returnSlots []int
	var declaredReturnSources map[int]struct{}
	for i, source := range fact.Sources() {
		targetIndex := source.TargetIndex
		if targetIndex < 0 {
			targetIndex = i
		}
		hasDeclaredContract := returnSourceHasDeclaredContract(facts, source)
		value, ok := returnSourceValue(ctx, facts, sources, read, in, out, source, resolver, projectPath, typeValues)
		if !ok {
			value = product.Top()
		} else {
			out, _ = materializeObjectLiteralHeapCachedWithKnownSourceValue(ctx, resolver, facts, sources, read, in, out, source, value, true, typeValues)
			out = markReachableHeapObjectValuePlacement(ctx.Registry, out, value, placement.OwnedHeap, map[identity.ID]struct{}{})
			if !hasDeclaredContract && returnSlotAllowsHeapContainerProjection(ctx.Registry, typeValues, value) {
				if projected, projectedOK := sourceprojection.HeapObjectContainerType(ctx.Registry, typeValues, out, value); projectedOK {
					value = typevalue.WithWitness(ctx.Registry, value, projected)
				}
			} else {
				if declaredReturnSources == nil {
					declaredReturnSources = make(map[int]struct{}, 1)
				}
				declaredReturnSources[targetIndex] = struct{}{}
			}
		}
		if !editing {
			edit = out.EditValues(ctx.Registry)
			editing = true
		}
		edit.WriteReturnSlot(targetIndex, value)
		returnSlots = append(returnSlots, targetIndex)
	}
	if !editing {
		return out
	}
	out = edit.DoneOn(out)
	for _, index := range returnSlots {
		if _, declared := declaredReturnSources[index]; declared {
			continue
		}
		value := out.ReadReturnSlot(ctx.Registry, index)
		if !returnSlotAllowsHeapContainerProjection(ctx.Registry, typeValues, value) {
			continue
		}
		projected, ok := sourceprojection.HeapObjectContainerType(ctx.Registry, typeValues, out, value)
		if !ok {
			continue
		}
		out = out.WriteReturnSlot(ctx.Registry, index, typevalue.WithWitness(ctx.Registry, value, projected))
	}
	return out
}

func returnSourceHasDeclaredContract(facts factflow.Facts, source factflow.ValueSource) bool {
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return false
	}
	refinement, ok := facts.ExpressionRefinement(source.ExprRef)
	return ok && refinement.Mode() == factflow.ExpressionRefinementDeclaredContract
}

func returnSlotAllowsHeapContainerProjection(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) bool {
	if reg == nil || typeValues == nil {
		return true
	}
	t, ok := typeValues.TypeOf(reg, value)
	if !ok || t == nil {
		return true
	}
	return !typ.ContainsAny(t) && !inspect.ContainsUnknown(t)
}

func returnSourceValue(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	source factflow.ValueSource,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	typeValues *typevalue.Cache,
) (product.Value, bool) {
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		if _, ok := facts.ExpressionRefinement(source.ExprRef); ok {
			if value, ok := sources.ValueOfSource(ctx.Point, source, out, readWithCurrentPointState(ctx.Point, read, out)); ok {
				return value, true
			}
		}
		if sourcePath, ok := facts.ExpressionPathRef(source.ExprRef); ok {
			if pathValue, ok := resolvePathValueAtCached(typeValues, ctx.Registry, resolver, ctx.Point, out, sourcePath, projectPath); ok {
				if cached, cachedOK := facts.ExpressionValue(source.ExprRef); cachedOK {
					if preserved, preserveOK := sourcevalue.PreservePathBackedGradualContract(ctx.Registry, typeValues, cached, pathValue.value); preserveOK {
						return preserved, true
					}
				}
				return pathValue.value, true
			}
		}
	}
	return sources.ValueOfSource(ctx.Point, source, out, readWithCurrentPointState(ctx.Point, read, out))
}

func emptyStateRead(cfg.Point) state.State {
	return state.State{}
}
