package cfgbuild

import (
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (b *builder) typeCompareConditionSupported(expr ast.Expr) bool {
	rel, ok := expr.(*ast.RelationalOpExpr)
	if !ok || (rel.Operator != "==" && rel.Operator != "~=") {
		return false
	}
	return b.typeCompareOperandsSupported(rel.Lhs, rel.Rhs) || b.typeCompareOperandsSupported(rel.Rhs, rel.Lhs)
}

func (b *builder) typeCompareOperandsSupported(callExpr, literalExpr ast.Expr) bool {
	if _, ok := literalExpr.(*ast.StringExpr); !ok {
		return false
	}
	call, ok := callExpr.(*ast.FuncCallExpr)
	if !ok {
		return false
	}
	return b.typeCallSubjectPathSupported(call)
}

func (b *builder) typeCallSubjectPathSupported(call *ast.FuncCallExpr) bool {
	arg, ok := b.typeCallSubjectExpr(call)
	if !ok {
		return false
	}
	_, ok = pathexpr.Resolve(arg, b.bindings)
	return ok
}

func (b *builder) typeCallSubjectExpr(call *ast.FuncCallExpr) (ast.Expr, bool) {
	if call == nil || call.Receiver != nil || call.Method != "" || len(call.Args) != 1 || len(call.TypeArgs) != 0 {
		return nil, false
	}
	fn, ok := call.Func.(*ast.IdentExpr)
	if !ok || !b.bindings.ResolvesToGlobal(fn, "type") {
		return nil, false
	}
	return call.Args[0], true
}
