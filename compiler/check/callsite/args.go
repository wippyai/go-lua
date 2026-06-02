package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// PositionalArgAt returns the positional argument at idx.
// Negative indices address from the end (-1 is last).
func PositionalArgAt(args []ast.Expr, idx int) ast.Expr {
	if len(args) == 0 {
		return nil
	}
	if idx < 0 {
		idx = len(args) + idx
	}
	if idx < 0 || idx >= len(args) {
		return nil
	}
	return args[idx]
}

// RuntimeArgCount returns call runtime arity, including receiver for method calls.
func RuntimeArgCount(info *cfg.CallInfo) int {
	if info == nil {
		return 0
	}
	return runtimeArgCount(info.Method != "", info.Args)
}

// RuntimeArgAt returns the runtime argument at parameter index paramIdx.
//
// For method calls, index 0 is the receiver (self), and remaining indices map
// to positional call args. Negative indices address from the runtime argument
// tail, including the receiver slot.
func RuntimeArgAt(info *cfg.CallInfo, paramIdx int) ast.Expr {
	if info == nil {
		return nil
	}
	return runtimeArgAt(info.Method != "", info.Receiver, info.Args, paramIdx)
}

// RuntimeArgExprCount returns call runtime arity directly from the AST call,
// including the receiver for method calls.
func RuntimeArgExprCount(call *ast.FuncCallExpr) int {
	if call == nil {
		return 0
	}
	return runtimeArgCount(call.Method != "", call.Args)
}

// RuntimeArgExprAt returns the runtime argument expression directly from the AST
// call. For method calls, index 0 is the receiver and listed args start at 1.
func RuntimeArgExprAt(call *ast.FuncCallExpr, paramIdx int) ast.Expr {
	if call == nil {
		return nil
	}
	return runtimeArgAt(call.Method != "", call.Receiver, call.Args, paramIdx)
}

func runtimeArgCount(method bool, args []ast.Expr) int {
	if method {
		return len(args) + 1
	}
	return len(args)
}

func runtimeArgAt(method bool, receiver ast.Expr, args []ast.Expr, paramIdx int) ast.Expr {
	if method {
		if paramIdx == 0 {
			return receiver
		}
		if paramIdx < 0 {
			adj := runtimeArgCount(method, args) + paramIdx
			if adj == 0 {
				return receiver
			}
			return PositionalArgAt(args, adj-1)
		}
		return PositionalArgAt(args, paramIdx-1)
	}
	return PositionalArgAt(args, paramIdx)
}
