package body

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
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

// FallbackFunctionReturnTypeSpans returns local function return spans for a
// callsite when no precomputed function-return span fact exists.
func (r *Result) FallbackFunctionReturnTypeSpans(site factflow.CallSite) []SourceSpan {
	if r == nil {
		return nil
	}
	if fn, ok := r.FunctionBySymbol(site.CalleeSymbol()); ok && fn != nil {
		return FunctionReturnTypeSpans(fn)
	}
	if callee := site.CalleePathRef(); callee.Symbol != 0 {
		if fn, ok := r.FunctionBySymbol(callee.Symbol); ok && fn != nil {
			return FunctionReturnTypeSpans(fn)
		}
	}
	return nil
}

// FunctionReturnTypeSpansForFunction returns source spans for this result's
// function return annotation.
func (r *Result) FunctionReturnTypeSpansForFunction() []SourceSpan {
	if r == nil {
		return nil
	}
	return FunctionReturnTypeSpans(r.Function())
}
