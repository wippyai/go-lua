package subtype

import "github.com/wippyai/go-lua/analysis/type/typ"

// stopDepthPair reports whether check/canWidenTo must stop without a proof:
// either side is missing, or the recursion has exhausted its depth budget.
// check() and canWidenTo() are positive relations (subtype, assignability):
// per invariants.md Rule 1, an exhausted budget must fail closed, so callers
// treat a true result here as "return false", never "return true".
func stopDepthPair(sub, super typ.Type, depth int) bool {
	return sub == nil || super == nil || depth > typ.DefaultRecursionDepth
}
