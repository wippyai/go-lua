package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/compiler/ast"
)

// contextualParameterArgumentOwnedByCallSite reports whether arg is a path rooted
// at an unannotated parameter in a caller-specialized function result. Those
// diagnostics are emitted at the outer call site with the projected obligation
// chain; annotated parameters remain body-owned implementation contracts.
func contextualParameterArgumentOwnedByCallSite(result *body.Result, context producerContext, arg ast.Expr) bool {
	if !context.callContextResult || result == nil || arg == nil {
		return false
	}
	argPath, ok := result.ExpressionPath(arg)
	if !ok || argPath.Symbol == 0 {
		return false
	}
	for _, slot := range result.FunctionParamSlots(result.Function()) {
		if slot.Symbol != argPath.Symbol {
			continue
		}
		_, annotated := result.SymbolTypeAnnotation(slot.Symbol)
		return !annotated
	}
	return false
}
