package address

import (
	"strconv"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/internal/keycodec"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// StableKey is the compact, version-insensitive key spelling for stable
// addresses. It is intentionally distinct from point-local path keys.
type StableKey pathdom.PathKey

// PathKey returns the PathKey carrier for existing map APIs.
func (k StableKey) PathKey() pathdom.PathKey { return pathdom.PathKey(k) }

// SymbolPathKey returns the stable, version-insensitive key for a symbol-rooted path.
func SymbolPathKey(sym symbol.ID, segments []segment.Segment) pathdom.PathKey {
	if sym == 0 {
		return ""
	}
	return pathdom.PathKey(SymbolStableKey(sym, segments))
}

// SymbolStableKey returns the typed stable key for a symbol-rooted address.
func SymbolStableKey(sym symbol.ID, segments []segment.Segment) StableKey {
	if sym == 0 {
		return ""
	}
	return StableKey(keycodec.PrefixedDecimalKey('s', uint64(sym), segment.FormatSegments(segments)))
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
	n, parsed, ok := parseStableSymbolRootSuffix(key)
	if !ok {
		return 0, nil, false
	}
	return symbol.ID(n), parsed.segments, true
}

func namedRootKey(root string, segments []segment.Segment) pathdom.PathKey {
	raw := pathdom.Path{Root: root, Segments: segments}.Key()
	if namedRootNeedsEncoding(root, segments, raw) {
		return pathdom.PathKey(encodeNamedRoot(root) + segment.FormatSegments(segments))
	}
	return raw
}

func namedRootNeedsEncoding(root string, segments []segment.Segment, key pathdom.PathKey) bool {
	s := string(key)
	if keycodec.LooksEncodedNamedRootKey(s) || keycodec.LooksStableSymbolRootSuffix(s) || keycodec.LooksResolverRootSuffix(s) {
		return true
	}
	if _, _, ok := ParseSymbolPathKey(key); ok {
		return true
	}
	if _, _, _, ok := ParseResolverPath(key); ok {
		return true
	}
	parsed, ok := parsePlainNamedRootSuffix(key)
	return !ok || parsed.root != root || !sameSegments(parsed.segments, segments)
}

func encodeNamedRoot(root string) string {
	return "n" + strconv.Itoa(len(root)) + ":" + root
}

func parseEncodedNamedRootKey(key string) (string, []segment.Segment, bool) {
	parsed, ok := parseEncodedNamedRootSuffix(key)
	if !ok {
		return "", nil, false
	}
	return parsed.root, parsed.segments, true
}

func sameSegments(a, b []segment.Segment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
