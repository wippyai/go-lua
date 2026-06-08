package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// ValueKey is the canonical key of one value cell in a PointState Env.
//
// It is intentionally not a plain string at the domain boundary. The encoded form
// is still deterministic and compact for maps/cache keys, but callers must choose
// a constructor that states what the key denotes.
type ValueKey string

// SymbolValueKey identifies the current point-local value of a CFG symbol.
func SymbolValueKey(sym cfg.SymbolID) ValueKey {
	return prefixedValueKey('s', uint64(sym))
}

// SymbolPathKey identifies a path rooted at a CFG symbol for components, such as
// numeric length facts, that need a stable key for a nested container. A bare
// symbol path uses the same encoding as SymbolValueKey; nested paths append the
// canonical structured segment suffix. Callers pass structured segments so the
// string form stays an internal deterministic cache key, not a semantic API.
func SymbolPathKey(sym cfg.SymbolID, segments []constraint.Segment) constraint.PathKey {
	key := string(SymbolValueKey(sym))
	if len(segments) > 0 {
		key += constraint.FormatSegments(segments)
	}
	return constraint.PathKey(key)
}

// SymbolPathKeyOf lowers a resolved constraint path to the canonical symbol-path
// key used by PointState components that reason about paths rather than values.
func SymbolPathKeyOf(path constraint.Path) (constraint.PathKey, bool) {
	if path.Symbol == 0 {
		return "", false
	}
	key := SymbolPathKey(path.Symbol, path.Segments)
	return key, key != ""
}

// ParseSymbolPathKey inverts SymbolPathKey. Non-symbol path keys return false.
func ParseSymbolPathKey(key constraint.PathKey) (cfg.SymbolID, []constraint.Segment, bool) {
	sym, segments, ok := parseInternedSymbolPathKey(key)
	if !ok {
		return 0, nil, false
	}
	return sym, cloneAddressSegments(segments), true
}

func parseInternedSymbolPathKey(key constraint.PathKey) (cfg.SymbolID, []constraint.Segment, bool) {
	s := string(key)
	if len(s) < 2 || s[0] != 's' {
		return 0, nil, false
	}
	i := 1
	var n uint64
	for i < len(s) {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + uint64(c-'0')
		i++
	}
	if n == 0 {
		return 0, nil, false
	}
	segments, ok := constraint.InternFormattedSegments(s[i:])
	if !ok {
		return 0, nil, false
	}
	return cfg.SymbolID(n), segments, true
}

// ReturnSlotValueKey identifies the value stashed for a non-identifier return
// expression at a return point.
func ReturnSlotValueKey(i int) ValueKey {
	return prefixedValueKey('r', uint64(i))
}

// ParseReturnSlotValueKey inverts ReturnSlotValueKey. Non-return-slot keys or
// negative/empty slot encodings return false.
func ParseReturnSlotValueKey(key ValueKey) (int, bool) {
	s := string(key)
	if len(s) < 2 || s[0] != 'r' {
		return 0, false
	}
	var n uint64
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + uint64(c-'0')
	}
	if n > uint64(^uint(0)>>1) {
		return 0, false
	}
	return int(n), true
}

// ParseSymbolValueKey inverts SymbolValueKey. Non-symbol keys return false.
func ParseSymbolValueKey(key ValueKey) (cfg.SymbolID, bool) {
	s := string(key)
	if len(s) < 2 || s[0] != 's' {
		return 0, false
	}
	var n uint64
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + uint64(c-'0')
	}
	if n == 0 {
		return 0, false
	}
	return cfg.SymbolID(n), true
}

func prefixedValueKey(prefix byte, v uint64) ValueKey {
	var buf [21]byte
	i := len(buf)
	for {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
		if v == 0 {
			break
		}
	}
	i--
	buf[i] = prefix
	return ValueKey(string(buf[i:]))
}
