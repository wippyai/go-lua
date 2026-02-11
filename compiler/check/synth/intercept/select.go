package intercept

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/numparse"
	"github.com/wippyai/go-lua/types/typ"
)

// VariadicTypeResolver provides access to the variadic (...) type in scope.
//
// Used by SelectIntercept to determine the type of select(n, ...) calls
// when selecting from the variadic arguments.
type VariadicTypeResolver interface {
	VariadicType() typ.Type
}

// SelectIntercept handles Lua's select() builtin for variadic manipulation.
//
// Patterns handled:
//   - select("#", ...) -> integer (count of variadic arguments)
//   - select(n, ...) -> nth variadic element type
//   - select(n, a, b, c) -> type of the nth argument after n
//
// Detection is effect-based: callee must have VariadicTransform effect.
//
// When selecting from varargs, uses VariadicResolver to get the variadic type.
// When selecting a concrete index from literal arguments, returns that argument's type.
type SelectIntercept struct {
	// VariadicResolver resolves the variadic type in current scope.
	// If nil, varargs pattern is not handled.
	VariadicResolver VariadicTypeResolver
}

func (s *SelectIntercept) InterceptCall(ex *ast.FuncCallExpr, ctx CallEnv) Result {
	if !isSelectCall(ex, ctx) {
		return Result{}
	}

	t := s.selectReturnType(ex, ctx)
	if t == nil {
		return Result{}
	}

	return Result{
		Types: []typ.Type{t},
		Skip:  true,
	}
}

func isSelectCall(ex *ast.FuncCallExpr, ctx CallEnv) bool {
	if ex == nil || ex.Func == nil {
		return false
	}
	return calleeHasEffect(ex, ctx, effect.Row.HasVariadicTransform)
}

func (s *SelectIntercept) selectReturnType(ex *ast.FuncCallExpr, ctx CallEnv) typ.Type {
	if ex == nil || len(ex.Args) == 0 {
		return nil
	}

	// select("#", ...) returns integer
	if str, ok := ex.Args[0].(*ast.StringExpr); ok && str.Value == "#" {
		return typ.Integer
	}

	if len(ex.Args) < 2 {
		return nil
	}

	// If selecting from varargs, use the variadic type.
	if _, ok := ex.Args[1].(*ast.Comma3Expr); ok {
		if s.VariadicResolver != nil {
			return s.VariadicResolver.VariadicType()
		}
		return nil
	}

	// If selecting a concrete index, use the type of that argument.
	if num, ok := ex.Args[0].(*ast.NumberExpr); ok {
		idx, ok := numparse.ParseIntegerLiteral(num.Value)
		if !ok || idx <= 0 {
			return nil
		}
		i := int(idx) - 1
		if i >= 0 && i < len(ex.Args)-1 && ctx.Recurse != nil {
			return ctx.Recurse(ex.Args[i+1])
		}
	}

	return nil
}
