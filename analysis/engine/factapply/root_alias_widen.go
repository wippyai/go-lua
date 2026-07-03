package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// applyCovariantExposures widens the source object's element/field witness for
// every covariant mutable-view exposure recorded at the point. A covariant
// exposure declares that a tracked object reachable at a source path is shared
// through a wider mutable view (alias, cast, reassignment, or container store)
// the heap cannot track writes back through, so a later narrow read of the
// source must reflect the wider contract regardless of where a write through the
// view lands. This is the single widening routine for every exposure site.
//
// A record exposure widens every (possibly nested) field of the exposed object
// whose contract field strictly supertypes the object's field, rebuilding the
// object's witness through the injected widener and dropping the precise
// per-field facts on those fields so a later read reflects the widened witness.
// A sub-path exposure (an individually exposed field/element such as
// narrow.inner) repairs the ancestor symbol's witness so the ancestor cannot
// re-project the narrow field type. An array exposure widens the element witness
// of an opaque array source; a heap-tracked array stays precise through
// identity-keyed element flow.
func applyCovariantExposures(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	widen CovariantWiden,
	facts factflow.Facts,
	out state.State,
) state.State {
	for _, exposure := range facts.CovariantExposures(ctx.Point) {
		out = applyCovariantExposure(ctx, resolver, widen, out, exposure)
	}
	return out
}

func applyCovariantExposure(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	widen CovariantWiden,
	out state.State,
	exposure factflow.CovariantExposure,
) state.State {
	sourcePath := exposure.SourcePath()
	if sourcePath.Symbol == 0 {
		return out
	}
	sourceKey := key.SymbolValue(sourcePath.Symbol)
	sourceValue := out.ReadValue(ctx.Registry, sourceKey)
	if product.Equal(ctx.Registry, sourceValue, product.Bottom(ctx.Registry)) {
		return out
	}
	wideWitness := product.Get(ctx.Registry, exposure.WideValue(), typewitness.Key)
	if wideWitness.IsTop() || wideWitness.IsBottom() {
		return out
	}
	if exposure.Kind() == factflow.CovariantExposureArray {
		return applyCovariantArrayExposure(ctx, out, sourceKey, sourceValue, wideWitness)
	}
	return applyCovariantRecordExposure(ctx, resolver, widen, out, sourcePath, sourceKey, sourceValue, wideWitness)
}

// applyCovariantArrayExposure widens an opaque array source's element witness to
// the exposure contract. A heap-tracked source (one carrying an exact identity)
// is left untouched so identity-keyed element flow keeps it precise.
func applyCovariantArrayExposure(
	ctx transfer.NodeContext,
	out state.State,
	sourceKey key.Value,
	sourceValue product.Value,
	wideWitness typewitness.Value,
) state.State {
	if _, tracked := product.Get(ctx.Registry, sourceValue, identity.Key).ID(); tracked {
		return out
	}
	sourceWitness := product.Get(ctx.Registry, sourceValue, typewitness.Key)
	if typewitness.Equal(sourceWitness, wideWitness) {
		return out
	}
	return out.WriteValue(ctx.Registry, sourceKey, product.Set(ctx.Registry, sourceValue, typewitness.Key, wideWitness))
}

// applyCovariantRecordExposure rebuilds the ancestor symbol witness through the
// injected widener so every strictly-wider field of the exposed object widens to
// the contract, writes it back, and drops the precise per-field facts on those
// fields so the structural read of a widened source field is not met back to the
// narrow value by a stale exact path-key or heap static member fact.
func applyCovariantRecordExposure(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	widen CovariantWiden,
	out state.State,
	sourcePath pathdom.Path,
	sourceKey key.Value,
	sourceValue product.Value,
	wideWitness typewitness.Value,
) state.State {
	if widen == nil {
		return out
	}
	sourceWitness := product.Get(ctx.Registry, sourceValue, typewitness.Key)
	sourceType, ok := sourceWitness.Type()
	if !ok {
		return out
	}
	contractType, ok := wideWitness.Type()
	if !ok {
		return out
	}
	widened, tops, ok := widen(sourceType, contractType, sourcePath.Segments)
	if !ok {
		return out
	}
	widenedWitness := typewitness.Of(widened)
	if widenedWitness.IsTop() || widenedWitness.IsBottom() {
		return out
	}
	out = out.WriteValue(ctx.Registry, sourceKey, product.Set(ctx.Registry, sourceValue, typewitness.Key, widenedWitness))

	if resolver == nil {
		return out
	}
	rootPath := pathdom.NewPath(sourcePath.Symbol, "")
	// Drop the precise per-field facts at and below the top-level widened field so a
	// read of the source field (or any nested field beneath it) is not met back to
	// the narrow declared value by a stale precise fact, and instead reflects the
	// widened structural witness written above. Dropping at the top widened segment
	// also clears the intermediate facts a parent-relative projection would
	// otherwise use to pin the narrow source.
	for _, top := range tops {
		fieldPath := rootPath.AppendSegments(top)
		if invalidated, ok := invalidatePathSubtreeAt(out, resolver, ctx.Point, fieldPath); ok {
			out = invalidated
		}
		if invalidated, ok := invalidateHeapStaticMemberSubtreeAt(ctx.Registry, out, resolver, ctx.Point, fieldPath); ok {
			out = invalidated
		}
	}
	return out
}

// covariantExposureSuppressesPathProof reports whether a bare-root covariant
// record exposure at point widens the root symbol named by source. Such an
// exposure suppresses the wide==narrow path-equality proof: a record's per-field
// facts flow through reference-equality member congruence, so the equality would
// reset the narrow source to Top on a write through the alias. An array exposure
// carries no per-member congruence and relies on the equality proof for its
// existing read-back diagnostics, so it keeps the equality. A sub-path exposure
// adds no such equality, so it needs no suppression.
func covariantExposureSuppressesPathProof(facts factflow.Facts, point cfg.Point, source factflow.ValueSource) bool {
	if !source.HasExpr {
		return false
	}
	sourcePath, ok := facts.ExpressionPathRef(source.ExprRef)
	if !ok || sourcePath.Symbol == 0 || len(sourcePath.Segments) != 0 {
		return false
	}
	for _, exposure := range facts.CovariantExposures(point) {
		if exposure.Kind() != factflow.CovariantExposureRecord {
			continue
		}
		ep := exposure.SourcePath()
		if ep.Symbol == sourcePath.Symbol && len(ep.Segments) == 0 {
			return true
		}
	}
	return false
}
