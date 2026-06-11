package cfgbuild

import (
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (b *builder) loopDirectModifiedOuters(loopVars []symbol.ID, stmts []ast.Stmt) []symbol.ID {
	excluded := make(map[symbol.ID]struct{})
	addLoopSymbols(excluded, loopVars)
	b.collectLoopLocalSymbols(excluded, stmts)

	seen := make(map[symbol.ID]struct{})
	var out []symbol.ID
	b.collectLoopWrites(&out, seen, excluded, stmts)
	return out
}

func (b *builder) collectLoopLocalSymbols(out map[symbol.ID]struct{}, stmts []ast.Stmt) {
	for _, stmt := range stmts {
		switch stmt := stmt.(type) {
		case *ast.LocalAssignStmt:
			addLoopSymbols(out, b.bindings.LocalSymbols(stmt))
		case *ast.DoBlockStmt:
			b.collectLoopLocalSymbols(out, stmt.Stmts)
		case *ast.IfStmt:
			b.collectLoopLocalSymbols(out, stmt.Then)
			b.collectLoopLocalSymbols(out, stmt.Else)
		case *ast.WhileStmt:
			b.collectLoopLocalSymbols(out, stmt.Stmts)
		case *ast.RepeatStmt:
			b.collectLoopLocalSymbols(out, stmt.Stmts)
		case *ast.NumberForStmt:
			if id, ok := b.bindings.NumForSymbol(stmt); ok {
				addLoopSymbol(out, id)
			}
			b.collectLoopLocalSymbols(out, stmt.Stmts)
		case *ast.GenericForStmt:
			addLoopSymbols(out, b.bindings.GenericForSymbols(stmt))
			b.collectLoopLocalSymbols(out, stmt.Stmts)
		case *ast.FuncDefStmt:
			// Function definition targets are same-function writes; closure bodies are not.
		}
	}
}

func (b *builder) collectLoopWrites(out *[]symbol.ID, seen, excluded map[symbol.ID]struct{}, stmts []ast.Stmt) {
	for _, stmt := range stmts {
		switch stmt := stmt.(type) {
		case *ast.AssignStmt:
			for _, lhs := range stmt.Lhs {
				ident, ok := lhs.(*ast.IdentExpr)
				if !ok {
					continue
				}
				id, ok := b.bindings.SymbolOf(ident)
				if ok {
					appendLoopWrite(out, seen, excluded, id)
				}
			}
		case *ast.FuncDefStmt:
			id, ok := b.bindings.FuncDefTargetSymbol(stmt)
			if ok {
				appendLoopWrite(out, seen, excluded, id)
			}
		case *ast.DoBlockStmt:
			b.collectLoopWrites(out, seen, excluded, stmt.Stmts)
		case *ast.IfStmt:
			b.collectLoopWrites(out, seen, excluded, stmt.Then)
			b.collectLoopWrites(out, seen, excluded, stmt.Else)
		case *ast.WhileStmt:
			b.collectLoopWrites(out, seen, excluded, stmt.Stmts)
		case *ast.RepeatStmt:
			b.collectLoopWrites(out, seen, excluded, stmt.Stmts)
		case *ast.NumberForStmt:
			b.collectLoopWrites(out, seen, excluded, stmt.Stmts)
		case *ast.GenericForStmt:
			b.collectLoopWrites(out, seen, excluded, stmt.Stmts)
		}
	}
}

func appendLoopWrite(out *[]symbol.ID, seen, excluded map[symbol.ID]struct{}, id symbol.ID) {
	if id == 0 {
		return
	}
	if _, ok := excluded[id]; ok {
		return
	}
	if _, ok := seen[id]; ok {
		return
	}
	seen[id] = struct{}{}
	*out = append(*out, id)
}

func addLoopSymbols(out map[symbol.ID]struct{}, ids []symbol.ID) {
	for _, id := range ids {
		addLoopSymbol(out, id)
	}
}

func addLoopSymbol(out map[symbol.ID]struct{}, id symbol.ID) {
	if id != 0 {
		out[id] = struct{}{}
	}
}
