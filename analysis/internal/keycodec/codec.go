// Package keycodec owns small, shared string codecs for compact map keys.
package keycodec

import "strings"

// PrefixedDecimalKey returns prefix followed by value in base 10 and suffix.
func PrefixedDecimalKey(prefix byte, value uint64, suffix string) string {
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

// ParseUnsignedDecimal parses canonical unsigned base-10 with overflow checks.
func ParseUnsignedDecimal(s string) (uint64, bool) {
	if s == "" {
		return 0, false
	}
	if len(s) > 1 && s[0] == '0' {
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

// ParsePrefixedNonZeroDecimal parses prefix followed by a canonical nonzero
// unsigned decimal and returns the value plus the index just past the digits.
func ParsePrefixedNonZeroDecimal(s, prefix string) (uint64, int, bool) {
	if len(s) <= len(prefix) || !strings.HasPrefix(s, prefix) {
		return 0, 0, false
	}
	i := len(prefix)
	for i < len(s) {
		ch := s[i]
		if ch < '0' || ch > '9' {
			break
		}
		i++
	}
	n, parsed := ParseUnsignedDecimal(s[len(prefix):i])
	if !parsed || n == 0 {
		return 0, 0, false
	}
	return n, i, true
}

// ParsePositiveIntAfterAt parses a positive int that starts at i in s.
func ParsePositiveIntAfterAt(s string, i int) (int, int, bool) {
	if i < 0 || i >= len(s) {
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
	n, parsed := ParseUnsignedDecimal(s[start:i])
	if i == start || !parsed || n == 0 || n > uint64(int(^uint(0)>>1)) {
		return 0, 0, false
	}
	return int(n), i, true
}
