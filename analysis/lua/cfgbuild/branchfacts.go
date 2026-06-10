package cfgbuild

import (
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (b *builder) branchMetadata(expr ast.Expr) cfgfacts.BranchFact {
	switch expr := expr.(type) {
	case *ast.IdentExpr:
		if id, ok := b.identSymbol(expr); ok {
			return cfgfacts.BranchFact{Symbol: id, Check: cfgfacts.BranchCheck{Kind: cfgfacts.CheckTruthy}}
		}
	case *ast.UnaryNotOpExpr:
		if ident, ok := expr.Expr.(*ast.IdentExpr); ok {
			if id, ok := b.identSymbol(ident); ok {
				return cfgfacts.BranchFact{Symbol: id, Check: cfgfacts.BranchCheck{Kind: cfgfacts.CheckFalsy}}
			}
		}
	case *ast.RelationalOpExpr:
		if expr.Operator != "==" && expr.Operator != "~=" {
			break
		}
		if fact, ok := b.typeCompareBranchFact(expr); ok {
			return fact
		}
		if id, ok := b.nilCompareSymbol(expr.Lhs, expr.Rhs); ok {
			if expr.Operator == "==" {
				return cfgfacts.BranchFact{Symbol: id, Check: cfgfacts.BranchCheck{Kind: cfgfacts.CheckNil}}
			}
			return cfgfacts.BranchFact{Symbol: id, Check: cfgfacts.BranchCheck{Kind: cfgfacts.CheckNotNil}}
		}
	}
	return cfgfacts.BranchFact{Check: cfgfacts.BranchCheck{Kind: cfgfacts.CheckNone}}
}

func (b *builder) typeCompareBranchFact(expr ast.Expr) (cfgfacts.BranchFact, bool) {
	rel, ok := expr.(*ast.RelationalOpExpr)
	if !ok || (rel.Operator != "==" && rel.Operator != "~=") {
		return cfgfacts.BranchFact{}, false
	}
	id, typeName, ok := b.typeCompareOperands(rel.Lhs, rel.Rhs)
	if !ok {
		id, typeName, ok = b.typeCompareOperands(rel.Rhs, rel.Lhs)
	}
	if !ok {
		return cfgfacts.BranchFact{}, false
	}
	kind := cfgfacts.CheckTypeEqual
	if rel.Operator == "~=" {
		kind = cfgfacts.CheckTypeNot
	}
	return cfgfacts.BranchFact{
		Symbol: id,
		Check:  cfgfacts.BranchCheck{Kind: kind, TypeName: typeName},
	}, true
}

func (b *builder) typeCompareOperands(callExpr, literalExpr ast.Expr) (symbol.ID, string, bool) {
	lit, ok := literalExpr.(*ast.StringExpr)
	if !ok {
		return 0, "", false
	}
	call, ok := callExpr.(*ast.FuncCallExpr)
	if !ok {
		return 0, "", false
	}
	id, ok := b.typeCallSubjectSymbol(call)
	if !ok {
		return 0, "", false
	}
	return id, lit.Value, true
}

func (b *builder) typeCallSubjectSymbol(call *ast.FuncCallExpr) (symbol.ID, bool) {
	if call == nil || call.Receiver != nil || call.Method != "" || len(call.Args) != 1 || len(call.TypeArgs) != 0 {
		return 0, false
	}
	fn, ok := call.Func.(*ast.IdentExpr)
	if !ok || fn.Value != "type" || !b.isGlobalTypeIdent(fn) {
		return 0, false
	}
	arg, ok := call.Args[0].(*ast.IdentExpr)
	if !ok {
		return 0, false
	}
	return b.identSymbol(arg)
}

func (b *builder) isGlobalTypeIdent(ident *ast.IdentExpr) bool {
	id, ok := b.identSymbol(ident)
	if !ok || b.bindings.Name(id) != "type" {
		return false
	}
	kind, ok := b.bindings.Kind(id)
	return ok && kind == symbol.Global
}

func (b *builder) nilCompareSymbol(lhs, rhs ast.Expr) (symbol.ID, bool) {
	if _, ok := lhs.(*ast.NilExpr); ok {
		if ident, ok := rhs.(*ast.IdentExpr); ok {
			return b.identSymbol(ident)
		}
	}
	if _, ok := rhs.(*ast.NilExpr); ok {
		if ident, ok := lhs.(*ast.IdentExpr); ok {
			return b.identSymbol(ident)
		}
	}
	return 0, false
}
