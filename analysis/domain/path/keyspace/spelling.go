package keyspace

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
)

// This file ports the spelling-space disambiguation predicates from the address
// package so the canonical stable-named encoding can be reproduced without an
// import dependency. The predicates are byte-for-byte equivalent to their
// address counterparts (see analysis/domain/path/address/key.go,
// root_suffix.go); the equivalence test pins them against the address oracle.

func isDecimalDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

// looksEncodedNamedRootKey mirrors address.looksEncodedNamedRootKey.
func looksEncodedNamedRootKey(s string) bool {
	if len(s) < 3 || s[0] != 'n' || !isDecimalDigit(s[1]) {
		return false
	}
	i := 2
	for i < len(s) && isDecimalDigit(s[i]) {
		i++
	}
	return i < len(s) && s[i] == ':'
}

// looksStableSymbolRootSuffix mirrors address.looksStableSymbolRootSuffix.
func looksStableSymbolRootSuffix(s string) bool {
	if len(s) < 2 || s[0] != 's' || !isDecimalDigit(s[1]) {
		return false
	}
	i := 2
	for i < len(s) && isDecimalDigit(s[i]) {
		i++
	}
	return i == len(s) || s[i] == '.' || s[i] == '['
}

// looksResolverRootSuffix mirrors address.looksResolverRootSuffix.
func looksResolverRootSuffix(s string) bool {
	if !strings.HasPrefix(s, "sym") || len(s) < 4 || !isDecimalDigit(s[3]) {
		return false
	}
	i := 4
	for i < len(s) && isDecimalDigit(s[i]) {
		i++
	}
	return i == len(s) || s[i] == '@' || s[i] == '.' || s[i] == '['
}

// symbolPathKeyParses reports whether raw is a valid stable symbol key s<id><suffix>.
func symbolPathKeyParses(raw string) bool {
	n, end, ok := parseDecimalRootEnd(raw, "s")
	if !ok || n == 0 {
		return false
	}
	_, ok = segment.InternFormattedSegments(raw[end:])
	return ok
}

// resolverPathKeyParses reports whether raw is a valid resolver key
// sym<id>[@<ver>]<suffix>.
func resolverPathKeyParses(raw string) bool {
	n, suffixStart, ok := parseDecimalRootEnd(raw, "sym")
	if !ok || n == 0 {
		return false
	}
	if suffixStart < len(raw) && raw[suffixStart] == '@' {
		ver, next, vok := parsePositiveIntAfterAt(raw, suffixStart+1)
		if !vok || ver == 0 {
			return false
		}
		suffixStart = next
	}
	_, ok = segment.InternFormattedSegments(raw[suffixStart:])
	return ok
}

// parsePlainNamedRoot mirrors address.parsePlainNamedRootSuffix, returning the
// root spelling and its parsed segments.
func parsePlainNamedRoot(s string) (string, []segment.Segment, bool) {
	if s == "" {
		return "", nil, false
	}
	if s[0] == '$' {
		end := 1
		for end < len(s) && isDecimalDigit(s[end]) {
			end++
		}
		if end == 1 {
			return "", nil, false
		}
		return parsePlainRootSuffix(s, end)
	}
	if strings.HasPrefix(s, "ret[") {
		end := 4
		for end < len(s) && isDecimalDigit(s[end]) {
			end++
		}
		if end == 4 || end >= len(s) || s[end] != ']' {
			return "", nil, false
		}
		end++
		return parsePlainRootSuffix(s, end)
	}
	end := 0
	for end < len(s) && s[end] != '.' && s[end] != '[' {
		end++
	}
	if end == 0 {
		return "", nil, false
	}
	return parsePlainRootSuffix(s, end)
}

func parsePlainRootSuffix(s string, rootEnd int) (string, []segment.Segment, bool) {
	segments, ok := segment.InternFormattedSegments(s[rootEnd:])
	if !ok {
		return "", nil, false
	}
	return s[:rootEnd], segments, true
}

// parseDecimalRootEnd mirrors address.parseDecimalRootEnd: it parses a canonical
// unsigned decimal immediately after prefix and returns the value and the index
// just past the digits. A nonzero, non-zero-padded value is required.
func parseDecimalRootEnd(s, prefix string) (uint64, int, bool) {
	if len(s) <= len(prefix) || !strings.HasPrefix(s, prefix) {
		return 0, 0, false
	}
	i := len(prefix)
	for i < len(s) && isDecimalDigit(s[i]) {
		i++
	}
	n, ok := parseUnsignedDecimal(s[len(prefix):i])
	if !ok || n == 0 {
		return 0, 0, false
	}
	return n, i, true
}

// parseUnsignedDecimal mirrors keycodec.ParseUnsignedDecimal: canonical base-10,
// no leading zero on multi-digit values, overflow-checked.
func parseUnsignedDecimal(s string) (uint64, bool) {
	if s == "" {
		return 0, false
	}
	if len(s) > 1 && s[0] == '0' {
		return 0, false
	}
	var n uint64
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if !isDecimalDigit(ch) {
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

// parsePositiveIntAfterAt mirrors keycodec.ParsePositiveIntAfterAt.
func parsePositiveIntAfterAt(s string, i int) (int, int, bool) {
	if i < 0 || i >= len(s) {
		return 0, 0, false
	}
	start := i
	for i < len(s) && isDecimalDigit(s[i]) {
		i++
	}
	n, ok := parseUnsignedDecimal(s[start:i])
	if i == start || !ok || n == 0 || n > uint64(int(^uint(0)>>1)) {
		return 0, 0, false
	}
	return int(n), i, true
}
