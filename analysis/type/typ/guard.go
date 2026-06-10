package typ

import "github.com/wippyai/go-lua/analysis/internal/recursion"

// DefaultRecursionDepth is the canonical recursion depth for typ operations.
const DefaultRecursionDepth = 64

// DeepRecursionDepth is used for deep structural expansions.
const DeepRecursionDepth = 256

// NewGuard returns a recursion guard using the canonical default depth.
func NewGuard() recursion.Guard {
	return recursion.NewGuard(DefaultRecursionDepth)
}

// NewDeepGuard returns a recursion guard using the canonical deep depth.
func NewDeepGuard() recursion.Guard {
	return recursion.NewGuard(DeepRecursionDepth)
}

// GuardForDepth returns a recursion guard for a specific depth.
// If maxDepth is non-positive, the default depth is used.
func GuardForDepth(maxDepth int) recursion.Guard {
	if maxDepth <= 0 {
		maxDepth = DefaultRecursionDepth
	}
	return recursion.NewGuard(maxDepth)
}

// DepthExceeded reports whether depth exceeds the default recursion limit.
func DepthExceeded(depth int) bool {
	return depth > DefaultRecursionDepth
}
