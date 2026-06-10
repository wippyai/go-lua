package semantics

import (
	"slices"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/compiler/ast"
)

type BranchConditionCheckKind uint8

const (
	BranchConditionCheckNone BranchConditionCheckKind = iota
	BranchConditionCheckTruthy
	BranchConditionCheckFalsy
	BranchConditionCheckNil
	BranchConditionCheckNotNil
	BranchConditionCheckTypeEqual
	BranchConditionCheckTypeNot
)

type BranchConditionCheck struct {
	Kind     BranchConditionCheckKind
	Path     path.Path
	TypeName string
}

func normalizeBranchCondition(expr ast.Expr, bindings *bind.Result) BranchConditionCheck {
	if p, ok := pathexpr.Resolve(expr, bindings); ok {
		return BranchConditionCheck{Kind: BranchConditionCheckTruthy, Path: p}
	}

	switch expr := expr.(type) {
	case *ast.UnaryNotOpExpr:
		if p, ok := pathexpr.Resolve(expr.Expr, bindings); ok {
			return BranchConditionCheck{Kind: BranchConditionCheckFalsy, Path: p}
		}
	case *ast.RelationalOpExpr:
		if expr.Operator != "==" && expr.Operator != "~=" {
			return BranchConditionCheck{}
		}
		if check, ok := normalizeTypeComparison(expr, bindings); ok {
			return check
		}
		if p, ok := nilComparisonPath(expr.Lhs, expr.Rhs, bindings); ok {
			kind := BranchConditionCheckNil
			if expr.Operator == "~=" {
				kind = BranchConditionCheckNotNil
			}
			return BranchConditionCheck{Kind: kind, Path: p}
		}
	}

	return BranchConditionCheck{}
}

func normalizeTypeComparison(expr *ast.RelationalOpExpr, bindings *bind.Result) (BranchConditionCheck, bool) {
	p, typeName, ok := typeComparisonOperands(expr.Lhs, expr.Rhs, bindings)
	if !ok {
		p, typeName, ok = typeComparisonOperands(expr.Rhs, expr.Lhs, bindings)
	}
	if !ok {
		return BranchConditionCheck{}, false
	}
	kind := BranchConditionCheckTypeEqual
	if expr.Operator == "~=" {
		kind = BranchConditionCheckTypeNot
	}
	return BranchConditionCheck{Kind: kind, Path: p, TypeName: typeName}, true
}

func typeComparisonOperands(callExpr, literalExpr ast.Expr, bindings *bind.Result) (path.Path, string, bool) {
	lit, ok := literalExpr.(*ast.StringExpr)
	if !ok {
		return path.Path{}, "", false
	}
	call, ok := callExpr.(*ast.FuncCallExpr)
	if !ok {
		return path.Path{}, "", false
	}
	p, ok := typeCallSubjectPath(call, bindings)
	if !ok {
		return path.Path{}, "", false
	}
	return p, lit.Value, true
}

func typeCallSubjectPath(call *ast.FuncCallExpr, bindings *bind.Result) (path.Path, bool) {
	if call == nil || call.Receiver != nil || call.Method != "" || len(call.Args) != 1 || len(call.TypeArgs) != 0 {
		return path.Path{}, false
	}
	fn, ok := call.Func.(*ast.IdentExpr)
	if !ok || !bindings.ResolvesToGlobal(fn, "type") {
		return path.Path{}, false
	}
	return pathexpr.Resolve(call.Args[0], bindings)
}

func nilComparisonPath(lhs, rhs ast.Expr, bindings *bind.Result) (path.Path, bool) {
	if _, ok := lhs.(*ast.NilExpr); ok {
		return pathexpr.Resolve(rhs, bindings)
	}
	if _, ok := rhs.(*ast.NilExpr); ok {
		return pathexpr.Resolve(lhs, bindings)
	}
	return path.Path{}, false
}

func copyBranchConditionFact(fact BranchConditionFact) BranchConditionFact {
	fact.Check = copyBranchConditionCheck(fact.Check)
	return fact
}

func copyBranchConditionCheck(check BranchConditionCheck) BranchConditionCheck {
	check.Path = copyPath(check.Path)
	return check
}

func copyPath(p path.Path) path.Path {
	p.Segments = slices.Clone(p.Segments)
	return p
}
