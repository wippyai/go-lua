package cond

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// enumFlagWitness records a boolean that can become true only after a value
// matches an element of a fixed literal array. The pattern is deliberately
// exact: any extra write or branch leaves the ordinary, wider flow type.
type enumFlagWitness struct {
	value  *ast.IdentExpr
	typeOf typ.Type
}

func literalArrayEnumFlags(body []ast.Stmt, bindings *bind.BindingTable) map[ast.Expr]enumFlagWitness {
	result := make(map[ast.Expr]enumFlagWitness)
	for i := 0; i+3 < len(body); i++ {
		arrayDecl, ok := body[i].(*ast.LocalAssignStmt)
		if !ok || len(arrayDecl.Names) != 1 || len(arrayDecl.Exprs) != 1 {
			continue
		}
		array, ok := arrayDecl.Exprs[0].(*ast.TableExpr)
		if !ok || len(array.Fields) == 0 {
			continue
		}
		members := make([]typ.Type, 0, len(array.Fields))
		for _, field := range array.Fields {
			literal, ok := field.Value.(*ast.StringExpr)
			if !ok || field.Key != nil {
				members = nil
				break
			}
			members = append(members, typ.LiteralString(literal.Value))
		}
		if len(members) == 0 {
			continue
		}
		flagDecl, ok := body[i+1].(*ast.LocalAssignStmt)
		if !ok || len(flagDecl.Names) != 1 || len(flagDecl.Exprs) != 1 {
			continue
		}
		if _, ok := flagDecl.Exprs[0].(*ast.FalseExpr); !ok {
			continue
		}
		loop, ok := body[i+2].(*ast.GenericForStmt)
		if !ok || len(loop.Names) != 2 || len(loop.Exprs) != 1 || len(loop.Stmts) != 1 {
			continue
		}
		iter, ok := loop.Exprs[0].(*ast.FuncCallExpr)
		if !ok || len(iter.Args) != 1 || iter.Receiver != nil {
			continue
		}
		iterFn, ok := iter.Func.(*ast.IdentExpr)
		if !ok || iterFn.Value != "ipairs" || bindings == nil {
			continue
		}
		iteratorSymbol, found := bindings.SymbolOf(iterFn)
		iteratorKind, known := bindings.Kind(iteratorSymbol)
		if !found || !known || iteratorKind != cfg.SymbolGlobal {
			continue
		}
		iterArray, ok := iter.Args[0].(*ast.IdentExpr)
		if !ok || iterArray.Value != arrayDecl.Names[0] {
			continue
		}
		witnessIf, ok := loop.Stmts[0].(*ast.IfStmt)
		if !ok || len(witnessIf.Else) != 0 || len(witnessIf.Then) != 2 {
			continue
		}
		comparison, ok := witnessIf.Condition.(*ast.RelationalOpExpr)
		if !ok || comparison.Operator != "==" {
			continue
		}
		value, iterator := comparedValueAndIterator(comparison, loop.Names[1])
		if value == nil || iterator == nil || value.Value == flagDecl.Names[0] || value.Value == arrayDecl.Names[0] {
			continue
		}
		setFlag, ok := witnessIf.Then[0].(*ast.AssignStmt)
		if !ok || len(setFlag.Lhs) != 1 || len(setFlag.Rhs) != 1 {
			continue
		}
		flag, ok := setFlag.Lhs[0].(*ast.IdentExpr)
		if !ok || flag.Value != flagDecl.Names[0] {
			continue
		}
		if _, ok := setFlag.Rhs[0].(*ast.TrueExpr); !ok {
			continue
		}
		if _, ok := witnessIf.Then[1].(*ast.BreakStmt); !ok {
			continue
		}
		guard, ok := body[i+3].(*ast.IfStmt)
		if !ok {
			continue
		}
		negated, ok := guard.Condition.(*ast.UnaryNotOpExpr)
		if !ok {
			continue
		}
		guardFlag, ok := negated.Expr.(*ast.IdentExpr)
		if !ok || guardFlag.Value != flagDecl.Names[0] {
			continue
		}
		result[guard.Condition] = enumFlagWitness{value: value, typeOf: typ.NewUnion(members...)}
	}
	return result
}

func comparedValueAndIterator(expr *ast.RelationalOpExpr, iteratorName string) (*ast.IdentExpr, *ast.IdentExpr) {
	left, leftOK := expr.Lhs.(*ast.IdentExpr)
	right, rightOK := expr.Rhs.(*ast.IdentExpr)
	if !leftOK || !rightOK || left.Value == right.Value {
		return nil, nil
	}
	if right.Value == iteratorName {
		return left, right
	}
	if left.Value == iteratorName {
		return right, left
	}
	return nil, nil
}
