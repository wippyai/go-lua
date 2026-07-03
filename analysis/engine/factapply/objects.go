package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
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
	typeValues *typevalue.Cache,
) state.State {
	return applyObjectLiteralEntriesWithKnownSourceValue(ctx, resolver, facts, sources, read, in, out, targetPath, valueSource, product.Value{}, false, typeValues)
}

func applyObjectLiteralEntriesWithKnownSourceValue(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	targetPath pathdom.Path,
	valueSource factflow.ValueSource,
	knownSourceValue product.Value,
	hasKnownSourceValue bool,
	typeValues *typevalue.Cache,
) state.State {
	if !valueSource.HasExpr {
		return out
	}
	literal, ok := facts.ObjectLiteralView(valueSource.ExprRef)
	if !ok {
		return out
	}
	var entryValues *objectLiteralSourceCache
	out, entryValues = materializeObjectLiteralHeapCachedWithKnownSourceValue(ctx, resolver, facts, sources, read, in, out, valueSource, knownSourceValue, hasKnownSourceValue, typeValues)
	if resolver == nil {
		return out
	}
	writes := make([]objectLiteralEntryWrite, 0, literal.EntryCount())
	literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		entryPath, ok := entry.AppendSuffixTo(targetPath)
		if !ok {
			return true
		}
		if factPathKeyAt(resolver, ctx.Point, entryPath) == "" {
			return true
		}
		source := entry.Source()
		value, ok := objectLiteralEntrySourceValue(ctx, sources, read, in, out, entryValues, source)
		if !ok {
			return true
		}
		value = objectEntryValue(ctx.Registry, typeValues, entry, value)
		if rootEvidence, ok := untrustedRootEvidence(ctx.Registry, out, targetPath.Symbol); ok {
			value = product.Set(ctx.Registry, value, evidence.Key, rootEvidence)
		}
		writes = append(writes, objectLiteralEntryWrite{path: entryPath, source: source, value: value})
		return true
	})
	if applied, ok := applyIndependentObjectLiteralEntryWrites(ctx, resolver, facts, out, writes); ok {
		return applied
	}
	for _, write := range writes {
		invalidated, ok := invalidatePathSubtreeAt(out, resolver, ctx.Point, write.path)
		if !ok {
			continue
		}
		written, ok := writePathAt(ctx.Registry, invalidated, resolver, ctx.Point, write.path, write.value)
		if !ok {
			continue
		}
		out = addPathEqualityProofFromSource(resolver, facts, ctx.Point, written, write.path, write.source)
	}
	return out
}

type objectLiteralEntryWrite struct {
	path   pathdom.Path
	source factflow.ValueSource
	value  product.Value
}

func applyIndependentObjectLiteralEntryWrites(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	out state.State,
	writes []objectLiteralEntryWrite,
) (state.State, bool) {
	if len(writes) < 2 || !objectLiteralEntryWritesIndependent(writes) {
		return out, false
	}
	invalidated := out
	for _, write := range writes {
		next, ok := invalidatePathSubtreeAt(invalidated, resolver, ctx.Point, write.path)
		if !ok {
			return out, false
		}
		invalidated = next
	}
	writeKeys := make([][]keyspace.Key, len(writes))
	seenKeys := make(map[keyspace.Key]struct{}, len(writes)*2)
	for i, write := range writes {
		keys, ok := pathWriteLocalKeysAt(invalidated, resolver, ctx.Point, write.path)
		if !ok {
			return out, false
		}
		for _, key := range keys {
			if _, exists := seenKeys[key]; exists {
				return out, false
			}
			seenKeys[key] = struct{}{}
		}
		writeKeys[i] = keys
	}
	edit := invalidated.EditPathEvidence(ctx.Registry)
	for i, write := range writes {
		for _, key := range writeKeys[i] {
			edit.WriteLocalPathKey(key, write.value)
		}
	}
	written := edit.DoneOn(invalidated)
	for _, write := range writes {
		written = addPathEqualityProofFromSource(resolver, facts, ctx.Point, written, write.path, write.source)
	}
	return written, true
}

func objectLiteralEntryWritesIndependent(writes []objectLiteralEntryWrite) bool {
	for i := 0; i < len(writes); i++ {
		for j := i + 1; j < len(writes); j++ {
			if pathsOverlap(writes[i].path, writes[j].path) {
				return false
			}
		}
	}
	return true
}

func pathsOverlap(a, b pathdom.Path) bool {
	if a.Symbol != b.Symbol || a.Root != b.Root || a.Version != b.Version {
		return false
	}
	return segmentsPrefix(a.Segments, b.Segments) || segmentsPrefix(b.Segments, a.Segments)
}

func segmentsPrefix(prefix, full []segment.Segment) bool {
	if len(prefix) > len(full) {
		return false
	}
	for i := range prefix {
		if prefix[i] != full[i] {
			return false
		}
	}
	return true
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
	typeValues *typevalue.Cache,
) state.State {
	out, _ = materializeObjectLiteralHeapCached(ctx, resolver, facts, sources, read, in, out, source, typeValues)
	return out
}

func materializeObjectLiteralHeapCached(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	source factflow.ValueSource,
	typeValues *typevalue.Cache,
) (state.State, *objectLiteralSourceCache) {
	return materializeObjectLiteralHeapCachedWithKnownSourceValue(ctx, resolver, facts, sources, read, in, out, source, product.Value{}, false, typeValues)
}

func materializeObjectLiteralHeapCachedWithKnownSourceValue(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	source factflow.ValueSource,
	knownSourceValue product.Value,
	hasKnownSourceValue bool,
	typeValues *typevalue.Cache,
) (state.State, *objectLiteralSourceCache) {
	if sources == nil || resolver == nil {
		return out, nil
	}
	if !source.HasExpr {
		return out, nil
	}
	literal, ok := facts.ObjectLiteralView(source.ExprRef)
	if !ok {
		return out, nil
	}
	cache := newObjectLiteralSourceCache(ctx.Point, sources, read, in, out)
	if hasKnownSourceValue {
		cache.seed(source, knownSourceValue)
	}
	return writeObjectLiteralHeap(ctx, resolver, facts, in, out, source, literal, nil, typeValues, cache), cache
}

func writeObjectLiteralHeap(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	in state.State,
	out state.State,
	source factflow.ValueSource,
	literal factflow.ObjectLiteralView,
	active map[factflow.ExprRef]bool,
	typeValues *typevalue.Cache,
	sourceCache *objectLiteralSourceCache,
) state.State {
	if !source.HasExpr {
		return out
	}
	if active != nil && active[source.ExprRef] {
		return out
	}
	rootValue, ok := objectLiteralRootValue(ctx, typeValues, source, literal, sourceCache)
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
		segments := entry.SuffixSegmentsView()
		key, ok := ks.FromRootlessSuffix(segments)
		if !ok {
			return true
		}
		entrySource := entry.Source()
		value, ok := sourceCache.value(entrySource)
		if !ok {
			return true
		}
		value = objectEntryValue(ctx.Registry, typeValues, entry, value)
		staticMembers[key] = value
		if canonical, ok := heapidentity.FieldCanonicalStaticMemberSuffixKey(ks, segments); ok {
			staticMembers[canonical] = value
		}
		if entrySource.HasExpr {
			if nested, ok := facts.ObjectLiteralView(entrySource.ExprRef); ok {
				out = writeObjectLiteralHeap(ctx, resolver, facts, in, out, entrySource, nested, active, typeValues, sourceCache)
			}
		}
		return true
	})
	delete(active, source.ExprRef)
	out = out.WritePlacement(id, placement.Join(out.ReadPlacement(id), placement.Stack))
	return out.WriteHeapTableObject(ctx.Registry, id, heapidentity.NewOwnedStaticTableObject(rootValue, staticMembers))
}

type objectLiteralSourceCache struct {
	point  cfg.Point
	source sourcevalue.SourceValues
	read   func(cfg.Point) state.State
	in     state.State
	out    state.State
	values map[factflow.ValueSource]cachedObjectLiteralSourceValue
}

type cachedObjectLiteralSourceValue struct {
	value product.Value
	ok    bool
}

func newObjectLiteralSourceCache(
	point cfg.Point,
	source sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
) *objectLiteralSourceCache {
	return &objectLiteralSourceCache{
		point:  point,
		source: source,
		read:   read,
		in:     in,
		out:    out,
	}
}

func (c *objectLiteralSourceCache) value(source factflow.ValueSource) (product.Value, bool) {
	if c == nil || c.source == nil {
		return product.Value{}, false
	}
	if value, ok := c.cached(source); ok {
		return value.value, value.ok
	}
	value, ok := c.source.ValueOfSource(c.point, source, c.in, readWithCurrentPointState(c.point, c.read, c.out))
	if c.values == nil {
		c.values = make(map[factflow.ValueSource]cachedObjectLiteralSourceValue)
	}
	c.values[source] = cachedObjectLiteralSourceValue{value: value, ok: ok}
	return value, ok
}

func (c *objectLiteralSourceCache) ResolveValueSource(source factflow.ValueSource) (product.Value, bool) {
	return c.value(source)
}

func (c *objectLiteralSourceCache) cached(source factflow.ValueSource) (cachedObjectLiteralSourceValue, bool) {
	if c == nil || c.values == nil {
		return cachedObjectLiteralSourceValue{}, false
	}
	value, ok := c.values[source]
	return value, ok
}

func (c *objectLiteralSourceCache) seed(source factflow.ValueSource, value product.Value) {
	if c == nil {
		return
	}
	if c.values == nil {
		c.values = make(map[factflow.ValueSource]cachedObjectLiteralSourceValue, 1)
	}
	c.values[source] = cachedObjectLiteralSourceValue{value: value, ok: true}
}

func objectLiteralEntrySourceValue(
	ctx transfer.NodeContext,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	cache *objectLiteralSourceCache,
	source factflow.ValueSource,
) (product.Value, bool) {
	if cache != nil {
		return cache.value(source)
	}
	if sources == nil {
		return product.Value{}, false
	}
	return sources.ValueOfSource(ctx.Point, source, in, readWithCurrentPointState(ctx.Point, read, out))
}

func objectLiteralRootValue(
	ctx transfer.NodeContext,
	typeValues *typevalue.Cache,
	source factflow.ValueSource,
	literal factflow.ObjectLiteralView,
	sourceCache *objectLiteralSourceCache,
) (product.Value, bool) {
	if cached, ok := sourceCache.cached(source); ok && cached.ok {
		if _, hasIdentity := product.Get(ctx.Registry, cached.value, identity.Key).ID(); hasIdentity && typevalue.HasWitness(ctx.Registry, cached.value) {
			return cached.value, true
		}
	}
	if _, ok := literal.Identity(); ok {
		t, ok := luasourcevalue.ObjectLiteralTypeViewCached(ctx.Registry, typeValues, literal, sourceCache)
		if !ok {
			return product.Value{}, false
		}
		return luasourcevalue.ObjectLiteralValueFromTypeCached(ctx.Registry, typeValues, literal, t), true
	}
	return sourceCache.value(source)
}

func objectEntryValue(reg *axis.Registry, typeValues *typevalue.Cache, entry factflow.ObjectEntryView, value product.Value) product.Value {
	expected, ok := entry.Expected()
	if !ok || reg == nil {
		return value
	}
	return overlayExpectedObjectEntryValue(reg, typeValues, value, expected)
}

func overlayExpectedObjectEntryValue(reg *axis.Registry, typeValues *typevalue.Cache, value, expected product.Value) product.Value {
	if !luasourcevalue.ExpectedEntryAdmissibleCached(reg, typeValues, value, expected) {
		return value
	}
	ed := product.Edit(reg, value)
	kind := product.Get(reg, expected, runtimekind.Key)
	if !kind.IsTop() && !kind.IsBottom() {
		product.EditSet(&ed, runtimekind.Key, kind)
	}
	witness := product.Get(reg, expected, typewitness.Key)
	if !witness.IsTop() && !witness.IsBottom() && expectedObjectEntryWitnessShouldReplace(reg, typeValues, value, expected) {
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

func expectedObjectEntryWitnessShouldReplace(reg *axis.Registry, typeValues *typevalue.Cache, value, expected product.Value) bool {
	expectedType, expectedOK := typevalue.TypeOf(reg, expected)
	if !expectedOK || expectedType == nil {
		return false
	}
	valueType, valueOK := typevalue.TypeOf(reg, value)
	if !valueOK || valueType == nil {
		return true
	}
	return !typeValues.IsSubtype(valueType, expectedType)
}
