package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func functionLiteralArgumentContextuallyChecked(result *body.Result, resolver typeannotation.Resolver, arg ast.Expr, got, want typ.Type) bool {
	if arg == nil || got == nil || want == nil {
		return false
	}
	fn, ok := unwrapFunctionLiteralArgument(arg)
	if !ok {
		return false
	}
	if declared, ok := declaredArgumentExprType(result, resolver, arg); ok && !topLikeFunctionPlaceholder(declared) {
		return functionLiteralTypeAdmitsContext(declared, want)
	}
	if functionLiteralHasExplicitParamTypes(fn) {
		return !topLikeFunctionPlaceholder(got) && functionLiteralTypeAdmitsContext(got, want)
	}
	return topLikeFunctionPlaceholder(got) && functionLiteralTypeAdmitsContext(got, want)
}

func functionLiteralTypeAdmitsContext(got, want typ.Type) bool {
	gotFn, ok := typecall.ContextualCallable(got)
	if !ok || gotFn == nil {
		return false
	}
	wantFn, ok := typecall.ContextualCallable(want)
	if !ok || wantFn == nil {
		return false
	}
	return placeholderFunctionLiteralTypeAdmitsContext(gotFn, wantFn)
}

func topLikeFunctionPlaceholder(t typ.Type) bool {
	fn, ok := typecall.ContextualCallable(t)
	if !ok || fn == nil {
		return false
	}
	if fn.Variadic != nil && !topLikeType(fn.Variadic) {
		return false
	}
	for _, param := range fn.Params {
		if !topLikeType(param.Type) {
			return false
		}
	}
	return len(fn.Returns) == 0
}

func placeholderFunctionLiteralTypeAdmitsContext(got, want *typ.Function) bool {
	if got == nil || want == nil || len(got.TypeParams) != len(want.TypeParams) || want.Variadic != nil {
		return false
	}
	if got.Variadic != nil {
		return topLikeType(got.Variadic)
	}
	if len(got.Params) != len(want.Params) {
		return false
	}
	for i, gotParam := range got.Params {
		wantParam := want.Params[i]
		if gotParam.Optional != wantParam.Optional {
			return false
		}
		if topLikeType(gotParam.Type) {
			continue
		}
		if wantParam.Type == nil || !subtype.IsSubtype(wantParam.Type, gotParam.Type) {
			return false
		}
	}
	if len(got.Returns) == 0 {
		return true
	}
	if len(got.Returns) != len(want.Returns) {
		return false
	}
	for i, gotReturn := range got.Returns {
		if gotReturn == nil || !subtype.IsSubtype(gotReturn, want.Returns[i]) {
			return false
		}
	}
	return true
}

func functionLiteralHasExplicitParamTypes(fn *ast.FunctionExpr) bool {
	if fn == nil || fn.ParList == nil {
		return false
	}
	for _, expr := range fn.ParList.Types {
		if expr != nil {
			return true
		}
	}
	return fn.ParList.VarargType != nil
}

func unwrapFunctionLiteralArgument(arg ast.Expr) (*ast.FunctionExpr, bool) {
	switch a := arg.(type) {
	case *ast.FunctionExpr:
		return a, true
	case *ast.CastExpr:
		return unwrapFunctionLiteralArgument(a.Expr)
	case *ast.NonNilAssertExpr:
		return unwrapFunctionLiteralArgument(a.Expr)
	default:
		return nil, false
	}
}
