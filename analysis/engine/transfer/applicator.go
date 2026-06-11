package transfer

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// FactsNodeTransferConfig configures the generic fact applicator.
type FactsNodeTransferConfig struct {
	Facts      Facts
	Sources    SourceValues
	Visibility *visibility.Resolver
}

// NewFactsNodeTransfer returns a generic node transfer that applies point-local
// transfer facts. It intentionally handles only root assignment, member/path
// assignment, and return-slot facts; richer Lua lowering, calls, branches, and
// diagnostics stay outside this package.
func NewFactsNodeTransfer(config FactsNodeTransferConfig) NodeTransfer {
	return func(ctx NodeContext, in state.State) state.State {
		facts := config.Facts
		sources := config.Sources
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
		if fact, ok := facts.PathAssignment(ctx.Point); ok {
			out = applyPathAssignment(ctx, config.Visibility, sources, read, in, out, fact)
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

func applyPathAssignment(
	ctx NodeContext,
	resolver *visibility.Resolver,
	sources SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact PathAssignment,
) state.State {
	if resolver == nil {
		return out
	}
	targetPath := fact.TargetPath()
	if len(targetPath.Segments) == 0 {
		return out
	}
	value, ok := sources.ValueOfSource(ctx.Point, fact.Source(), in, read)
	if !ok {
		return out
	}
	invalidated, ok := out.InvalidatePathSubtreeAt(resolver, ctx.Point, targetPath)
	if !ok {
		return out
	}
	written, ok := invalidated.WritePathAt(ctx.Registry, resolver, ctx.Point, targetPath, value)
	if !ok {
		return out
	}
	return written
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
