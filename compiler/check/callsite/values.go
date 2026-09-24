package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/typ"
)

// ArgumentTypes applies Lua's expression-list adjustment at a call site. Only
// the final expression can contribute more than one value; parentheses around
// a call make it a single-valued expression, as reflected by multi.
func ArgumentTypes(args []ast.Expr, single func(ast.Expr) typ.Type, multi func(ast.Expr) []typ.Type) []typ.Type {
	if len(args) == 0 {
		return nil
	}
	result := make([]typ.Type, 0, len(args))
	for _, arg := range args[:len(args)-1] {
		result = append(result, single(arg))
	}
	last := args[len(args)-1]
	switch last.(type) {
	case *ast.FuncCallExpr, *ast.Comma3Expr:
		result = append(result, multi(last)...)
	default:
		result = append(result, single(last))
	}
	return result
}
