package ops

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/types/typ"
)

// ParseNumber parses a Lua numeric literal and returns its type.
//
// Distinguishes between integer and float literals to enable integer-specific
// operations (bitwise operators, integer division).
//
// Recognized formats:
//   - Hex: 0x1A, 0XFF -> LiteralInt
//   - Integer: 42, -7 -> LiteralInt
//   - Float: 3.14, 1e10, 2.5e-3 -> LiteralNumber
//
// Returns typ.Number if parsing fails.
func ParseNumber(value string) typ.Type {
	// Try hex first
	if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") {
		if i, err := strconv.ParseInt(value, 0, 64); err == nil {
			return typ.LiteralInt(i)
		}

		return typ.Number
	}

	// Try float
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return typ.Number
	}

	// Check if it's a whole number (integer literal)
	if f == float64(int64(f)) && !strings.Contains(value, ".") && !strings.ContainsAny(value, "eE") {
		return typ.LiteralInt(int64(f))
	}

	return typ.LiteralNumber(f)
}

// IsIntegerLiteral checks if the string represents an integer literal.
func IsIntegerLiteral(value string) bool {
	// Hex literals are integers
	if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") {
		_, err := strconv.ParseInt(value, 0, 64)
		return err == nil
	}

	// Check for float indicators
	if strings.Contains(value, ".") || strings.ContainsAny(value, "eE") {
		return false
	}

	// Try parsing as integer
	_, err := strconv.ParseInt(value, 10, 64)

	return err == nil
}

// ParseNumberValue extracts the numeric value from a literal string.
// Returns the value and true for integers, or the value and false for floats.
func ParseNumberValue(value string) (float64, bool) {
	// Try hex first
	if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") {
		if i, err := strconv.ParseInt(value, 0, 64); err == nil {
			return float64(i), true
		}
	}

	// Try float
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}

	// Check if it's a whole number
	isInt := f == float64(int64(f)) && !strings.Contains(value, ".") && !strings.ContainsAny(value, "eE")

	return f, isInt
}
