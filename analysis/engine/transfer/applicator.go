package transfer

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// NewFactsNodeTransfer returns a generic node transfer that applies point-local
// transfer facts. It intentionally handles only root assignment and return-slot
// facts; richer Lua lowering, member writes, calls, branches, and diagnostics
// stay outside this package.
func NewFactsNodeTransfer(facts Facts, sources SourceValues) NodeTransfer {
	return func(ctx NodeContext, in state.State) state.State {
		if sources == nil {
			return in
		}
		read := ctx.Read
		if read == nil {
			read = emptyStateRead
		}

		out := in
		if fact, ok := facts.LocalAssignment(ctx.Point); ok {
			out = applyRootAssignment(ctx, sources, read, in, out, fact.TargetSymbol(), fact.TargetPath(), fact.Source())
		}
		if fact, ok := facts.OrdinaryAssignment(ctx.Point); ok {
			out = applyRootAssignment(ctx, sources, read, in, out, fact.TargetSymbol(), fact.TargetPath(), fact.Source())
		}
		if fact, ok := facts.Return(ctx.Point); ok {
			out = applyReturn(ctx, sources, read, in, out, fact)
		}
		return out
	}
}

func applyRootAssignment(
	ctx NodeContext,
	sources SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	target symbol.ID,
	targetPath pathdom.Path,
	source ValueSource,
) state.State {
	target, ok := rootAssignmentTarget(target, targetPath)
	if !ok {
		return out
	}
	value, ok := sources.ValueOfSource(ctx.Point, source, in, read)
	if !ok {
		return out
	}
	return writeRootSymbol(ctx, out, target, value)
}

func rootAssignmentTarget(target symbol.ID, targetPath pathdom.Path) (symbol.ID, bool) {
	if len(targetPath.Segments) != 0 {
		return 0, false
	}
	if target != 0 {
		return target, true
	}
	if targetPath.Symbol != 0 {
		return targetPath.Symbol, true
	}
	return 0, false
}

func writeRootSymbol(ctx NodeContext, out state.State, target symbol.ID, value product.Value) state.State {
	if target == 0 {
		return out
	}
	return out.WriteValue(ctx.Registry, key.SymbolValue(target), value)
}

func applyReturn(
	ctx NodeContext,
	sources SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact Return,
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
