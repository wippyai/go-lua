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

// RetPath returns the path for a return value.
func RetPath(index int) Path {
	return Path{Root: fmt.Sprintf("ret[%d]", index)}
}

// IsReturnPath checks if the path represents a return value (ret[N] format).
func IsReturnPath(p Path) bool {
	return strings.HasPrefix(p.Root, "ret[") && strings.HasSuffix(p.Root, "]")
}
