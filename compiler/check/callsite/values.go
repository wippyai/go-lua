package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Arguments is the runtime value sequence of a Lua call. OpenTail means the
// last expression can yield further values whose count and types are unknown.
// Written counts the positions produced by the written expressions: values past
// it come from expanding the final expression, and Lua drops those a callee
// does not declare.
type Arguments struct {
	Types    []typ.Type
	OpenTail bool
	Written  int
}

// ArgumentTypes applies Lua's expression-list adjustment at a call site. Only
// the final expression can contribute more than one value; parentheses around
// a call make it a single-valued expression, as reflected by multi.
func ArgumentTypes(args []ast.Expr, single func(ast.Expr) typ.Type, multi func(ast.Expr) []typ.Type) Arguments {
	if len(args) == 0 {
		return Arguments{}
	}
	result := Arguments{Types: make([]typ.Type, 0, len(args))}
	for _, arg := range args[:len(args)-1] {
		result.Types = append(result.Types, single(arg))
	}
	last := args[len(args)-1]
	switch last.(type) {
	case *ast.FuncCallExpr, *ast.Comma3Expr:
		values := multi(last)
		result.Types = append(result.Types, values...)
		result.Written = len(args) - 1 + min(1, len(values))
		adjusted := false
		switch ex := last.(type) {
		case *ast.FuncCallExpr:
			adjusted = ex.AdjustRet
		case *ast.Comma3Expr:
			adjusted = ex.AdjustRet
		}
		_, vararg := last.(*ast.Comma3Expr)
		call, isCall := last.(*ast.FuncCallExpr)
		result.OpenTail = !adjusted && (vararg || isCall && unknownCallArity(call, single))
	default:
		result.Types = append(result.Types, single(last))
		result.Written = len(result.Types)
	}
	return result
}

// Return types alone do not determine whether a call has an open tail: a
// declared () -> unknown function still produces exactly one value.
func unknownCallArity(call *ast.FuncCallExpr, single func(ast.Expr) typ.Type) bool {
	var callee typ.Type
	if call.Receiver != nil {
		receiver := unwrap.Alias(single(call.Receiver))
		if typ.IsAny(receiver) || typ.IsUnknown(receiver) {
			return true
		}
		callee, _ = core.Method(receiver, call.Method)
	} else if call.Func != nil {
		callee = single(call.Func)
	}
	callee = unwrap.Alias(typ.UnwrapAnnotated(callee))
	return typ.IsAny(callee) || typ.IsUnknown(callee)
}
