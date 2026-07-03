package visibility

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/ssa"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// VersionSource provides SSA version lookup for symbols at CFG points.
type VersionSource interface {
	VisibleVersion(cfg.Point, symbol.ID) ssa.Version
}

// InputVersionSource optionally provides the version visible before point-local
// definitions are applied. Resolvers fall back to VisibleVersion when a source
// does not expose input snapshots.
type InputVersionSource interface {
	VisibleVersionBefore(cfg.Point, symbol.ID) ssa.Version
}

// PathKeyResolver resolves a source path to the point-visible state key.
type PathKeyResolver interface {
	KeyAt(cfg.Point, pathdom.Path) pathdom.PathKey
	KeySpace() *keyspace.KeySpace
}

type versionedRoot struct {
	sym     symbol.ID
	version int
}

type singleSegment struct {
	root versionedRoot
	seg  segment.Segment
}

// Resolver provides normalized point-local state path keys from source paths.
// Every symbol-rooted state key includes a concrete nonzero SSA version.
type Resolver struct {
	source VersionSource
	root   map[versionedRoot]pathdom.PathKey
	single map[singleSegment]pathdom.PathKey
	keys   *keyspace.KeySpace
	input  bool
}

// NewResolver creates a resolver bound to a visibility source.
func NewResolver(source VersionSource) *Resolver {
	return &Resolver{
		source: source,
		root:   make(map[versionedRoot]pathdom.PathKey),
		single: make(map[singleSegment]pathdom.PathKey),
		keys:   keyspace.New(),
	}
}

// Before returns a resolver view that resolves symbol roots at the input to a
// CFG point. It shares the same keyspace and key caches, so before/boundary
// projections can compare state keys directly.
func (r *Resolver) Before() *Resolver {
	if r == nil {
		return nil
	}
	out := *r
	out.input = true
	return &out
}

// KeySpace returns the per-analysis structural key interner. Callers thread it
// into the path-evidence value lane alongside the value registry.
func (r *Resolver) KeySpace() *keyspace.KeySpace {
	if r == nil {
		return nil
	}
	return r.keys
}

// StructKeyAt returns the interned structural state key for path at point. Its
// Format() equals KeyAt(point, path); a non-point-local or unresolved path
// yields the invalid zero key (Format == "").
func (r *Resolver) StructKeyAt(point cfg.Point, path pathdom.Path) keyspace.Key {
	key, ok := AddressAt(r, point, path).VisibleLocalKeyspaceKey()
	if !ok {
		return keyspace.Key{}
	}
	return key
}

// VisibleLocalKeyspaceKeyAt returns the interned point-local value-lane key for
// path without formatting and reparsing the legacy PathKey string.
func (r *Resolver) VisibleLocalKeyspaceKeyAt(point cfg.Point, path pathdom.Path) (keyspace.Key, bool) {
	if path.IsEmpty() || path.Symbol == 0 || r == nil || r.source == nil || r.keys == nil {
		return keyspace.Key{}, false
	}
	version := r.visibleVersion(point, path.Symbol)
	if version.IsZero() {
		return keyspace.Key{}, false
	}
	return r.keys.FromResolverKey(path.Symbol, version.ID, path.Segments)
}

// VisibleKeyspaceKeyAt returns the interned key for the visible state spelling
// of path. Symbol roots use the point-local SSA version; placeholder roots keep
// their structural local identity.
func (r *Resolver) VisibleKeyspaceKeyAt(point cfg.Point, path pathdom.Path) (keyspace.Key, bool) {
	if path.IsEmpty() || r == nil || r.keys == nil {
		return keyspace.Key{}, false
	}
	if path.IsPlaceholder() {
		return r.keys.FromPath(path), true
	}
	return r.VisibleLocalKeyspaceKeyAt(point, path)
}

// RootOrVisibleKeyspaceKeyAt returns the interned key for facts that store
// root-symbol paths structurally but member paths under the point-visible SSA
// version.
func (r *Resolver) RootOrVisibleKeyspaceKeyAt(point cfg.Point, path pathdom.Path) (keyspace.Key, bool) {
	if path.IsEmpty() || path.Symbol == 0 || r == nil || r.keys == nil {
		return keyspace.Key{}, false
	}
	if len(path.Segments) == 0 {
		return r.keys.FromPath(path), true
	}
	return r.VisibleLocalKeyspaceKeyAt(point, path)
}

// KeyAt returns the point-visible state key for path at point. Symbol-rooted
// paths must resolve to a nonzero visible SSA version. Placeholder paths keep
// structural local identity because they are not point-local symbol values.
func (r *Resolver) KeyAt(point cfg.Point, path pathdom.Path) pathdom.PathKey {
	if path.IsEmpty() {
		return ""
	}
	if path.IsPlaceholder() {
		local, ok := pathaddr.LocalOfPath(path)
		if !ok {
			return ""
		}
		return local.LocalKey().PathKey()
	}
	if path.Symbol == 0 || r == nil || r.source == nil {
		return ""
	}
	version := r.visibleVersion(point, path.Symbol)
	if version.IsZero() {
		return ""
	}
	return r.KeyForVersion(path.Symbol, version.ID, path.Segments)
}

func (r *Resolver) visibleVersion(point cfg.Point, sym symbol.ID) ssa.Version {
	if r == nil || r.source == nil || sym == 0 {
		return ssa.Version{}
	}
	if r.input {
		if before, ok := r.source.(InputVersionSource); ok {
			if version := before.VisibleVersionBefore(point, sym); !version.IsZero() {
				return version
			}
		}
	}
	return r.source.VisibleVersion(point, sym)
}

// StateKeyAt returns the typed state-key carrier for KeyAt(point, path).
func (r *Resolver) StateKeyAt(point cfg.Point, path pathdom.Path) (pathaddr.StateKey, bool) {
	key := r.KeyAt(point, path)
	if key == "" {
		return "", false
	}
	return pathaddr.StateKeyFromPathKey(key)
}

// RootOrVisibleKeyAt returns the state key for facts that store root-symbol
// paths under their structural key but require visibility normalization for
// member paths.
func RootOrVisibleKeyAt(resolver PathKeyResolver, point cfg.Point, path pathdom.Path) pathdom.PathKey {
	key, ok := AddressAt(resolver, point, path).RootOrVisiblePathKey()
	if !ok {
		return ""
	}
	return key
}

// RootOrVisibleStateKeyAt returns the typed state key for facts that store
// root-symbol paths under their structural key but require visibility
// normalization for member paths.
func RootOrVisibleStateKeyAt(resolver PathKeyResolver, point cfg.Point, path pathdom.Path) (pathaddr.StateKey, bool) {
	return AddressAt(resolver, point, path).RootOrVisibleStateKey()
}

// KeyForVersion returns a point-local state key for an explicit nonzero SSA version.
func (r *Resolver) KeyForVersion(sym symbol.ID, version int, segments []segment.Segment) pathdom.PathKey {
	if sym == 0 || version <= 0 {
		return ""
	}
	if r != nil {
		cacheKey := versionedRoot{sym: sym, version: version}
		if len(segments) == 0 {
			if cached, ok := r.root[cacheKey]; ok {
				return cached
			}
			rootKey := pathdom.PathKey(pathaddr.VersionedRootString(sym, version))
			r.root[cacheKey] = rootKey
			return rootKey
		}
		if len(segments) == 1 {
			singleKey := singleSegment{root: cacheKey, seg: segments[0]}
			if cached, ok := r.single[singleKey]; ok {
				return cached
			}
			key, ok := pathaddr.LocalKeyForVersion(sym, version, segments)
			if !ok {
				return ""
			}
			r.single[singleKey] = key.PathKey()
			return key.PathKey()
		}
	}
	key, ok := pathaddr.LocalKeyForVersion(sym, version, segments)
	if !ok {
		return ""
	}
	return key.PathKey()
}
