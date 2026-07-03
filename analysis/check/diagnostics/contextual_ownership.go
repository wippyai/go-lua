package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/compiler/ast"
)

// contextualParameterArgumentOwnedByCallSite reports whether arg is a path rooted
// at a caller-specialized parameter. Those diagnostics are emitted at the outer
// call site with the projected obligation chain. Concrete annotated parameters
// remain body-owned implementation contracts; generic annotations whose shape
// still contains free type parameters are caller-context contracts.
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
		expr, annotated := result.SymbolTypeAnnotation(slot.Symbol)
		if !annotated {
			return true
		}
		t, ok := lowerType(expr, context.resolver)
		return ok && containsTypeParamSyntax(t)
	}
	return false
}
