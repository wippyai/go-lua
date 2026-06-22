package diagnostics

import "github.com/wippyai/go-lua/compiler/ast"

func nestedElseIfStatements(stmts []*ast.IfStmt) map[*ast.IfStmt]bool {
	out := make(map[*ast.IfStmt]bool)
	for _, stmt := range stmts {
		if stmt == nil || len(stmt.Else) == 0 {
			continue
		}
		if nested, ok := stmt.Else[0].(*ast.IfStmt); ok && nested != nil {
			out[nested] = true
		}
	}
	return out
}

func hasElseIf(stmt *ast.IfStmt) bool {
	if stmt == nil || len(stmt.Else) == 0 {
		return false
	}
	_, ok := stmt.Else[0].(*ast.IfStmt)
	return ok
}

func hasDefaultElse(stmt *ast.IfStmt) bool {
	for stmt != nil {
		if len(stmt.Else) == 0 {
			return false
		}
		next, ok := stmt.Else[0].(*ast.IfStmt)
		if !ok {
			return true
		}
		stmt = next
	}
	return false
}

func ifElseIfChain(head *ast.IfStmt) []*ast.IfStmt {
	var chain []*ast.IfStmt
	for stmt := head; stmt != nil; {
		chain = append(chain, stmt)
		if len(stmt.Else) == 0 {
			break
		}
		next, ok := stmt.Else[0].(*ast.IfStmt)
		if !ok {
			break
		}
		stmt = next
	}
	return chain
}
