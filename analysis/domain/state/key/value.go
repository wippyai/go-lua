package key

import (
	"strconv"
	"strings"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// Value identifies one value cell in an abstract state.
type Value string

// SymbolValue identifies the current point-local value of a symbol.
func SymbolValue(sym symbol.ID) Value {
	if sym == 0 {
		return ""
	}
	return Value(prefixedDecimalKey('s', uint64(sym), ""))
}

// ReturnSlot identifies a non-symbol return value slot.
func ReturnSlot(index int) Value {
	if index < 0 {
		return ""
	}
	return Value(prefixedDecimalKey('r', uint64(index), ""))
}

// ParseSymbolValue inverts SymbolValue.
func ParseSymbolValue(value Value) (symbol.ID, bool) {
	s := string(value)
	if len(s) < 2 || s[0] != 's' {
		return 0, false
	}
	n, ok := parseUnsignedDecimal(s[1:])
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
	n, ok := parseUnsignedDecimal(s[1:])
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
	root := SymbolVersionRoot(sym, version)
	if root == "" {
		return ""
	}
	return pathdom.PathKey(root + segment.FormatSegments(segments))
}

// ParsePathKey extracts a verbose symbol key. Version is zero when absent.
func ParsePathKey(key pathdom.PathKey) (sym symbol.ID, version int, suffix string, ok bool) {
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
	n, parsed := parseUnsignedDecimal(s[3:i])
	if !parsed {
		return 0, 0, "", false
	}
	if n == 0 {
		return 0, 0, "", false
	}
	suffixStart := i
	if i < len(s) && s[i] == '@' {
		parsed, next, parsedOK := parseVersionAfterAt(s, i+1)
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

func parseVersionAfterAt(s string, i int) (int, int, bool) {
	if i >= len(s) {
		return 0, 0, false
	}
	start := i
	for i < len(s) {
		ch := s[i]
		if ch < '0' || ch > '9' {
			break
		}
		i++
	}
	n, parsed := parseUnsignedDecimal(s[start:i])
	if i == start || n == 0 || n > uint64(int(^uint(0)>>1)) {
		return 0, 0, false
	}
	if !parsed {
		return 0, 0, false
	}
	return int(n), i, true
}

func parseUnsignedDecimal(s string) (uint64, bool) {
	if s == "" {
		return 0, false
	}
	var n uint64
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch < '0' || ch > '9' {
			return 0, false
		}
		digit := uint64(ch - '0')
		if n > (^uint64(0)-digit)/10 {
			return 0, false
		}
		n = n*10 + digit
	}
	return n, true
}

func prefixedDecimalKey(prefix byte, value uint64, suffix string) string {
	var buf [21]byte
	i := len(buf)
	for {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
		if value == 0 {
			break
		}
	}
	i--
	buf[i] = prefix
	if suffix == "" {
		return string(buf[i:])
	}
	out := make([]byte, len(buf)-i+len(suffix))
	copy(out, buf[i:])
	copy(out[len(buf)-i:], suffix)
	return string(out)
}
