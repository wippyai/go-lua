package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func applyPathAssignment(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact factflow.PathAssignment,
) (state.State, bool) {
	if resolver == nil {
		return out, false
	}
	targetPath := fact.TargetPath()
	if len(targetPath.Segments) == 0 {
		return out, false
	}
	source := fact.Source()
	value, ok := sources.ValueOfSource(ctx.Point, source, in, readWithSamePointCallSource(ctx.Point, source, read, out))
	if !ok {
		return out, false
	}
	invalidated := out
	if withOrigins, ok := invalidateRootOriginsForPathMutationAt(ctx.Registry, invalidated, resolver, ctx.Point, targetPath, false); ok {
		invalidated = withOrigins
	}
	if withHeap, ok := invalidateHeapStaticMemberSubtreeAt(ctx.Registry, invalidated, resolver, ctx.Point, targetPath); ok {
		invalidated = withHeap
	}
	invalidated, ok = invalidatePathSubtreeAt(invalidated, resolver, ctx.Point, targetPath)
	if !ok {
		return out, false
	}
	written, ok := writePathAt(ctx.Registry, invalidated, resolver, ctx.Point, targetPath, value)
	if !ok {
		return out, false
	}
	return written, true
}

func applyPathDescendantInvalidation(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	fact factflow.PathDescendantInvalidation,
	clearStructuralWitness bool,
) state.State {
	if resolver == nil {
		return out
	}
	containerPath := fact.ContainerPath()
	if containerPath.IsEmpty() {
		return out
	}
	invalidated := out
	if withOrigins, ok := invalidateRootOriginsForPathMutationAt(ctx.Registry, invalidated, resolver, ctx.Point, containerPath, true); ok {
		invalidated = withOrigins
	}
	if clearStructuralWitness {
		if withWitness, ok := invalidateRootStructuralWitnessForPathMutationAt(ctx.Registry, invalidated, resolver, ctx.Point, containerPath); ok {
			invalidated = withWitness
		}
	}
	if withHeap, ok := invalidateHeapStaticMemberDescendantsAt(ctx.Registry, invalidated, resolver, ctx.Point, containerPath); ok {
		invalidated = withHeap
	}
	invalidated, ok := invalidatePathDescendantsAt(invalidated, resolver, ctx.Point, containerPath)
	if !ok {
		return out
	}
	return invalidated
}

func copyPath(p pathdom.Path) pathdom.Path {
	if len(p.Segments) == 0 {
		return p
	}
	out := p
	out.Segments = append(p.Segments[:0:0], p.Segments...)
	return out
}
