package numparse

import (
	"math"
	"strconv"
	"strings"
)

// ParseIntegerLiteral parses integer-syntax numeric literals.
//
// Supported forms:
//   - Decimal integers (including leading zeros), e.g. "42", "08"
//   - Hex integers, e.g. "0xDEAD", "-0x10"
//
// Rejected forms:
//   - Decimal/hex float syntax ('.', exponent markers)
func ParseIntegerLiteral(s string) (int64, bool) {
	if s == "" || strings.Contains(s, ".") {
		return 0, false
	}

	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "0x") || strings.HasPrefix(lower, "+0x") || strings.HasPrefix(lower, "-0x") {
		// Hex float syntax uses p/P exponents (e.g. 0x1p2).
		if strings.ContainsAny(lower, "p") {
			return 0, false
		}
		i, err := strconv.ParseInt(s, 0, 64)
		if err != nil {
			return 0, false
		}
		return i, true
	}

	// Decimal scientific notation is not integer syntax.
	if strings.ContainsAny(s, "eE") {
		return 0, false
	}

	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return i, true
}

// ParseNumberLiteral parses a numeric literal.
//
// Returns one of:
//   - (intValue, 0, true) for integer-syntax literals
//   - (0, floatValue, true) for float-syntax literals
//   - (_, _, false) when parsing fails
func ParseNumberLiteral(s string) (int64, float64, bool) {
	if s == "" {
		return 0, 0, false
	}
	if i, ok := ParseIntegerLiteral(s); ok {
		return i, 0, true
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return 0, f, true
	}
	return 0, 0, false
}

// ParseFloatLiteral parses a numeric literal as float64.
//
// Integer literals are accepted and converted to float64.
func ParseFloatLiteral(s string) (float64, bool) {
	if i, ok := ParseIntegerLiteral(s); ok {
		return float64(i), true
	}
	_, f, ok := ParseNumberLiteral(s)
	if !ok {
		return 0, false
	}
	return f, true
}

// ParseIntegralLiteral parses a numeric literal as an integral int64 value.
//
// Unlike ParseIntegerLiteral, this accepts float-syntax literals whose value is
// mathematically integral (for example "1.0", "1e0", "0x1p0").
func ParseIntegralLiteral(s string) (int64, bool) {
	if i, ok := ParseIntegerLiteral(s); ok {
		return i, true
	}
	f, ok := ParseFloatLiteral(s)
	if !ok || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, false
	}
	if math.Trunc(f) != f {
		return 0, false
	}
	if f < math.MinInt64 || f > math.MaxInt64 {
		return 0, false
	}
	return int64(f), true
}
