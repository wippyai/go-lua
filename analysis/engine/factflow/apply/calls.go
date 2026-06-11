package apply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factflow/source"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
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

func callResultReader(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	provider CallResultProvider,
) (func(cfg.Point) state.State, func(cfg.Point, state.State) state.State) {
	rawRead := ctx.Read
	if rawRead == nil {
		rawRead = emptyStateRead
	}
	if provider == nil {
		return rawRead, func(_ cfg.Point, base state.State) state.State {
			return base
		}
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
		out := materializeCallResults(callContextAt(ctx, point, read), facts, provider, read, base, base)
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
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
) state.State {
	if provider == nil {
		return out
	}
	call, ok := facts.Call(ctx.Point)
	if !ok {
		return out
	}
	for _, result := range provider(ctx, call, in, read) {
		if result.Index < 0 {
			continue
		}
		out = out.WriteReturnSlot(ctx.Registry, result.Index, result.Value)
	}
	return out
}

func applyReturn(
	ctx transfer.NodeContext,
	sources source.SourceValues,
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
