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
	if r == nil || r.keys == nil {
		return keyspace.Key{}
	}
	key, ok := r.keys.FromPathKey(r.KeyAt(point, path))
	if !ok {
		return keyspace.Key{}
	}
	return key
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
	version := r.source.VisibleVersion(point, path.Symbol)
	if version.IsZero() {
		return ""
	}
	return r.KeyForVersion(path.Symbol, version.ID, path.Segments)
}

// RootOrVisibleKeyAt returns the state key for facts that store root-symbol
// paths under their structural key but require visibility normalization for
// member paths.
func RootOrVisibleKeyAt(resolver PathKeyResolver, point cfg.Point, path pathdom.Path) pathdom.PathKey {
	if path.IsEmpty() || path.Symbol == 0 {
		return ""
	}
	if len(path.Segments) == 0 {
		return path.Key()
	}
	if resolver == nil {
		return ""
	}
	return resolver.KeyAt(point, path)
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
