package bind

import "github.com/wippyai/go-lua/compiler/ast"

// AssertedParam returns the formal ordinal selected by a return-position
// assertion. The ordinal is relative to the immediate containing callable
// signature, including an implicit method self formal when present. Missing,
// unnamed, and outer-scope formals intentionally have no result.
func (r *Result) AssertedParam(expr *ast.AssertsTypeExpr) (int, bool) {
	if r == nil || expr == nil {
		return 0, false
	}
	ordinal, ok := r.assertedParams[expr]
	return ordinal, ok && ordinal >= 0
}
