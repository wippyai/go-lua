package core

import (
	"github.com/wippyai/go-lua/compiler/ast"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ExpectedFunctionLiteralSignature returns the contextual function signature
// for a function literal, including arity-compatible function members inside
// an expected union.
func ExpectedFunctionLiteralSignature(fn *ast.FunctionExpr, expected typ.Type) *typ.Function {
	if expected == nil {
		return nil
	}
	if expectedFn, ok := unwrap.Alias(expected).(*typ.Function); ok {
		return expectedFn
	}
	return querycore.CompatibleFunctionFromUnion(functionLiteralParamCount(fn), expected)
}

func functionLiteralParamCount(fn *ast.FunctionExpr) int {
	if fn == nil || fn.ParList == nil {
		return 0
	}
	return len(fn.ParList.Names)
}

// ShallowFunctionLiteralSignature is the non-recursive probe type for a
// function literal before a call site provides contextual parameter types. It
// deliberately omits returns: a shallow body has not proved an output value, so
// it must not bind a higher-order callee's output type parameter to any.
func ShallowFunctionLiteralSignature(fn *ast.FunctionExpr) *typ.Function {
	builder := typ.Func()
	if fn != nil && fn.ParList != nil {
		builder.ReserveParams(len(fn.ParList.Names))
		for _, name := range fn.ParList.Names {
			builder.OptParam(name, typ.Any)
		}
		if fn.ParList.HasVargs {
			builder.Variadic(typ.Any)
		}
	} else {
		builder.Variadic(typ.Any)
	}
	return builder.Build()
}
