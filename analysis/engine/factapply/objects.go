package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
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
	literal, ok := facts.ObjectLiteralView(valueSource.ExprRef)
	if !ok {
		return out
	}
	out = materializeObjectLiteralHeap(ctx, resolver, facts, sources, read, in, out, valueSource)
	if resolver == nil {
		return out
	}
	literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		entryPath, ok := entry.AppendSuffixTo(targetPath)
		if !ok {
			return true
		}
		if resolver.KeyAt(ctx.Point, entryPath) == "" {
			return true
		}
		source := entry.Source()
		value, ok := sources.ValueOfSource(ctx.Point, source, in, readWithSamePointCallSource(ctx.Point, source, read, out))
		if !ok {
			return true
		}
		value = objectEntryValue(ctx.Registry, entry, value)
		invalidated, ok := invalidatePathSubtreeAt(out, resolver, ctx.Point, entryPath)
		if !ok {
			return true
		}
		written, ok := writePathAt(ctx.Registry, invalidated, resolver, ctx.Point, entryPath, value)
		if !ok {
			return true
		}
		out = addPathEqualityProofFromSource(resolver, facts, ctx.Point, written, entryPath, source)
		return true
	})
	return out
}

func materializeObjectLiteralHeap(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	source factflow.ValueSource,
) state.State {
	if sources == nil || resolver == nil {
		return out
	}
	if !source.HasExpr {
		return out
	}
	literal, ok := facts.ObjectLiteralView(source.ExprRef)
	if !ok {
		return out
	}
	return writeObjectLiteralHeap(ctx, resolver, facts, sources, read, in, out, source, literal, nil)
}

func writeObjectLiteralHeap(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	source factflow.ValueSource,
	literal factflow.ObjectLiteralView,
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
	ks := resolver.KeySpace()
	staticMembers := make(map[keyspace.Key]product.Value, literal.EntryCount())
	literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		segments := entry.SuffixSegments()
		key, ok := ks.FromRootlessSuffix(segments)
		if !ok {
			return true
		}
		entrySource := entry.Source()
		value, ok := sources.ValueOfSource(ctx.Point, entrySource, in, readWithSamePointCallSource(ctx.Point, entrySource, read, out))
		if !ok {
			return true
		}
		value = objectEntryValue(ctx.Registry, entry, value)
		staticMembers[key] = value
		if canonical, ok := heapidentity.FieldCanonicalStaticMemberSuffixKey(ks, segments); ok {
			staticMembers[canonical] = value
		}
		if entrySource.HasExpr {
			if nested, ok := facts.ObjectLiteralView(entrySource.ExprRef); ok {
				out = writeObjectLiteralHeap(ctx, resolver, facts, sources, read, in, out, entrySource, nested, active)
			}
		}
		return true
	})
	delete(active, source.ExprRef)
	out = out.WritePlacement(id, placement.Join(out.ReadPlacement(id), placement.Stack))
	return out.WriteHeapTableObject(ctx.Registry, id, heapidentity.NewOwnedStaticTableObject(rootValue, staticMembers))
}

func objectEntryValue(reg *axis.Registry, entry factflow.ObjectEntryView, value product.Value) product.Value {
	expected, ok := entry.Expected()
	if !ok || reg == nil {
		return value
	}
	return overlayExpectedObjectEntryValue(reg, value, expected)
}

func overlayExpectedObjectEntryValue(reg *axis.Registry, value, expected product.Value) product.Value {
	ed := product.Edit(reg, value)
	kind := product.Get(reg, expected, runtimekind.Key)
	if !kind.IsTop() && !kind.IsBottom() {
		product.EditSet(&ed, runtimekind.Key, kind)
	}
	witness := product.Get(reg, expected, typewitness.Key)
	if !witness.IsTop() && !witness.IsBottom() {
		product.EditSet(&ed, typewitness.Key, witness)
	}
	origin := product.Get(reg, expected, variantorigin.Key)
	if !origin.IsTop() && !origin.IsBottom() {
		product.EditSet(&ed, variantorigin.Key, origin)
	}
	proof := product.Get(reg, expected, evidence.Key)
	if !evidence.Equal(proof, evidence.Top()) && !evidence.Equal(proof, evidence.Bottom()) {
		product.EditSet(&ed, evidence.Key, proof)
	}
	return ed.Done()
}
