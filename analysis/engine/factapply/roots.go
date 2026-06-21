package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func applyRootAssignmentFact(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact factflow.RootAssignment,
) (state.State, bool) {
	declared, hasDeclared := fact.DeclaredValue()
	out, targetPath, applied := applyRootAssignment(ctx, resolver, sources, read, in, out, fact.TargetSymbol(), fact.TargetPath(), fact.Source(), declared, hasDeclared, fact.DeclaredValueContracts())
	if applied {
		// The source path-equality proof is suppressed inside the shared helper for a
		// covariant record exposure of this source: the alias is typed wider than its
		// source, so the equality would couple them through reference-equality member
		// congruence and let a write through the wide alias reset the narrow source to
		// Top. The eager source widen (the covariant exposure applied at the end of the
		// node transfer) establishes the sound widened source field type instead. An
		// array exposure keeps the equality for its read-back diagnostics.
		out = addPathEqualityProofFromSource(resolver, facts, ctx.Point, out, targetPath, fact.Source())
		out = applyRootAssignmentNumFloor(ctx, resolver, facts, in, out, targetPath, fact.Source())
		out = applyObjectLiteralEntries(ctx, resolver, facts, sources, read, in, out, targetPath, fact.Source())
	}
	return out, applied
}

func applyRootAssignment(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	target symbol.ID,
	targetPath pathdom.Path,
	source factflow.ValueSource,
	declared product.Value,
	hasDeclared bool,
	declaredContracts bool,
) (state.State, pathdom.Path, bool) {
	root, ok := rootAssignmentTarget(target, targetPath)
	if !ok {
		return out, pathdom.Path{}, false
	}
	var value product.Value
	if hasDeclared && declaredContracts {
		value = declared
	} else if sourceValue, ok := sources.ValueOfSource(ctx.Point, source, in, readWithSamePointCallSource(ctx.Point, source, read, out)); ok {
		value = sourceValue
	} else {
		if !hasDeclared {
			return out, pathdom.Path{}, false
		}
		value = declared
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
	out := targetPath
	if out.Symbol == 0 {
		out.Symbol = target
	}
	return out
}

func writeRootSymbol(ctx transfer.NodeContext, resolver *visibility.Resolver, out state.State, target symbol.ID, targetPath pathdom.Path, value product.Value) state.State {
	if target == 0 {
		return out
	}
	if resolver != nil {
		if invalidated, ok := invalidatePathSubtreeAt(out, resolver, ctx.Point, targetPath); ok {
			out = invalidated
		}
	}
	return out.WriteValue(ctx.Registry, key.SymbolValue(target), value)
}

func applyRootAssignmentNumFloor(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	in state.State,
	out state.State,
	targetPath pathdom.Path,
	source factflow.ValueSource,
) state.State {
	if resolver == nil || targetPath.Symbol == 0 || len(targetPath.Segments) != 0 {
		return out
	}
	targetKey := visibility.RootOrVisibleKeyAt(resolver, ctx.Point, targetPath)
	if targetKey == "" {
		return out
	}
	// Reassigning the root invalidates every difference relation over its old
	// value (and, if it is an array, its old length).
	out = out.ClearDiffConstraintsFor(targetKey)
	if floor, ok := sourcevalue.NumFloorForSource(ctx.Registry, resolver, ctx.Point, facts, in, source); ok {
		return out.WriteNumFloor(resolver.KeySpace(), targetKey, floor)
	}
	return out.ClearNumFloor(resolver.KeySpace(), targetKey)
}
