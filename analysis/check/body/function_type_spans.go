package body

import (
	"github.com/wippyai/go-lua/compiler/ast"
)

// FunctionReturnTypeSpans returns source spans for a function expression's
// declared return types.
func FunctionReturnTypeSpans(fn *ast.FunctionExpr) []SourceSpan {
	if fn == nil || len(fn.ReturnTypes) == 0 {
		return nil
	}
	out := make([]SourceSpan, len(fn.ReturnTypes))
	for i, ret := range fn.ReturnTypes {
		out[i] = sourceSpanFromAST(ast.SpanOf(ret))
	}
	return out
}
