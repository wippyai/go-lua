package functionfact

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
	"github.com/wippyai/go-lua/types/typ"
)

// PostflowReturnSummary canonicalizes the return summary stored in the
// FunctionFact product after abstract interpretation.
func PostflowReturnSummary(fn *ast.FunctionExpr, returns []typ.Type) []typ.Type {
	if len(returns) == 0 {
		return nil
	}
	if fn != nil && len(fn.ReturnTypes) > 0 {
		return returnsummary.Canonical(returns)
	}
	if returnsummary.AllNil(returns) {
		return nil
	}
	return returnsummary.Canonical(returns)
}
