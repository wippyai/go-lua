package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
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
		value = objectEntryValue(ctx.Registry, entry, value)
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
		value = objectEntryValue(ctx.Registry, entry, value)
		staticMembers[key] = value
		if canonical, ok := pathaddr.FieldCanonicalRelativeStaticMemberSuffixKey(entry.Suffix().Segments); ok {
			staticMembers[canonical] = value
		}
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

func objectEntryValue(reg *axis.Registry, entry factflow.ObjectEntry, value product.Value) product.Value {
	expected, ok := entry.Expected()
	if !ok || reg == nil {
		return value
	}
	return overlayExpectedObjectEntryValue(reg, value, expected)
}

func overlayExpectedObjectEntryValue(reg *axis.Registry, value, expected product.Value) product.Value {
	kind := product.Get(reg, expected, runtimekind.Key)
	if !kind.IsTop() && !kind.IsBottom() {
		value = product.Set(reg, value, runtimekind.Key, kind)
	}
	witness := product.Get(reg, expected, typewitness.Key)
	if !witness.IsTop() && !witness.IsBottom() {
		value = product.Set(reg, value, typewitness.Key, witness)
	}
	origin := product.Get(reg, expected, variantorigin.Key)
	if !origin.IsTop() && !origin.IsBottom() {
		value = product.Set(reg, value, variantorigin.Key, origin)
	}
	proof := product.Get(reg, expected, evidence.Key)
	if !evidence.Equal(proof, evidence.Top()) && !evidence.Equal(proof, evidence.Bottom()) {
		value = product.Set(reg, value, evidence.Key, proof)
	}
	return value
}

func objectEntryTargetPath(root pathdom.Path, suffix pathdom.Path) (pathdom.Path, bool) {
	if root.IsEmpty() || len(suffix.Segments) == 0 {
		return pathdom.Path{}, false
	}
	out := root.Clone()
	out.Segments = append(out.Segments, suffix.Segments...)
	return out, true
}
