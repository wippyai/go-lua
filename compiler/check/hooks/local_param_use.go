package hooks

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
)

// unobservedLocalParamMask returns true for graph-local parameter slots whose
// value is never observed by the function body. Call diagnostics do not need to
// reject an explicit any argument for such a slot because no transfer can depend
// on the stricter annotation at runtime.
func unobservedLocalParamMask(
	store api.StoreReader,
	sym cfg.SymbolID,
	fn *ast.FunctionExpr,
	cache map[cfg.SymbolID][]bool,
) []bool {
	if sym == 0 || fn == nil {
		return nil
	}
	if cache != nil {
		if mask, ok := cache[sym]; ok {
			return mask
		}
	}
	fnGraph := graphForFunctionSymbol(store, sym)
	mask := computeUnobservedLocalParamMask(fn, fnGraph)
	if cache != nil {
		cache[sym] = mask
	}
	return mask
}

func graphForFunctionSymbol(store api.StoreReader, sym cfg.SymbolID) *cfg.Graph {
	if store == nil || sym == 0 {
		return nil
	}
	ref := store.FunctionRefBySym(sym)
	if ref == nil || ref.GraphID == 0 {
		return nil
	}
	graphs := store.Graphs()
	if graphs == nil {
		return nil
	}
	return graphs[ref.GraphID]
}

func computeUnobservedLocalParamMask(fn *ast.FunctionExpr, graph *cfg.Graph) []bool {
	if fn == nil || graph == nil {
		return nil
	}
	slots := graph.ParamSlotsReadOnly()
	if len(slots) == 0 {
		return nil
	}
	bindings := graph.Bindings()
	if bindings == nil {
		return nil
	}

	paramIndex := make(map[cfg.SymbolID]int, len(slots))
	for i, slot := range slots {
		if slot.Symbol != 0 {
			paramIndex[slot.Symbol] = i
		}
	}
	if len(paramIndex) == 0 {
		return nil
	}

	used := make([]bool, len(slots))
	markParamUsesInStmts(fn.Stmts, bindings, paramIndex, used)

	var mask []bool
	for i, slot := range slots {
		if slot.Symbol == 0 || used[i] {
			continue
		}
		if mask == nil {
			mask = make([]bool, len(slots))
		}
		mask[i] = true
	}
	return mask
}

func markParamUsesInStmts(stmts []ast.Stmt, bindings *bind.BindingTable, paramIndex map[cfg.SymbolID]int, used []bool) {
	for _, stmt := range stmts {
		markParamUsesInStmt(stmt, bindings, paramIndex, used)
	}
}

func markParamUsesInStmt(stmt ast.Stmt, bindings *bind.BindingTable, paramIndex map[cfg.SymbolID]int, used []bool) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		markParamUsesInExprs(s.Lhs, bindings, paramIndex, used)
		markParamUsesInExprs(s.Rhs, bindings, paramIndex, used)
	case *ast.LocalAssignStmt:
		markParamUsesInExprs(s.Exprs, bindings, paramIndex, used)
	case *ast.FuncCallStmt:
		markParamUsesInExpr(s.Expr, bindings, paramIndex, used)
	case *ast.DoBlockStmt:
		markParamUsesInStmts(s.Stmts, bindings, paramIndex, used)
	case *ast.WhileStmt:
		markParamUsesInExpr(s.Condition, bindings, paramIndex, used)
		markParamUsesInStmts(s.Stmts, bindings, paramIndex, used)
	case *ast.RepeatStmt:
		markParamUsesInStmts(s.Stmts, bindings, paramIndex, used)
		markParamUsesInExpr(s.Condition, bindings, paramIndex, used)
	case *ast.IfStmt:
		markParamUsesInExpr(s.Condition, bindings, paramIndex, used)
		markParamUsesInStmts(s.Then, bindings, paramIndex, used)
		markParamUsesInStmts(s.Else, bindings, paramIndex, used)
	case *ast.NumberForStmt:
		markParamUsesInExpr(s.Init, bindings, paramIndex, used)
		markParamUsesInExpr(s.Limit, bindings, paramIndex, used)
		markParamUsesInExpr(s.Step, bindings, paramIndex, used)
		markParamUsesInStmts(s.Stmts, bindings, paramIndex, used)
	case *ast.GenericForStmt:
		markParamUsesInExprs(s.Exprs, bindings, paramIndex, used)
		markParamUsesInStmts(s.Stmts, bindings, paramIndex, used)
	case *ast.FuncDefStmt:
		if s.Name != nil {
			markParamUsesInExpr(s.Name.Func, bindings, paramIndex, used)
			markParamUsesInExpr(s.Name.Receiver, bindings, paramIndex, used)
		}
		if s.Func != nil {
			markParamUsesInExpr(s.Func, bindings, paramIndex, used)
		}
	case *ast.ReturnStmt:
		markParamUsesInExprs(s.Exprs, bindings, paramIndex, used)
	}
}

func markParamUsesInExprs(exprs []ast.Expr, bindings *bind.BindingTable, paramIndex map[cfg.SymbolID]int, used []bool) {
	for _, expr := range exprs {
		markParamUsesInExpr(expr, bindings, paramIndex, used)
	}
}

func markParamUsesInExpr(expr ast.Expr, bindings *bind.BindingTable, paramIndex map[cfg.SymbolID]int, used []bool) {
	switch e := expr.(type) {
	case nil:
		return
	case *ast.IdentExpr:
		if sym, ok := bindings.SymbolOf(e); ok {
			if idx, ok := paramIndex[sym]; ok && idx >= 0 && idx < len(used) {
				used[idx] = true
			}
		}
	case *ast.AttrGetExpr:
		markParamUsesInExpr(e.Object, bindings, paramIndex, used)
		markParamUsesInExpr(e.Key, bindings, paramIndex, used)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			markParamUsesInExpr(field.Key, bindings, paramIndex, used)
			markParamUsesInExpr(field.Value, bindings, paramIndex, used)
		}
	case *ast.FuncCallExpr:
		markParamUsesInExpr(e.Func, bindings, paramIndex, used)
		markParamUsesInExpr(e.Receiver, bindings, paramIndex, used)
		markParamUsesInExprs(e.Args, bindings, paramIndex, used)
	case *ast.LogicalOpExpr:
		markParamUsesInExpr(e.Lhs, bindings, paramIndex, used)
		markParamUsesInExpr(e.Rhs, bindings, paramIndex, used)
	case *ast.RelationalOpExpr:
		markParamUsesInExpr(e.Lhs, bindings, paramIndex, used)
		markParamUsesInExpr(e.Rhs, bindings, paramIndex, used)
	case *ast.StringConcatOpExpr:
		markParamUsesInExpr(e.Lhs, bindings, paramIndex, used)
		markParamUsesInExpr(e.Rhs, bindings, paramIndex, used)
	case *ast.ArithmeticOpExpr:
		markParamUsesInExpr(e.Lhs, bindings, paramIndex, used)
		markParamUsesInExpr(e.Rhs, bindings, paramIndex, used)
	case *ast.UnaryMinusOpExpr:
		markParamUsesInExpr(e.Expr, bindings, paramIndex, used)
	case *ast.UnaryNotOpExpr:
		markParamUsesInExpr(e.Expr, bindings, paramIndex, used)
	case *ast.UnaryLenOpExpr:
		markParamUsesInExpr(e.Expr, bindings, paramIndex, used)
	case *ast.UnaryBNotOpExpr:
		markParamUsesInExpr(e.Expr, bindings, paramIndex, used)
	case *ast.FunctionExpr:
		markParamUsesInStmts(e.Stmts, bindings, paramIndex, used)
	case *ast.CastExpr:
		markParamUsesInExpr(e.Expr, bindings, paramIndex, used)
	case *ast.NonNilAssertExpr:
		markParamUsesInExpr(e.Expr, bindings, paramIndex, used)
	}
}
