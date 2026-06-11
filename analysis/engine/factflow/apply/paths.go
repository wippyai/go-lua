package apply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factflow/source"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func applyPathAssignment(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	sources source.SourceValues,
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
	value, ok := sources.ValueOfSource(ctx.Point, fact.Source(), in, read)
	if !ok {
		return out, false
	}
	invalidated, ok := invalidatePathSubtreeAt(out, resolver, ctx.Point, targetPath)
	if !ok {
		return out, false
	}
	written, ok := writePathAt(ctx.Registry, invalidated, resolver, ctx.Point, targetPath, value)
	if !ok {
		return out, false
	}
	return written, true
}

func copyPath(p pathdom.Path) pathdom.Path {
	if len(p.Segments) == 0 {
		return p
	}
	out := p
	out.Segments = append(p.Segments[:0:0], p.Segments...)
	return out
}
