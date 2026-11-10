package ast

import "fmt"

// SourceKey identifies a function within its source chunk, including functions
// on the same line. It is used only to connect analysis metadata to compilation.
func (fn *FunctionExpr) SourceKey() string {
	return fmt.Sprintf("%d:%d", fn.Line(), fn.Column())
}

// HasTypeSyntax reports whether a chunk needs declaration resolution even when
// static diagnostics are disabled. It does not infer types from Lua operations.
func HasTypeSyntax(chunk []Stmt) bool {
	var found bool
	var expr func(Expr)
	var stmts func([]Stmt)
	expr = func(e Expr) {
		if found || e == nil {
			return
		}
		switch e := e.(type) {
		case *FunctionExpr:
			if len(e.TypeParams) > 0 || len(e.ReturnTypes) > 0 || (e.ParList != nil && e.ParList.VarargType != nil) {
				found = true
				return
			}
			if e.ParList != nil {
				for _, t := range e.ParList.Types {
					if t != nil {
						found = true
						return
					}
				}
			}
			stmts(e.Stmts)
		case *CastExpr:
			found = true
		default:
			WalkExprChildren(e, func(child Expr, _ int) { expr(child) })
		}
	}
	stmts = func(list []Stmt) {
		for _, stmt := range list {
			if found {
				return
			}
			switch s := stmt.(type) {
			case *TypeDefStmt, *InterfaceDefStmt:
				found = true
			case *FuncDefStmt:
				expr(s.Func)
			case *AssignStmt:
				for _, e := range s.Lhs {
					expr(e)
				}
				for _, e := range s.Rhs {
					expr(e)
				}
			case *LocalAssignStmt:
				for _, t := range s.Types {
					if t != nil {
						found = true
					}
				}
				for _, e := range s.Exprs {
					expr(e)
				}
			case *FuncCallStmt:
				expr(s.Expr)
			case *ReturnStmt:
				for _, e := range s.Exprs {
					expr(e)
				}
			case *DoBlockStmt:
				stmts(s.Stmts)
			case *WhileStmt:
				expr(s.Condition)
				stmts(s.Stmts)
			case *RepeatStmt:
				stmts(s.Stmts)
				expr(s.Condition)
			case *IfStmt:
				expr(s.Condition)
				stmts(s.Then)
				stmts(s.Else)
			case *NumberForStmt:
				expr(s.Init)
				expr(s.Limit)
				expr(s.Step)
				stmts(s.Stmts)
			case *GenericForStmt:
				for _, e := range s.Exprs {
					expr(e)
				}
				stmts(s.Stmts)
			}
		}
	}
	stmts(chunk)
	return found
}
