package trace

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/compiler/check/domain/paramuse"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/types/constraint"
)

// ParameterUses records the parameter surface demanded by fn's body.
func ParameterUses(graph *cfg.Graph, fn *ast.FunctionExpr) []api.ParameterUseEvidence {
	if graph == nil || fn == nil {
		return nil
	}
	paramSymbols := make(map[cfg.SymbolID]struct{})
	for _, slot := range graph.ParamSlotsReadOnly() {
		if slot.Symbol != 0 {
			paramSymbols[slot.Symbol] = struct{}{}
		}
	}
	if len(paramSymbols) == 0 {
		return nil
	}

	collector := parameterUseCollector{
		bindings:               graph.Bindings(),
		paramSymbols:           paramSymbols,
		currentFunctionSymbols: currentFunctionSymbols(graph, fn),
	}
	for _, stmt := range fn.Stmts {
		collector.stmt(stmt)
	}
	return collector.uses.Evidence()
}

type parameterUseCollector struct {
	bindings               *bind.BindingTable
	paramSymbols           map[cfg.SymbolID]struct{}
	currentFunctionSymbols map[cfg.SymbolID]struct{}
	uses                   paramuse.Set
}

func (c *parameterUseCollector) stmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		var skipRHS map[int]struct{}
		for i, lhs := range s.Lhs {
			if i < len(s.Rhs) && c.isParamSelfDefault(lhs, s.Rhs[i]) {
				if skipRHS == nil {
					skipRHS = make(map[int]struct{}, 1)
				}
				skipRHS[i] = struct{}{}
				continue
			}
			c.lvalue(lhs)
		}
		for i, rhs := range s.Rhs {
			if skipRHS != nil {
				if _, skip := skipRHS[i]; skip {
					continue
				}
			}
			c.expr(rhs)
		}
	case *ast.LocalAssignStmt:
		for _, expr := range s.Exprs {
			c.expr(expr)
		}
	case *ast.FuncCallStmt:
		c.expr(s.Expr)
	case *ast.DoBlockStmt:
		c.stmts(s.Stmts)
	case *ast.WhileStmt:
		c.condition(s.Condition)
		c.stmts(s.Stmts)
	case *ast.RepeatStmt:
		c.stmts(s.Stmts)
		c.condition(s.Condition)
	case *ast.IfStmt:
		c.condition(s.Condition)
		c.stmts(s.Then)
		c.stmts(s.Else)
	case *ast.NumberForStmt:
		c.expr(s.Init)
		c.expr(s.Limit)
		c.expr(s.Step)
		c.stmts(s.Stmts)
	case *ast.GenericForStmt:
		for _, expr := range s.Exprs {
			c.expr(expr)
		}
		c.stmts(s.Stmts)
	case *ast.FuncDefStmt:
		if s.Name != nil {
			c.expr(s.Name.Func)
			c.expr(s.Name.Receiver)
		}
		if s.Func != nil {
			c.stmts(s.Func.Stmts)
		}
	case *ast.ReturnStmt:
		for _, expr := range s.Exprs {
			c.expr(expr)
		}
	}
}

func (c *parameterUseCollector) stmts(stmts []ast.Stmt) {
	for _, stmt := range stmts {
		c.stmt(stmt)
	}
}

func (c *parameterUseCollector) condition(expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		if c.isParamIdent(e) {
			return
		}
	case *ast.UnaryNotOpExpr:
		if ident, ok := e.Expr.(*ast.IdentExpr); ok && c.isParamIdent(ident) {
			return
		}
		c.condition(e.Expr)
		return
	case *ast.RelationalOpExpr:
		if isNilLiteral(e.Lhs) && c.isParamExpr(e.Rhs) {
			return
		}
		if isNilLiteral(e.Rhs) && c.isParamExpr(e.Lhs) {
			return
		}
	case *ast.LogicalOpExpr:
		c.condition(e.Lhs)
		c.condition(e.Rhs)
		return
	}
	c.expr(expr)
}

func (c *parameterUseCollector) expr(expr ast.Expr) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.IdentExpr:
		c.whole(e)
	case *ast.AttrGetExpr:
		if c.pathUse(expr) {
			return
		}
		c.expr(e.Object)
		c.expr(e.Key)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			c.expr(field.Key)
			c.expr(field.Value)
		}
	case *ast.FuncCallExpr:
		c.call(e)
	case *ast.LogicalOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.RelationalOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.StringConcatOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.ArithmeticOpExpr:
		c.expr(e.Lhs)
		c.expr(e.Rhs)
	case *ast.UnaryMinusOpExpr:
		c.expr(e.Expr)
	case *ast.UnaryNotOpExpr:
		c.expr(e.Expr)
	case *ast.UnaryLenOpExpr:
		c.expr(e.Expr)
	case *ast.UnaryBNotOpExpr:
		c.expr(e.Expr)
	case *ast.FunctionExpr:
		c.stmts(e.Stmts)
	case *ast.CastExpr:
		c.expr(e.Expr)
	case *ast.NonNilAssertExpr:
		c.expr(e.Expr)
	}
}

func (c *parameterUseCollector) call(call *ast.FuncCallExpr) {
	if call == nil {
		return
	}
	recursive := c.isDirectRecursiveCall(call)
	if call.Method != "" {
		if recv := flowpath.FromExprWithBindings(call.Receiver, nil, c.bindings); c.isParamPath(recv) {
			if len(recv.Segments) == 0 {
				c.fieldName(recv.Symbol, call.Method)
			} else {
				c.fieldSegment(recv.Symbol, recv.Segments[0])
			}
		} else {
			c.expr(call.Receiver)
		}
	} else if callee := flowpath.FromExprWithBindings(call.Func, nil, c.bindings); c.isParamPath(callee) {
		if len(callee.Segments) == 0 {
			c.markWhole(callee.Symbol)
		} else {
			c.fieldSegment(callee.Symbol, callee.Segments[0])
		}
	} else {
		c.expr(call.Func)
	}

	for _, arg := range call.Args {
		if recursive && c.isParamExpr(arg) {
			continue
		}
		if c.isBuiltinTypeCall(call) && c.isParamExpr(arg) {
			continue
		}
		c.expr(arg)
	}
}

func (c *parameterUseCollector) isBuiltinTypeCall(call *ast.FuncCallExpr) bool {
	if call == nil || call.Method != "" {
		return false
	}
	ident, ok := call.Func.(*ast.IdentExpr)
	if !ok || ident == nil || ident.Value != "type" {
		return false
	}
	if c.bindings != nil {
		if sym, ok := c.bindings.SymbolOf(ident); ok && sym != 0 {
			kind, hasKind := c.bindings.Kind(sym)
			return hasKind && kind == cfg.SymbolGlobal
		}
	}
	return true
}

func (c *parameterUseCollector) isDirectRecursiveCall(call *ast.FuncCallExpr) bool {
	if call == nil || call.Method != "" || len(c.currentFunctionSymbols) == 0 {
		return false
	}
	callee := flowpath.FromExprWithBindings(call.Func, nil, c.bindings)
	if callee.Symbol == 0 || len(callee.Segments) != 0 {
		return false
	}
	_, ok := c.currentFunctionSymbols[callee.Symbol]
	return ok
}

func (c *parameterUseCollector) lvalue(expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return
	case *ast.AttrGetExpr:
		if c.lvaluePathUse(expr) {
			return
		}
		c.expr(e.Object)
		c.expr(e.Key)
	default:
		c.expr(expr)
	}
}

func (c *parameterUseCollector) lvaluePathUse(expr ast.Expr) bool {
	p := flowpath.FromExprWithBindings(expr, nil, c.bindings)
	if !c.isParamPath(p) {
		return false
	}
	if len(p.Segments) <= 1 {
		return true
	}
	c.fieldSegment(p.Symbol, p.Segments[0])
	return true
}

func (c *parameterUseCollector) isParamSelfDefault(lhs, rhs ast.Expr) bool {
	lhsIdent, ok := lhs.(*ast.IdentExpr)
	if !ok || !c.isParamIdent(lhsIdent) {
		return false
	}
	op, ok := rhs.(*ast.LogicalOpExpr)
	if !ok || op.Operator != "or" {
		return false
	}
	rhsIdent, ok := op.Lhs.(*ast.IdentExpr)
	if !ok || !c.sameParamIdent(lhsIdent, rhsIdent) {
		return false
	}
	_, ok = op.Rhs.(*ast.TableExpr)
	return ok
}

func (c *parameterUseCollector) pathUse(expr ast.Expr) bool {
	p := flowpath.FromExprWithBindings(expr, nil, c.bindings)
	if !c.isParamPath(p) {
		return false
	}
	if len(p.Segments) == 0 {
		c.markWhole(p.Symbol)
		return true
	}
	c.fieldSegment(p.Symbol, p.Segments[0])
	return true
}

func (c *parameterUseCollector) whole(expr ast.Expr) {
	if c.bindings == nil || expr == nil {
		return
	}
	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return
	}
	sym, ok := c.bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return
	}
	if _, isParam := c.paramSymbols[sym]; !isParam {
		return
	}
	c.markWhole(sym)
}

func (c *parameterUseCollector) isParamExpr(expr ast.Expr) bool {
	ident, ok := expr.(*ast.IdentExpr)
	return ok && c.isParamIdent(ident)
}

func (c *parameterUseCollector) isParamIdent(ident *ast.IdentExpr) bool {
	if c.bindings == nil || ident == nil {
		return false
	}
	sym, ok := c.bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return false
	}
	_, ok = c.paramSymbols[sym]
	return ok
}

func (c *parameterUseCollector) sameParamIdent(a, b *ast.IdentExpr) bool {
	if c.bindings == nil || a == nil || b == nil {
		return false
	}
	asym, aok := c.bindings.SymbolOf(a)
	bsym, bok := c.bindings.SymbolOf(b)
	if !aok || !bok || asym == 0 || bsym == 0 || asym != bsym {
		return false
	}
	_, ok := c.paramSymbols[asym]
	return ok
}

func isNilLiteral(expr ast.Expr) bool {
	_, ok := expr.(*ast.NilExpr)
	return ok
}

func (c *parameterUseCollector) isParamPath(p constraint.Path) bool {
	if p.IsEmpty() || p.Symbol == 0 {
		return false
	}
	_, ok := c.paramSymbols[p.Symbol]
	return ok
}

func (c *parameterUseCollector) markWhole(sym cfg.SymbolID) {
	c.uses.MarkWhole(sym)
}

func (c *parameterUseCollector) fieldName(sym cfg.SymbolID, name string) {
	key, ok := fieldkey.FromName(name)
	if !ok {
		c.markWhole(sym)
		return
	}
	c.field(sym, key)
}

func (c *parameterUseCollector) fieldSegment(sym cfg.SymbolID, seg constraint.Segment) {
	key, ok := fieldkey.FromSegment(seg)
	if !ok {
		c.markWhole(sym)
		return
	}
	c.field(sym, key)
}

func (c *parameterUseCollector) field(sym cfg.SymbolID, key fieldkey.Key) {
	c.uses.Field(sym, key)
}

func currentFunctionSymbols(graph *cfg.Graph, fn *ast.FunctionExpr) map[cfg.SymbolID]struct{} {
	if graph == nil || fn == nil {
		return nil
	}
	syms := make(map[cfg.SymbolID]struct{}, 1)
	if bindings := graph.Bindings(); bindings != nil {
		if sym, ok := bindings.FuncLitSymbol(fn); ok && sym != 0 {
			syms[sym] = struct{}{}
		}
	}
	for _, localFn := range graph.LocalFunctionAssignments() {
		if localFn.Func == fn && localFn.Symbol != 0 {
			syms[localFn.Symbol] = struct{}{}
		}
	}
	for _, def := range FunctionDefinitions(graph) {
		if def.FuncDef != nil && def.FuncDef.FuncExpr == fn && def.Symbol != 0 {
			syms[def.Symbol] = struct{}{}
		}
	}
	if len(syms) == 0 {
		return nil
	}
	return syms
}
