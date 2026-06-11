package visibility

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/ssa"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// VersionSource provides SSA version lookup for symbols at CFG points.
type VersionSource interface {
	VisibleVersion(cfg.Point, symbol.ID) ssa.Version
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
}

// NewResolver creates a resolver bound to a visibility source.
func NewResolver(source VersionSource) *Resolver {
	return &Resolver{
		source: source,
		root:   make(map[versionedRoot]pathdom.PathKey),
		single: make(map[singleSegment]pathdom.PathKey),
	}
}

// KeyAt returns the versioned state key for path at point. Placeholder paths use
// their current canonical path key because they are not point-local symbol
// values. Symbol-rooted paths must resolve to a nonzero visible SSA version.
func (r *Resolver) KeyAt(point cfg.Point, path pathdom.Path) pathdom.PathKey {
	if path.IsEmpty() {
		return ""
	}
	if path.IsPlaceholder() {
		return path.Key()
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

// KeyForVersion returns a point-local state key for an explicit nonzero SSA version.
func (r *Resolver) KeyForVersion(sym symbol.ID, version int, segments []segment.Segment) pathdom.PathKey {
	if sym == 0 || version <= 0 {
		return ""
	}
	rootKey := r.rootKey(sym, version)
	if len(segments) == 0 {
		return rootKey
	}
	if len(segments) == 1 && r != nil {
		cacheKey := singleSegment{
			root: versionedRoot{sym: sym, version: version},
			seg:  segments[0],
		}
		if cached, ok := r.single[cacheKey]; ok {
			return cached
		}
		key := pathdom.PathKey(string(rootKey) + segment.FormatSegments(segments))
		r.single[cacheKey] = key
		return key
	}
	return pathdom.PathKey(string(rootKey) + segment.FormatSegments(segments))
}

func (r *Resolver) rootKey(sym symbol.ID, version int) pathdom.PathKey {
	if r != nil {
		cacheKey := versionedRoot{sym: sym, version: version}
		if cached, ok := r.root[cacheKey]; ok {
			return cached
		}
		rootKey := pathdom.PathKey(key.SymbolVersionRoot(sym, version))
		r.root[cacheKey] = rootKey
		return rootKey
	}
	return pathdom.PathKey(key.SymbolVersionRoot(sym, version))
}
