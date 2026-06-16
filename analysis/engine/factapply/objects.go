package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
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
	if !valueSource.HasExpr {
		return out
	}
	literal, ok := facts.ObjectLiteral(valueSource.ExprRef)
	if !ok {
		return out
	}
	out = materializeObjectLiteralHeap(ctx, facts, sources, read, in, out, valueSource)
	if resolver == nil {
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
		source := entry.Source()
		value, ok := sources.ValueOfSource(ctx.Point, source, in, readWithSamePointCallSource(ctx.Point, source, read, out))
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

func materializeObjectLiteralHeap(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	source factflow.ValueSource,
) state.State {
	if sources == nil {
		return out
	}
	if !source.HasExpr {
		return out
	}
	literal, ok := facts.ObjectLiteral(source.ExprRef)
	if !ok {
		return out
	}
	return writeObjectLiteralHeap(ctx, facts, sources, read, in, out, source, literal, nil)
}

func writeObjectLiteralHeap(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	source factflow.ValueSource,
	literal factflow.ObjectLiteral,
	active map[factflow.ExprRef]bool,
) state.State {
	if !source.HasExpr {
		return out
	}
	if active != nil && active[source.ExprRef] {
		return out
	}
	rootValue, ok := sources.ValueOfSource(ctx.Point, source, in, readWithSamePointCallSource(ctx.Point, source, read, out))
	if !ok {
		return out
	}
	id, ok := product.Get(ctx.Registry, rootValue, identity.Key).ID()
	if !ok {
		return out
	}
	if active == nil {
		active = make(map[factflow.ExprRef]bool, 1)
	}
	active[source.ExprRef] = true
	staticMembers := make(map[pathdom.PathKey]product.Value, len(literal.Entries()))
	for _, entry := range literal.Entries() {
		key, ok := heapidentity.StaticMemberSuffixKey(entry.Suffix().Segments)
		if !ok {
			continue
		}
		entrySource := entry.Source()
		value, ok := sources.ValueOfSource(ctx.Point, entrySource, in, readWithSamePointCallSource(ctx.Point, entrySource, read, out))
		if !ok {
			continue
		}
		staticMembers[key] = value
		if entrySource.HasExpr {
			if nested, ok := facts.ObjectLiteral(entrySource.ExprRef); ok {
				out = writeObjectLiteralHeap(ctx, facts, sources, read, in, out, entrySource, nested, active)
			}
		}
	}
	delete(active, source.ExprRef)
	out = out.WritePlacement(id, placement.Join(out.ReadPlacement(id), placement.Stack))
	return out.WriteHeapTableObject(ctx.Registry, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:          rootValue,
		StaticMembers: staticMembers,
	}))
}

func objectEntryTargetPath(root pathdom.Path, suffix pathdom.Path) (pathdom.Path, bool) {
	if root.IsEmpty() || len(suffix.Segments) == 0 {
		return pathdom.Path{}, false
	}
	out := copyPath(root)
	out.Segments = append(out.Segments, suffix.Segments...)
	return out, true
}
