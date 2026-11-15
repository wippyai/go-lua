package numparse

import (
	"errors"
	"math"
	"strconv"
)

// ParseNonNegativeDecimalInt parses a base-10 non-negative integer into int.
//
// Accepted forms:
//   - One or more ASCII digits, e.g. "0", "42", "001"
//
// Rejected forms:
//   - Empty strings
//   - Signs, non-digit characters, and overflow for platform int width
func ParseNonNegativeDecimalInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}

	maxInt := int(^uint(0) >> 1)
	value := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch < '0' || ch > '9' {
			return 0, false
		}
		digit := int(ch - '0')
		if value > (maxInt-digit)/10 {
			return 0, false
		}
		value = value*10 + digit
	}
	return value, true
}

// Number is a Lua number: an integer, or a float when IsFloat is set.
type Number struct {
	Int     int64
	Float   float64
	IsFloat bool
}

// Parse converts s to a Lua number as lua_stringtonumber does in Lua 5.4.
// Lexer numerals, tonumber and string arithmetic coercion share this
// conversion.
//
// Leading and trailing whitespace is ignored and an optional sign is
// accepted. A decimal numeral without '.' or exponent is an integer unless
// it overflows, in which case it is a float. A hexadecimal numeral without
// '.' or 'p' exponent is an integer that wraps around modulo 2^64.
// Floats use C strtod syntax: decimal with an optional 'e' exponent, or
// hexadecimal with an optional fraction and 'p' exponent. Out-of-range
// floats become infinities. "inf", "nan" and their variants are rejected.
func Parse(s string) (Number, bool) {
	if i, ok := parseInt(s); ok {
		return Number{Int: i}, true
	}
	if f, ok := parseFloat(s); ok {
		return Number{Float: f, IsFloat: true}, true
	}
	return Number{}, false
}

// ParseBase converts s, a numeral in the given base (2 to 36), as
// tonumber(s, base) does in Lua 5.4. Digits above 9 are letters in either
// case. Surrounding whitespace and an optional sign are accepted, and the
// value wraps around modulo 2^64.
func ParseBase(s string, base int) (int64, bool) {
	s = trimSpace(s)
	neg := false
	s, neg = trimSign(s)
	if s == "" {
		return 0, false
	}
	var n uint64
	for i := 0; i < len(s); i++ {
		d := digitValue(s[i])
		if d >= base {
			return 0, false
		}
		n = n*uint64(base) + uint64(d)
	}
	if neg {
		n = -n
	}
	return int64(n), true
}

// parseInt is l_str2int from Lua 5.4 lobject.c.
func parseInt(s string) (int64, bool) {
	s = trimSpace(s)
	s, neg := trimSign(s)
	if s == "" {
		return 0, false
	}
	var a uint64
	if hasHexPrefix(s) {
		digits := s[2:]
		if digits == "" {
			return 0, false
		}
		for i := 0; i < len(digits); i++ {
			d := digitValue(digits[i])
			if d >= 16 {
				return 0, false
			}
			a = a*16 + uint64(d)
		}
	} else {
		const maxBy10 = math.MaxInt64 / 10
		const maxLastDigit = math.MaxInt64 % 10
		for i := 0; i < len(s); i++ {
			d := digitValue(s[i])
			if d >= 10 {
				return 0, false
			}
			if a >= maxBy10 && (a > maxBy10 || uint64(d) > maxLastDigit+boolToUint(neg)) {
				return 0, false
			}
			a = a*10 + uint64(d)
		}
	}
	if neg {
		a = -a
	}
	return int64(a), true
}

// parseFloat is l_str2d from Lua 5.4 lobject.c with strtod numeral syntax.
func parseFloat(s string) (float64, bool) {
	s = trimSpace(s)
	body, _ := trimSign(s)
	hex := hasHexPrefix(body)
	mantissa := body
	if hex {
		mantissa = body[2:]
	}
	base, expMark := 10, byte('e')
	if hex {
		base, expMark = 16, 'p'
	}
	n := scanDigits(mantissa, base)
	digits := n
	if n < len(mantissa) && mantissa[n] == '.' {
		frac := scanDigits(mantissa[n+1:], base)
		digits += frac
		n += 1 + frac
	}
	if digits == 0 {
		return 0, false
	}
	hasExp := false
	if n < len(mantissa) && lower(mantissa[n]) == expMark {
		e := n + 1
		if e < len(mantissa) && (mantissa[e] == '+' || mantissa[e] == '-') {
			e++
		}
		expDigits := scanDigits(mantissa[e:], 10)
		if expDigits == 0 {
			return 0, false
		}
		n = e + expDigits
		hasExp = true
	}
	if n != len(mantissa) {
		return 0, false
	}
	if hex && !hasExp {
		// strconv requires a binary exponent on hexadecimal floats.
		s += "p0"
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0, false
	}
	return f, true
}

// trimSpace removes the characters C isspace accepts.
func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && isSpace(s[start]) {
		start++
	}
	for end > start && isSpace(s[end-1]) {
		end--
	}
	return s[start:end]
}

func isSpace(c byte) bool {
	return c == ' ' || (c >= '\t' && c <= '\r')
}

func trimSign(s string) (string, bool) {
	if s != "" {
		switch s[0] {
		case '-':
			return s[1:], true
		case '+':
			return s[1:], false
		}
	}
	return s, false
}

func hasHexPrefix(s string) bool {
	return len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X')
}

// scanDigits returns the length of the run of base digits at the start of s.
func scanDigits(s string, base int) int {
	n := 0
	for n < len(s) && digitValue(s[n]) < base {
		n++
	}
	return n
}

// digitValue returns the value of an alphanumeric digit, or 36 for any other
// byte.
func digitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10
	}
	return 36
}

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

func boolToUint(b bool) uint64 {
	if b {
		return 1
	}
	return 0
}

// ParseIntegerLiteral parses a numeral whose Lua value is an integer.
func ParseIntegerLiteral(s string) (int64, bool) {
	n, ok := Parse(s)
	if !ok || n.IsFloat {
		return 0, false
	}
	return n.Int, true
}

// ParseNumberLiteral parses a numeral.
//
// Returns one of:
//   - (intValue, 0, true) for integer numerals
//   - (0, floatValue, true) for float numerals
//   - (_, _, false) when parsing fails
func ParseNumberLiteral(s string) (int64, float64, bool) {
	n, ok := Parse(s)
	if !ok {
		return 0, 0, false
	}
	return n.Int, n.Float, true
}

// ParseFloatLiteral parses a numeral as float64; integers convert to float64.
func ParseFloatLiteral(s string) (float64, bool) {
	n, ok := Parse(s)
	if !ok {
		return 0, false
	}
	if n.IsFloat {
		return n.Float, true
	}
	return float64(n.Int), true
}

// ParseIntegralLiteral parses a numeral as an integral int64 value.
//
// Unlike ParseIntegerLiteral, this accepts float numerals whose value is
// mathematically integral and in int64 range (for example "1.0", "1e0",
// "0x1p0").
func ParseIntegralLiteral(s string) (int64, bool) {
	n, ok := Parse(s)
	if !ok {
		return 0, false
	}
	if !n.IsFloat {
		return n.Int, true
	}
	f := n.Float
	if f >= -0x1p63 && f < 0x1p63 && math.Trunc(f) == f {
		return int64(f), true
	}
	return 0, false
}
