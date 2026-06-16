package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
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
	outcomeProvider CallOutcomeProvider,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	typeValues *typevalue.Cache,
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
		out := materializeCallOutcome(callContextAt(ctx, point, read), facts, outcomeProvider, resolver, projectPath, typeValues, read, base, base)
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

func materializeCallOutcome(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	outcomeProvider CallOutcomeProvider,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	typeValues *typevalue.Cache,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
) state.State {
	siteView, ok := facts.CallSiteView(ctx.Point)
	if !ok {
		return applyChannelSelectResult(ctx, typeValues, resolver, projectPath, out, facts.ChannelSelects(ctx.Point))
	}
	hasProducer := callproducer.Has(facts, ctx.Point)
	if hasProducer {
		out = clearCallProducerReturnSlots(ctx, siteView, out)
	}
	if outcomeProvider != nil {
		site := siteView.CallSite()
		outcome := outcomeProvider(ctx, site, in, read)
		if hasProducer {
			for _, result := range outcome.Results {
				if result.Index < 0 {
					continue
				}
				out = out.WriteReturnSlot(ctx.Registry, result.Index, result.Value)
			}
		}
		out = applyCallOutcomeFacts(ctx, facts, resolver, projectPath, out, site, outcome)
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
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact factflow.Return,
) state.State {
	for i, source := range fact.Sources() {
		value, ok := sources.ValueOfSource(ctx.Point, source, in, readWithSamePointCallSource(ctx.Point, source, read, out))
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
