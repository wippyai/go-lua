package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
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
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
)

type objectLiteralLookup func(factflow.ExprRef) (factflow.ObjectLiteralView, bool)

func objectLiteralContiguousListLengthFloor(literal factflow.ObjectLiteralView) int64 {
	if literal.EntryCount() == 0 {
		return 0
	}
	present := make(map[int]struct{}, literal.EntryCount())
	literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		if entry.SuffixSegmentCount() != 1 {
			return true
		}
		seg, ok := entry.SuffixSegmentAt(0)
		if !ok || seg.Kind != segment.SegmentIndexInt || seg.Index <= 0 {
			return true
		}
		present[seg.Index] = struct{}{}
		return true
	})
	var floor int64
	for i := 1; ; i++ {
		if _, ok := present[i]; !ok {
			return floor
		}
		floor = int64(i)
	}
}

// ObjectLiteralListLengthFloor is the canonical structural list-prefix
// calculation shared by concrete resolution and symbolic N4 compilation.
func ObjectLiteralListLengthFloor(literal factflow.ObjectLiteralView) int64 {
	return objectLiteralContiguousListLengthFloor(literal)
}

// materializeObjectLiteralHeapBatchCached resolves one ordered set of object-
// literal roots into the callback-free ResolvedPathStoreObject vocabulary and
// publishes the complete graph through the sole resolved heap kernel. One
// source cache and one active/done traversal are shared by the whole batch, so
// repeated call arguments and shared nested literals are evaluated exactly
// once. Resolution is atomic: no heap is published unless every reachable
// literal is well formed and acyclic.
func materializeObjectLiteralHeapBatchCached(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	roots []factflow.ValueSource,
	typeValues *typevalue.Cache,
) (state.State, *objectLiteralSourceCache) {
	if sources == nil || resolver == nil {
		return out, nil
	}
	cache := newObjectLiteralSourceCache(ctx.Point, sources, read, in, out)
	return materializeObjectLiteralHeapBatchWithCache(ctx, resolver, facts.ObjectLiteralView, out, roots, typeValues, cache), cache
}

func materializeObjectLiteralHeapBatchWithCache(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	lookup objectLiteralLookup,
	out state.State,
	roots []factflow.ValueSource,
	typeValues *typevalue.Cache,
	sourceCache *objectLiteralSourceCache,
) state.State {
	if resolver == nil || sourceCache == nil || len(roots) == 0 {
		return out
	}
	active := make(map[factflow.ExprRef]bool)
	done := make(map[factflow.ExprRef]bool)
	heaps := make([]ResolvedPathStoreHeapObject, 0, len(roots))
	var resolve func(factflow.ValueSource) bool
	resolve = func(source factflow.ValueSource) bool {
		if !source.HasExpr {
			return true
		}
		literal, object := lookup(source.ExprRef)
		if !object {
			return true
		}
		if _, identified := literal.Identity(); !identified || active[source.ExprRef] {
			return false
		}
		if done[source.ExprRef] {
			return true
		}
		active[source.ExprRef] = true
		defer delete(active, source.ExprRef)
		rootValue, ok := objectLiteralRootValue(ctx, typeValues, literal, sourceCache)
		if !ok {
			return false
		}
		if _, identified := product.Get(ctx.Registry, rootValue, identity.Key).ID(); !identified {
			return false
		}
		heap := ResolvedPathStoreHeapObject{Root: rootValue, Members: make([]ResolvedPathStoreHeapMember, 0, literal.EntryCount())}
		_, hasListTail := literal.ListElementSource()
		heap.StableShape = literal.StaticStringKeysComplete() && !hasListTail
		exact := true
		literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
			entrySource := entry.Source()
			if !resolve(entrySource) {
				exact = false
				return false
			}
			value, available := sourceCache.value(entrySource)
			if !available {
				return true
			}
			member := ResolvedPathStoreHeapMember{Suffix: entry.SuffixSegments(), Value: objectEntryValue(ctx.Registry, typeValues, entry, value)}
			heap.Members = append(heap.Members, member)
			return true
		})
		if !exact {
			return false
		}
		heaps = append(heaps, heap)
		done[source.ExprRef] = true
		return true
	}
	for _, root := range roots {
		if !resolve(root) {
			return out
		}
	}
	if len(heaps) == 0 {
		return out
	}
	return applyResolvedObjectHeaps(ctx.Registry, resolver.KeySpace(), out, heaps)
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

func objectLiteralRootValue(
	ctx transfer.NodeContext,
	typeValues *typevalue.Cache,
	literal factflow.ObjectLiteralView,
	sourceCache *objectLiteralSourceCache,
) (product.Value, bool) {
	return luasourcevalue.ObjectLiteralValueFromViewCached(ctx.Registry, typeValues, literal, sourceCache)
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
