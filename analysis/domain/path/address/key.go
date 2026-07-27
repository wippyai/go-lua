package address

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// StableKey carries a canonical key admitted by stable-address semantics.
type StableKey struct {
	key pathdom.PathKey
}

// PathKey returns the PathKey carrier for existing map APIs.
func (k StableKey) PathKey() pathdom.PathKey { return k.key }

// SymbolPathKey returns the stable, version-insensitive key for a symbol-rooted path.
func SymbolPathKey(sym symbol.ID, segments []segment.Segment) pathdom.PathKey {
	if sym == 0 {
		return ""
	}
	return SymbolStableKey(sym, segments).PathKey()
}

// SymbolStableKey returns the typed stable key for a symbol-rooted address.
func SymbolStableKey(sym symbol.ID, segments []segment.Segment) StableKey {
	if sym == 0 {
		return StableKey{}
	}
	return StableKey{key: pathdom.FormatKey(pathdom.Path{Symbol: sym, Segments: segments})}
}

// ParseSymbolPathKey inverts SymbolPathKey and returns a defensive segment copy.
func ParseSymbolPathKey(key pathdom.PathKey) (symbol.ID, []segment.Segment, bool) {
	sym, segments, ok := parseInternedSymbolPathKey(key)
	if !ok {
		return 0, nil, false
	}
	return sym, cloneSegments(segments), true
}

func parseInternedSymbolPathKey(key pathdom.PathKey) (symbol.ID, []segment.Segment, bool) {
	path, ok := pathdom.ParseKey(key)
	if !ok || path.Symbol == 0 || path.Version != 0 {
		return 0, nil, false
	}
	return path.Symbol, path.Segments, true
}

func namedRootKey(root string, segments []segment.Segment) pathdom.PathKey {
	return pathdom.FormatKey(pathdom.Path{Root: root, Segments: segments})
}
