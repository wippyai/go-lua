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

func applyObjectLiteralEntries(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	targetPath pathdom.Path,
	valueSource factflow.ValueSource,
) state.State {
	if resolver == nil || !valueSource.HasExpr {
		return out
	}
	literal, ok := facts.ObjectLiteral(valueSource.ExprRef)
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
		invalidated, ok := invalidatePathSubtreeAt(out, resolver, ctx.Point, entryPath)
		if !ok {
			continue
		}
		written, ok := writePathAt(ctx.Registry, invalidated, resolver, ctx.Point, entryPath, value)
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
