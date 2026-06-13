package address

import (
	"strconv"
	"strings"

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

// SymbolPathKeyOf lowers a resolved path to its stable symbol-path key.
func SymbolPathKeyOf(path pathdom.Path) (pathdom.PathKey, bool) {
	if path.Symbol == 0 {
		return "", false
	}
	key := SymbolPathKey(path.Symbol, path.Segments)
	return key, key != ""
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
	s := string(key)
	if len(s) < 2 || s[0] != 's' {
		return 0, nil, false
	}
	i := 1
	for i < len(s) {
		ch := s[i]
		if ch < '0' || ch > '9' {
			break
		}
		i++
	}
	n, parsed := keycodec.ParseUnsignedDecimal(s[1:i])
	if !parsed || n == 0 {
		return 0, nil, false
	}
	segments, ok := segment.InternFormattedSegments(s[i:])
	if !ok {
		return 0, nil, false
	}
	return symbol.ID(n), segments, true
}

func namedRootKey(root string, segments []segment.Segment) pathdom.PathKey {
	raw := pathdom.Path{Root: root, Segments: segments}.Key()
	if namedRootNeedsEncoding(raw) {
		return pathdom.PathKey(encodeNamedRoot(root) + segment.FormatSegments(segments))
	}
	return raw
}

func namedRootNeedsEncoding(key pathdom.PathKey) bool {
	if _, _, ok := ParseSymbolPathKey(key); ok {
		return true
	}
	if _, _, _, ok := ParseResolverPath(key); ok {
		return true
	}
	_, _, ok := parseEncodedNamedRootKey(string(key))
	return ok
}

func encodeNamedRoot(root string) string {
	return "n" + strconv.Itoa(len(root)) + ":" + root
}

func parseEncodedNamedRootKey(key string) (string, []segment.Segment, bool) {
	if len(key) < 3 || key[0] != 'n' {
		return "", nil, false
	}
	i := 1
	length := 0
	for i < len(key) {
		ch := key[i]
		if ch < '0' || ch > '9' {
			break
		}
		digit := int(ch - '0')
		if length > (int(^uint(0)>>1)-digit)/10 {
			return "", nil, false
		}
		length = length*10 + digit
		i++
	}
	if i == 1 || i >= len(key) || key[i] != ':' || length == 0 {
		return "", nil, false
	}
	rootStart := i + 1
	rootEnd := rootStart + length
	if rootEnd > len(key) {
		return "", nil, false
	}
	suffix := key[rootEnd:]
	segments, ok := segment.InternFormattedSegments(suffix)
	if !ok {
		return "", nil, false
	}
	return key[rootStart:rootEnd], segments, true
}

func parseRootAndSuffix(key pathdom.PathKey) (root string, suffix string, ok bool) {
	s := string(key)
	if s == "" {
		return "", "", false
	}
	if s[0] == '$' {
		end := 1
		for end < len(s) && s[end] >= '0' && s[end] <= '9' {
			end++
		}
		if end == 1 {
			return "", "", false
		}
		return s[:end], s[end:], segment.ValidFormattedSegments(s[end:])
	}
	if strings.HasPrefix(s, "ret[") {
		end := 4
		for end < len(s) && s[end] >= '0' && s[end] <= '9' {
			end++
		}
		if end == 4 || end >= len(s) || s[end] != ']' {
			return "", "", false
		}
		end++
		return s[:end], s[end:], segment.ValidFormattedSegments(s[end:])
	}
	end := 0
	for end < len(s) && s[end] != '.' && s[end] != '[' {
		end++
	}
	if end == 0 {
		return "", "", false
	}
	return s[:end], s[end:], segment.ValidFormattedSegments(s[end:])
}
