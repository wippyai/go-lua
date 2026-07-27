package state

import (
	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/internal/mapedit"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

func boundaryContainsStateKey(keys *keyspace.KeySpace, closure BoundaryClosure, value pathaddr.StateKey) bool {
	path, ok := keys.FromStateKey(pathdom.PathKey(value.String()))
	return ok && closure.ContainsPath(path)
}

func boundaryRebasePaths(ctx *boundaryRebaseContext, path keyspace.Key) ([]keyspace.Key, bool) {
	if ctx != nil && ctx.structuralIdentity {
		if ctx.fromKeys != ctx.toKeys || path.Kind == keyspace.KindInvalid || ctx.fromKeys.FormatReadOnly(path) == "" {
			return nil, false
		}
		return []keyspace.Key{path}, true
	}
	if ctx != nil && ctx.formalRekey != nil {
		mapped, ok := ctx.formalRekey.rekey(path)
		if !ok {
			return nil, false
		}
		preimages := ctx.quotient.paths[mapped]
		seen := false
		for _, prior := range preimages {
			seen = seen || prior == path
		}
		if !seen {
			ctx.quotient.paths[mapped] = append(preimages, path)
		}
		return []keyspace.Key{mapped}, true
	}
	return rebaseBoundaryPaths(ctx.fromKeys, ctx.toKeys, ctx.roots, path)
}

func boundaryRebaseSlots(ctx *boundaryRebaseContext, slot key.Value) ([]key.Value, bool) {
	if ctx != nil && ctx.structuralIdentity {
		if slot == 0 {
			return nil, false
		}
		return []key.Value{slot}, true
	}
	next := ctx.slots[slot]
	return next, len(next) != 0
}

func projectFiniteMap[K comparable, V any](in map[K]V, keep func(K, V) bool) map[K]V {
	var out map[K]V
	for key, value := range in {
		if !keep(key, value) {
			continue
		}
		if out == nil {
			out = make(map[K]V)
		}
		out[key] = value
	}
	return out
}

func applyFiniteMap[K comparable, V any](destination, fragment map[K]V, remove func(K, V) bool) map[K]V {
	out := mapedit.Clone(destination)
	for key, value := range out {
		if remove(key, value) {
			delete(out, key)
		}
	}
	if out == nil && len(fragment) != 0 {
		out = make(map[K]V, len(fragment))
	}
	for key, value := range fragment {
		out[key] = value
	}
	return out
}

// applyFiniteMapEqual applies the same finite replacement as applyFiniteMap,
// but preserves the destination representation when that replacement is
// semantically unchanged. The equality proof is computed before cloning: a
// selected destination entry must be replaced by an equal fragment entry,
// every fragment addition must already denote an equal destination entry, and
// an unpaired selected entry is a real deletion. Published maps remain
// immutable; this is operand reuse, not an in-place update or result cache.
func applyFiniteMapEqual[K comparable, V any](
	destination, fragment map[K]V,
	remove func(K, V) bool,
	equal func(V, V) bool,
) map[K]V {
	unchanged := equal != nil
	if unchanged {
		for key, value := range destination {
			if !remove(key, value) {
				continue
			}
			replacement, present := fragment[key]
			if !present || !equal(value, replacement) {
				unchanged = false
				break
			}
		}
	}
	if unchanged {
		for key, value := range fragment {
			prior, present := destination[key]
			if !present || !equal(prior, value) {
				unchanged = false
				break
			}
		}
	}
	if unchanged {
		return destination
	}
	return applyFiniteMap(destination, fragment, remove)
}

func projectFiniteSet[T comparable](in map[T]struct{}, keep func(T) bool) map[T]struct{} {
	var out map[T]struct{}
	for value := range in {
		if keep(value) {
			if out == nil {
				out = make(map[T]struct{})
			}
			out[value] = struct{}{}
		}
	}
	return out
}

func applyFiniteSet[T comparable](destination, fragment map[T]struct{}, remove func(T) bool) map[T]struct{} {
	out := mapedit.Clone(destination)
	for value := range out {
		if remove(value) {
			delete(out, value)
		}
	}
	if out == nil && len(fragment) != 0 {
		out = make(map[T]struct{}, len(fragment))
	}
	for value := range fragment {
		out[value] = struct{}{}
	}
	return out
}

func rebaseBoundaryProduct(ctx *boundaryRebaseContext, value product.Value) (product.Value, bool) {
	return rebaseBoundaryValue(ctx, value)
}

// values: only explicitly-bound root slots cross the boundary.
