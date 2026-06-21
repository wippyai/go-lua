package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/callproducer"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func callResultReader(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	outcomeProvider callpayload.CallOutcomeProvider,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	widen CovariantWiden,
	typeValues *typevalue.Cache,
) (func(cfg.Point) state.State, func(cfg.Point, state.State) state.State) {
	rawRead := ctx.Read
	if rawRead == nil {
		rawRead = emptyStateRead
	}

	var cache callResultPointStateCache
	var active callResultPointStateCache
	var read func(cfg.Point) state.State
	materialize := func(point cfg.Point, base state.State) state.State {
		if out, ok := cache.lookup(point); ok {
			return out
		}
		if activeBase, ok := active.lookup(point); ok {
			return activeBase
		}
		active.store(point, base)
		out := materializeCallOutcome(callContextAt(ctx, point, read), facts, sources, outcomeProvider, resolver, projectPath, widen, typeValues, read, base, base)
		active.remove(point)
		cache.store(point, out)
		return out
	}
	read = func(point cfg.Point) state.State {
		return materialize(point, rawRead(point))
	}
	return read, materialize
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
		out = materializeObjectLiteralHeap(ctx, resolver, facts, sources, read, in, out, source)
		return true
	})
	hasProducer := callproducer.Has(facts, ctx.Point)
	if hasProducer {
		out = clearCallProducerReturnSlots(ctx, siteView, out)
	}
	if outcomeProvider != nil {
		outcome := outcomeProvider(ctx, siteView, in, read)
		if hasProducer {
			for _, result := range outcome.Results {
				if result.Index < 0 {
					continue
				}
				out = out.WriteReturnSlot(ctx.Registry, result.Index, result.Value)
			}
		}
		out = applyCallOutcomeFacts(ctx, facts, resolver, projectPath, widen, out, siteView, outcome)
	}
	out = applyChannelSelectResult(ctx, typeValues, resolver, projectPath, out, facts.ChannelSelects(ctx.Point))
	if hasProducer {
		facts.ForEachCallResultValue(ctx.Point, func(result factflow.CallResultValue) bool {
			out = constrainReturnSlot(ctx, out, result)
			return true
		})
	}
	return out
}

func clearCallProducerReturnSlots(ctx transfer.NodeContext, site factflow.CallSiteView, out state.State) state.State {
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		if target.ResultIndex() < 0 {
			return true
		}
		out = out.WriteReturnSlot(ctx.Registry, target.ResultIndex(), product.Bottom(ctx.Registry))
		return true
	})
	return out
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
	for i, source := range fact.Sources() {
		value, ok := returnSourceValue(ctx, facts, sources, read, in, out, source, resolver, projectPath, typeValues)
		if !ok {
			continue
		}
		out = out.WriteReturnSlot(ctx.Registry, i, value)
		out = materializeObjectLiteralHeap(ctx, resolver, facts, sources, read, in, out, source)
	}
	return out
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
		if sourcePath, ok := facts.ExpressionPath(source.ExprRef); ok {
			if pathValue, ok := resolvePathValueAtCached(typeValues, ctx.Registry, resolver, ctx.Point, out, sourcePath, projectPath); ok {
				return pathValue.value, true
			}
		}
	}
	return sources.ValueOfSource(ctx.Point, source, in, readWithSamePointCallSource(ctx.Point, source, read, out))
}

func emptyStateRead(cfg.Point) state.State {
	return state.State{}
}
