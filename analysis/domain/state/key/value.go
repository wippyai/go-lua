package key

import (
	"strconv"
	"strings"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keycodec"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// Value identifies one value cell in an abstract state.
type Value string

// ResolverPath identifies a point-local path refinement key emitted by state
// visibility/resolver code. Symbol roots must carry a nonzero SSA version.
type ResolverPath pathdom.PathKey

// PathKey returns the PathKey carrier for existing map APIs.
func (k ResolverPath) PathKey() pathdom.PathKey { return pathdom.PathKey(k) }

// SymbolValue identifies the current point-local value of a symbol.
func SymbolValue(sym symbol.ID) Value {
	if sym == 0 {
		return ""
	}
	return Value(keycodec.PrefixedDecimalKey('s', uint64(sym), ""))
}

// ReturnSlot identifies a non-symbol return value slot.
func ReturnSlot(index int) Value {
	if index < 0 {
		return ""
	}
	return Value(keycodec.PrefixedDecimalKey('r', uint64(index), ""))
}

// ParseSymbolValue inverts SymbolValue.
func ParseSymbolValue(value Value) (symbol.ID, bool) {
	s := string(value)
	if len(s) < 2 || s[0] != 's' {
		return 0, false
	}
	n, ok := keycodec.ParseUnsignedDecimal(s[1:])
	if !ok || n == 0 {
		return 0, false
	}
	return symbol.ID(n), true
}

// ParseReturnSlot inverts ReturnSlot.
func ParseReturnSlot(value Value) (int, bool) {
	s := string(value)
	if len(s) < 2 || s[0] != 'r' {
		return 0, false
	}
	n, ok := keycodec.ParseUnsignedDecimal(s[1:])
	if !ok || n > uint64(int(^uint(0)>>1)) {
		return 0, false
	}
	return int(n), true
}

// SymbolRoot returns the verbose resolver root for an unversioned symbol key.
func SymbolRoot(sym symbol.ID) string {
	if sym == 0 {
		return ""
	}
	return "sym" + strconv.FormatUint(uint64(sym), 10)
}

// SymbolVersionRoot returns the verbose resolver root for a versioned symbol key.
func SymbolVersionRoot(sym symbol.ID, version int) string {
	if sym == 0 || version <= 0 {
		return ""
	}
	return SymbolRoot(sym) + "@" + strconv.Itoa(version)
}

// SymbolVersionPath returns the verbose resolver key for a symbol/version path.
func SymbolVersionPath(sym symbol.ID, version int, segments []segment.Segment) pathdom.PathKey {
	return SymbolVersionResolverPath(sym, version, segments).PathKey()
}

// SymbolVersionResolverPath returns the typed verbose resolver key for a
// symbol/version path.
func SymbolVersionResolverPath(sym symbol.ID, version int, segments []segment.Segment) ResolverPath {
	root := SymbolVersionRoot(sym, version)
	if root == "" {
		return ""
	}
	return ResolverPath(root + segment.FormatSegments(segments))
}

// ParsePathKey extracts a verbose symbol key. Version is zero when absent.
func ParsePathKey(key pathdom.PathKey) (sym symbol.ID, version int, suffix string, ok bool) {
	return ParseResolverPath(key)
}

// ParseResolverPath extracts a verbose state resolver path key. Version is zero
// only for accepted plain/current symbol paths; point-local state keys require
// callers to reject zero-version symbol paths when appropriate.
func ParseResolverPath(key pathdom.PathKey) (sym symbol.ID, version int, suffix string, ok bool) {
	s := string(key)
	if !strings.HasPrefix(s, "sym") {
		return 0, 0, "", false
	}
	i := 3
	for i < len(s) {
		ch := s[i]
		if ch < '0' || ch > '9' {
			break
		}
		i++
	}
	n, parsed := keycodec.ParseUnsignedDecimal(s[3:i])
	if !parsed {
		return 0, 0, "", false
	}
	if n == 0 {
		return 0, 0, "", false
	}
	suffixStart := i
	if i < len(s) && s[i] == '@' {
		parsed, next, parsedOK := keycodec.ParsePositiveIntAfterAt(s, i+1)
		if !parsedOK {
			return 0, 0, "", false
		}
		version = parsed
		suffixStart = next
	}
	suffix = s[suffixStart:]
	if suffix != "" {
		switch suffix[0] {
		case '.', '[':
		default:
			return 0, 0, "", false
		}
		if !segment.ValidFormattedSegments(suffix) {
			return 0, 0, "", false
		}
	}
	return symbol.ID(n), version, suffix, true
}
