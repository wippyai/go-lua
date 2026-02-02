// util.go provides parsing utilities for path key construction.
//
// These functions handle low-level parsing of identifiers and numbers
// in path syntax, enabling path key parsing and construction.
package pathkey

// ParseIntLiteral parses a string as a non-negative integer literal.
//
// Returns the integer value and true if the string contains only ASCII digits.
// Returns (0, false) for empty strings, negative numbers, or non-digit characters.
//
// This is used to distinguish integer indices ([0], [1]) from string indices
// ([key]) in path suffix parsing.
func ParseIntLiteral(s string) (int, bool) {
	if s == "" {
		return 0, false
	}

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch < '0' || ch > '9' {
			return 0, false
		}
	}

	value := 0
	for i := 0; i < len(s); i++ {
		value = value*10 + int(s[i]-'0')
	}

	return value, true
}

// IsIdentStart reports whether ch can start a Lua identifier.
//
// In Lua, identifiers must start with a letter (A-Z, a-z) or underscore (_).
// This matches Lua's lexical rules for variable names.
func IsIdentStart(ch byte) bool {
	return ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

// IsIdentPart reports whether ch can appear in a Lua identifier (after the first character).
//
// In Lua, identifiers can contain letters, digits (0-9), and underscores.
// The first character must satisfy IsIdentStart; subsequent characters satisfy IsIdentPart.
func IsIdentPart(ch byte) bool {
	return IsIdentStart(ch) || (ch >= '0' && ch <= '9')
}

// ReadIdent reads a Lua identifier from s starting at *idx, advancing *idx past it.
//
// An identifier starts with IsIdentStart and continues with IsIdentPart characters.
// The function modifies *idx in place to point past the consumed identifier.
//
// Returns the identifier string, or empty string if no valid identifier starts at *idx.
// If idx is nil or out of bounds, returns empty string without modifying idx.
func ReadIdent(s string, idx *int) string {
	if idx == nil || *idx >= len(s) {
		return ""
	}

	start := *idx

	ch := s[*idx]
	if !IsIdentStart(ch) {
		return ""
	}

	*idx++
	for *idx < len(s) {
		ch = s[*idx]
		if !IsIdentPart(ch) {
			break
		}
		*idx++
	}

	return s[start:*idx]
}

// IsIdentName reports whether s is a valid Lua identifier name.
//
// A valid identifier is non-empty, starts with IsIdentStart, and all subsequent
// characters satisfy IsIdentPart. This validates variable names, field names, etc.
func IsIdentName(s string) bool {
	if s == "" {
		return false
	}

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if i == 0 {
			if !IsIdentStart(ch) {
				return false
			}
		} else if !IsIdentPart(ch) {
			return false
		}
	}

	return true
}

// IntToString converts an integer to its string representation.
//
// This is a simple implementation without using strconv to avoid allocation
// in hot paths. Handles negative numbers by prepending "-" to the absolute value.
func IntToString(v int) string {
	if v == 0 {
		return "0"
	}

	if v < 0 {
		return "-" + IntToString(-v)
	}

	var buf [20]byte

	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}

	return string(buf[i:])
}

// MaxSafeFloat64Int is the largest integer magnitude that can be exactly represented in float64.
//
// IEEE 754 double precision has a 53-bit significand, so integers up to 2^53 - 1
// can be represented exactly. Beyond this, consecutive integers may round to the
// same float64 value, causing precision loss.
//
// This constant is used to validate integer conversions from Lua numbers, which
// are all represented as float64 internally.
const MaxSafeFloat64Int = (1 << 53) - 1

// FloatToSafeInt converts a float64 to int if it represents a whole number safely.
//
// Returns (value, true) if:
//   - The float has no fractional part (float64(int64(f)) == f)
//   - The magnitude is within MaxSafeFloat64Int
//
// Returns (0, false) if the conversion would lose precision or the value is
// not an exact integer. This is used for array index validation where only
// exact integers are valid indices.
func FloatToSafeInt(f float64) (int, bool) {
	i := int64(f)
	if float64(i) != f {
		return 0, false
	}

	if i > MaxSafeFloat64Int || i < -MaxSafeFloat64Int {
		return 0, false
	}

	return int(i), true
}
