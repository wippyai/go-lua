package body

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/expressionid"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ExpressionEvaluationFact is the body-owned source projection for a structural
// expression evaluation anchor recorded by WIR.
type ExpressionEvaluationFact struct {
	Point cfg.Point
	Expr  ast.Expr
	Span  SourceSpan
}

func (r *Result) sourceExpressionByID(id wir.ExpressionID) (ast.Expr, bool) {
	if r == nil || id == 0 {
		return nil, false
	}
	exprs := r.sourceExpressionsByID()
	expr, ok := exprs[id]
	return expr, ok
}

func (r *Result) sourceExpressionsByID() map[wir.ExpressionID]ast.Expr {
	if r == nil {
		return nil
	}
	if r.queries.expressionsByIDOK {
		return r.queries.expressionsByID
	}
	out := map[wir.ExpressionID]ast.Expr{}
	var walkStmts func([]ast.Stmt)
	var walkExpr func(ast.Expr)
	walkExprs := func(exprs []ast.Expr) {
		for _, expr := range exprs {
			walkExpr(expr)
		}
	}
	walkExpr = func(expr ast.Expr) {
		if expr == nil {
			return
		}
		if id := expressionid.Of(expr); id != 0 {
			out[id] = expr
		}
		switch expr := expr.(type) {
		case *ast.AttrGetExpr:
			walkExpr(expr.Object)
			walkExpr(expr.Key)
		case *ast.TableExpr:
			for _, field := range expr.Fields {
				if field == nil {
					continue
				}
				walkExpr(field.Key)
				walkExpr(field.Value)
			}
		case *ast.FuncCallExpr:
			walkExpr(expr.Func)
			walkExpr(expr.Receiver)
			walkExprs(expr.Args)
		case *ast.LogicalOpExpr:
			walkExpr(expr.Lhs)
			walkExpr(expr.Rhs)
		case *ast.RelationalOpExpr:
			walkExpr(expr.Lhs)
			walkExpr(expr.Rhs)
		case *ast.StringConcatOpExpr:
			walkExpr(expr.Lhs)
			walkExpr(expr.Rhs)
		case *ast.ArithmeticOpExpr:
			walkExpr(expr.Lhs)
			walkExpr(expr.Rhs)
		case *ast.UnaryMinusOpExpr:
			walkExpr(expr.Expr)
		case *ast.UnaryNotOpExpr:
			walkExpr(expr.Expr)
		case *ast.UnaryLenOpExpr:
			walkExpr(expr.Expr)
		case *ast.UnaryBNotOpExpr:
			walkExpr(expr.Expr)
		case *ast.CastExpr:
			walkExpr(expr.Expr)
		case *ast.NonNilAssertExpr:
			walkExpr(expr.Expr)
		case *ast.FunctionExpr:
			// The nested function body is lowered as its own body result. Keep the
			// function expression identity, but do not index child-body expressions.
		}
	}
	walkStmts = func(stmts []ast.Stmt) {
		for _, stmt := range stmts {
			switch stmt := stmt.(type) {
			case *ast.AssignStmt:
				walkExprs(stmt.Lhs)
				walkExprs(stmt.Rhs)
			case *ast.LocalAssignStmt:
				walkExprs(stmt.Exprs)
			case *ast.FuncCallStmt:
				walkExpr(stmt.Expr)
			case *ast.ReturnStmt:
				walkExprs(stmt.Exprs)
			case *ast.DoBlockStmt:
				walkStmts(stmt.Stmts)
			case *ast.IfStmt:
				walkExpr(stmt.Condition)
				walkStmts(stmt.Then)
				walkStmts(stmt.Else)
			case *ast.WhileStmt:
				walkExpr(stmt.Condition)
				walkStmts(stmt.Stmts)
			case *ast.RepeatStmt:
				walkStmts(stmt.Stmts)
				walkExpr(stmt.Condition)
			case *ast.NumberForStmt:
				walkExpr(stmt.Init)
				walkExpr(stmt.Limit)
				walkExpr(stmt.Step)
				walkStmts(stmt.Stmts)
			case *ast.GenericForStmt:
				walkExprs(stmt.Exprs)
				walkStmts(stmt.Stmts)
			case *ast.FuncDefStmt:
				if stmt.Name != nil {
					walkExpr(stmt.Name.Func)
					walkExpr(stmt.Name.Receiver)
				}
				walkExpr(stmt.Func)
			}
		}
	}
	walkStmts(r.sourceStmts)
	r.queries.expressionsByID = out
	r.queries.expressionsByIDOK = true
	return out
}
