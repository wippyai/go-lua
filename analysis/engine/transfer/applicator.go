package transfer

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// FactsNodeTransferConfig configures the generic fact applicator.
type FactsNodeTransferConfig struct {
	Facts       Facts
	Sources     SourceValues
	CallResults CallResultProvider
	Visibility  *visibility.Resolver
}

// FactsEdgeTransferConfig configures the generic edge fact applicator.
type FactsEdgeTransferConfig struct {
	Facts      Facts
	Visibility *visibility.Resolver
}

// NewFactsNodeTransfer returns a generic node transfer that applies point-local
// transfer facts. It intentionally handles only root assignment, member/path
// assignment, call return-slot production, and return-slot facts; branch-edge
// refinements are handled by NewFactsEdgeTransfer.
func NewFactsNodeTransfer(config FactsNodeTransferConfig) NodeTransfer {
	return func(ctx NodeContext, in state.State) state.State {
		facts := config.Facts
		sources := config.Sources
		callResults := config.CallResults
		read, materialize := callResultReader(ctx, facts, callResults)

		out := materialize(ctx.Point, in)
		if sources == nil {
			return out
		}
		sources = withValueOverlaySourceValues(ctx.Registry, sources, facts.valueOverlays)
		if fact, ok := facts.LocalAssignment(ctx.Point); ok {
			var targetPath pathdom.Path
			var applied bool
			out, targetPath, applied = applyRootAssignment(ctx, config.Visibility, sources, read, in, out, fact.TargetSymbol(), fact.TargetPath(), fact.Source())
			if applied {
				out = applyObjectLiteralEntries(ctx, config.Visibility, facts, sources, read, in, out, targetPath, fact.Source())
			}
		}
		if fact, ok := facts.OrdinaryAssignment(ctx.Point); ok {
			var targetPath pathdom.Path
			var applied bool
			out, targetPath, applied = applyRootAssignment(ctx, config.Visibility, sources, read, in, out, fact.TargetSymbol(), fact.TargetPath(), fact.Source())
			if applied {
				out = applyObjectLiteralEntries(ctx, config.Visibility, facts, sources, read, in, out, targetPath, fact.Source())
			}
		}
		if fact, ok := facts.PathAssignment(ctx.Point); ok {
			var applied bool
			out, applied = applyPathAssignment(ctx, config.Visibility, sources, read, in, out, fact)
			if applied {
				out = applyObjectLiteralEntries(ctx, config.Visibility, facts, sources, read, in, out, fact.TargetPath(), fact.Source())
			}
		}
		if fact, ok := facts.Return(ctx.Point); ok {
			out = applyReturn(ctx, sources, read, in, out, fact)
		}
		return out
	}
}

// NewFactsEdgeTransfer returns a generic edge transfer that applies
// branch refinements for the selected branch edge.
func NewFactsEdgeTransfer(config FactsEdgeTransferConfig) EdgeTransfer {
	return func(ctx EdgeContext, out state.State) state.State {
		if !ctx.HasCond {
			return out
		}
		fact, ok := config.Facts.BranchRefinement(ctx.Edge.From)
		if !ok {
			return out
		}
		refinement, ok := fact.ValueForEdge(ctx.Edge.Cond)
		if !ok {
			return out
		}
		return applyBranchRefinement(ctx, config.Visibility, out, fact.TargetPath(), refinement)
	}
}

func callResultReader(
	ctx NodeContext,
	facts Facts,
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

func callContextAt(ctx NodeContext, point cfg.Point, read func(cfg.Point) state.State) NodeContext {
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
	ctx NodeContext,
	facts Facts,
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

func applyRootAssignment(
	ctx NodeContext,
	resolver *visibility.Resolver,
	sources SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	target symbol.ID,
	targetPath pathdom.Path,
	source ValueSource,
) (state.State, pathdom.Path, bool) {
	root, ok := rootAssignmentTarget(target, targetPath)
	if !ok {
		return out, pathdom.Path{}, false
	}
	value, ok := sources.ValueOfSource(ctx.Point, source, in, read)
	if !ok {
		return out, pathdom.Path{}, false
	}
	targetPath = rootAssignmentPath(root, targetPath)
	return writeRootSymbol(ctx, resolver, out, root, targetPath, value), targetPath, true
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

func rootAssignmentPath(target symbol.ID, targetPath pathdom.Path) pathdom.Path {
	out := copyPath(targetPath)
	if out.Symbol == 0 {
		out.Symbol = target
	}
	return out
}

func writeRootSymbol(ctx NodeContext, resolver *visibility.Resolver, out state.State, target symbol.ID, targetPath pathdom.Path, value product.Value) state.State {
	if target == 0 {
		return out
	}
	if resolver != nil {
		if invalidated, ok := out.InvalidatePathSubtreeAt(resolver, ctx.Point, targetPath); ok {
			out = invalidated
		}
	}
	return out.WriteValue(ctx.Registry, key.SymbolValue(target), value)
}

func applyBranchRefinement(
	ctx EdgeContext,
	resolver *visibility.Resolver,
	out state.State,
	targetPath pathdom.Path,
	refinement ValueRefinement,
) state.State {
	if targetPath.Symbol == 0 {
		return out
	}
	if len(targetPath.Segments) == 0 {
		return out.UpdateValue(ctx.Registry, key.SymbolValue(targetPath.Symbol), func(value product.Value) product.Value {
			return refineProductValue(ctx.Registry, value, refinement)
		})
	}
	if resolver == nil {
		return out
	}
	updated, ok := out.UpdatePathAt(ctx.Registry, resolver, ctx.Edge.From, targetPath, func(value product.Value) product.Value {
		return refineProductValue(ctx.Registry, value, refinement)
	})
	if !ok {
		return out
	}
	return updated
}

func refineProductValue(reg *axis.Registry, value product.Value, refinement ValueRefinement) product.Value {
	constraint, ok := refinement.Constraint()
	if !ok {
		return value
	}
	return product.Meet(reg, value, constraint)
}

func applyPathAssignment(
	ctx NodeContext,
	resolver *visibility.Resolver,
	sources SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact PathAssignment,
) (state.State, bool) {
	if resolver == nil {
		return out, false
	}
	targetPath := fact.TargetPath()
	if len(targetPath.Segments) == 0 {
		return out, false
	}
	value, ok := sources.ValueOfSource(ctx.Point, fact.Source(), in, read)
	if !ok {
		return out, false
	}
	invalidated, ok := out.InvalidatePathSubtreeAt(resolver, ctx.Point, targetPath)
	if !ok {
		return out, false
	}
	written, ok := invalidated.WritePathAt(ctx.Registry, resolver, ctx.Point, targetPath, value)
	if !ok {
		return out, false
	}
	return written, true
}

func applyObjectLiteralEntries(
	ctx NodeContext,
	resolver *visibility.Resolver,
	facts Facts,
	sources SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	targetPath pathdom.Path,
	source ValueSource,
) state.State {
	if resolver == nil || !source.HasExpr {
		return out
	}
	literal, ok := facts.ObjectLiteral(source.ExprRef)
	if !ok {
		return out
	}
	for _, entry := range literal.Entries() {
		entryPath, ok := objectEntryTargetPath(targetPath, entry.Suffix())
		if !ok {
			continue
		}
		if resolver.KeyAt(ctx.Point, entryPath) == "" {
			continue
		}
		value, ok := sources.ValueOfSource(ctx.Point, entry.Source(), in, read)
		if !ok {
			continue
		}
		invalidated, ok := out.InvalidatePathSubtreeAt(resolver, ctx.Point, entryPath)
		if !ok {
			continue
		}
		written, ok := invalidated.WritePathAt(ctx.Registry, resolver, ctx.Point, entryPath, value)
		if !ok {
			continue
		}
		out = written
	}
	return out
}

func objectEntryTargetPath(root pathdom.Path, suffix pathdom.Path) (pathdom.Path, bool) {
	if root.IsEmpty() || len(suffix.Segments) == 0 {
		return pathdom.Path{}, false
	}
	out := copyPath(root)
	out.Segments = append(out.Segments, suffix.Segments...)
	return out, true
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
