package constraint

import (
	"fmt"
	"strings"
)

// ParamPath returns the path for a parameter value.
// Uses the $N format which is recognized by IsPlaceholder() and PlaceholderIndex().
func ParamPath(index int) Path {
	return Path{Root: fmt.Sprintf("$%d", index)}
}

// PlaceholderIndexFromString extracts a placeholder index from canonical forms.
//
// Accepted forms:
//   - "$N" (constraint path placeholder)
//   - "param[N]" (legacy predicate placeholder)
//
// Returns -1 for invalid syntax, negative values, and overflow.
func PlaceholderIndexFromString(s string) int {
	if len(s) >= 2 && s[0] == '$' {
		return parseNonNegativeDecimal(s[1:])
	}
	if strings.HasPrefix(s, "param[") && strings.HasSuffix(s, "]") {
		return parseNonNegativeDecimal(s[6 : len(s)-1])
	}
	return -1
}

func parseNonNegativeDecimal(s string) int {
	if s == "" {
		return -1
	}

	maxInt := int(^uint(0) >> 1)
	idx := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch < '0' || ch > '9' {
			return -1
		}
		digit := int(ch - '0')
		if idx > (maxInt-digit)/10 {
			return -1
		}
		idx = idx*10 + digit
	}

	return idx
}

// RetPath returns the path for a return value.
func RetPath(index int) Path {
	return Path{Root: fmt.Sprintf("ret[%d]", index)}
}

// IsReturnPath checks if the path represents a return value (ret[N] format).
func IsReturnPath(p Path) bool {
	return strings.HasPrefix(p.Root, "ret[") && strings.HasSuffix(p.Root, "]")
}
