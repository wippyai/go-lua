package bind

import (
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (b *binder) bindStmts(stmts []ast.Stmt) {
	b.hoistTypeDecls(stmts)
	for _, stmt := range stmts {
		b.bindStmt(stmt)
	}
}

// hoistTypeDecls pre-declares every type alias and interface in the current
// type scope before any body is bound. Type declarations are not
// order-dependent within a scope, so a sibling alias may reference one
// declared later (forward reference) or reference itself through a chain
// (recursive and mutually-recursive types).
func (b *binder) hoistTypeDecls(stmts []ast.Stmt) {
	for _, stmt := range stmts {
		switch stmt := stmt.(type) {
		case *ast.TypeDefStmt:
			b.declareTypeDef(stmt)
		case *ast.InterfaceDefStmt:
			b.declareInterfaceDef(stmt)
		}
	}
}

func (b *binder) bindStmt(stmt ast.Stmt) {
	switch stmt := stmt.(type) {
	case *ast.AssignStmt:
		b.bindExprs(stmt.Rhs)
		for _, lhs := range stmt.Lhs {
			b.bindLValue(lhs)
		}
	case *ast.LocalAssignStmt:
		b.bindLocalAssign(stmt)
	case *ast.FuncCallStmt:
		b.bindExpr(stmt.Expr)
	case *ast.DoBlockStmt:
		b.pushScope()
		b.bindStmts(stmt.Stmts)
		b.popScope()
	case *ast.WhileStmt:
		b.bindExpr(stmt.Condition)
		b.pushScope()
		b.bindStmts(stmt.Stmts)
		b.popScope()
	case *ast.RepeatStmt:
		b.pushScope()
		b.bindStmts(stmt.Stmts)
		b.bindExpr(stmt.Condition)
		b.popScope()
	case *ast.IfStmt:
		b.bindExpr(stmt.Condition)
		b.pushScope()
		b.bindStmts(stmt.Then)
		b.popScope()
		if len(stmt.Else) > 0 {
			b.pushScope()
			b.bindStmts(stmt.Else)
			b.popScope()
		}
	case *ast.NumberForStmt:
		b.bindNumberFor(stmt)
	case *ast.GenericForStmt:
		b.bindGenericFor(stmt)
	case *ast.FuncDefStmt:
		b.bindFuncDef(stmt)
	case *ast.ReturnStmt:
		b.bindExprs(stmt.Exprs)
	case *ast.BreakStmt, *ast.LabelStmt, *ast.GotoStmt:
	case *ast.TypeDefStmt:
		b.bindTypeDef(stmt)
	case *ast.InterfaceDefStmt:
		b.bindInterfaceDef(stmt)
	default:
		b.invalidateRuntimeUseScan()
	}
}

func (b *binder) bindLocalAssign(stmt *ast.LocalAssignStmt) {
	b.bindTypeExprs(stmt.Types)

	ids := make([]symbol.ID, len(stmt.Names))
	pending := make(map[string]symbol.ID, len(stmt.Names))
	for i, name := range stmt.Names {
		id := b.newSymbol(name, symbol.Local)
		b.result.setDeclaration(id, declarationForPosition(namePosition(stmt.NamePositions, i), name, false))
		ids[i] = id
		if name != "" {
			pending[name] = id
		}
	}
	b.result.localSymbols[stmt] = ids

	oldDeferredLen := len(b.deferred)
	if len(pending) > 0 && len(b.scopes) > 0 {
		b.deferred = append(b.deferred, deferredScope{
			scopeIndex: len(b.scopes) - 1,
			names:      pending,
		})
	}
	for i, expr := range stmt.Exprs {
		if fn, ok := expr.(*ast.FunctionExpr); ok {
			details := functionOriginDetails{
				kind:       FunctionOriginLiteral,
				localIndex: -1,
			}
			if i < len(ids) {
				details.kind = FunctionOriginLocalAssignment
				details.stmt = stmt
				details.localIndex = i
				details.targetSymbol = ids[i]
				details.hasTargetSymbol = ids[i] != 0
			}
			b.bindFunction(fn, false, details)
			continue
		}
		b.bindExpr(expr)
	}
	b.deferred = b.deferred[:oldDeferredLen]
	if b.visibleDeferred > len(b.deferred) {
		b.visibleDeferred = len(b.deferred)
	}

	for i, name := range stmt.Names {
		b.define(name, ids[i])
	}
}

func (b *binder) bindNumberFor(stmt *ast.NumberForStmt) {
	b.bindExpr(stmt.Init)
	b.bindExpr(stmt.Limit)
	b.bindExpr(stmt.Step)

	id := b.newSymbol(stmt.Name, symbol.Local)
	b.result.setDeclaration(id, declarationForPosition(stmt.NamePosition, stmt.Name, false))
	b.result.numForSymbols[stmt] = id

	b.pushScope()
	b.define(stmt.Name, id)
	b.bindStmts(stmt.Stmts)
	b.popScope()
}

func (b *binder) bindGenericFor(stmt *ast.GenericForStmt) {
	b.bindExprs(stmt.Exprs)

	ids := make([]symbol.ID, len(stmt.Names))
	b.pushScope()
	for i, name := range stmt.Names {
		id := b.newSymbol(name, symbol.Local)
		b.result.setDeclaration(id, declarationForPosition(namePosition(stmt.NamePositions, i), name, false))
		ids[i] = id
		b.define(name, id)
	}
	b.result.genericForSymbols[stmt] = ids
	b.bindStmts(stmt.Stmts)
	b.popScope()
}

func (b *binder) bindFuncDef(stmt *ast.FuncDefStmt) {
	if stmt.Name != nil {
		if stmt.Name.Func != nil {
			b.bindLValue(stmt.Name.Func)
		}
		if stmt.Name.Receiver != nil {
			b.bindExpr(stmt.Name.Receiver)
		}
	}
	details := functionOriginDetails{
		kind:       FunctionOriginDeclaration,
		stmt:       stmt,
		localIndex: -1,
	}
	if stmt.Name != nil && stmt.Name.Method != "" {
		details.kind = FunctionOriginMethod
		details.method = stmt.Name.Method
		if name := receiverTypeName(stmt.Name.Receiver); name != "" {
			if decl, ok := b.lookupType(name); ok {
				details.receiverType = decl
				details.hasReceiverType = true
			}
		}
	} else if id, ok := b.result.FuncDefTargetSymbol(stmt); ok {
		details.targetSymbol = id
		details.hasTargetSymbol = true
	}
	b.bindFunction(stmt.Func, stmt.Name != nil && stmt.Name.Method != "", details)
}

// receiverTypeName returns the type name that a colon-method receiver
// expression refers to. For `function R:m` the receiver is the identifier R;
// for `function ns.R:m` it is the trailing field R. The returned name is used
// to find the sibling type declaration that types the implicit self receiver.
func receiverTypeName(receiver ast.Expr) string {
	switch e := receiver.(type) {
	case *ast.IdentExpr:
		return e.Value
	case *ast.AttrGetExpr:
		if e.KeySyntax == ast.AttrKeyDot {
			return ast.KeyName(e.Key)
		}
	}
	return ""
}

func (b *binder) bindExprs(exprs []ast.Expr) {
	for _, expr := range exprs {
		b.bindExpr(expr)
	}
}

type exprBindMode uint8

const (
	exprBindRuntime exprBindMode = iota
	exprBindTypeQuery
)

func (b *binder) bindExpr(expr ast.Expr) {
	b.bindExprMode(expr, exprBindRuntime)
}

func (b *binder) bindTypeQueryExpr(expr ast.Expr) {
	b.bindExprMode(expr, exprBindTypeQuery)
}

func (b *binder) bindExprMode(expr ast.Expr, mode exprBindMode) {
	switch expr := expr.(type) {
	case nil:
	case *ast.TrueExpr, *ast.FalseExpr, *ast.NilExpr, *ast.NumberExpr, *ast.StringExpr:
	case *ast.Comma3Expr:
		if mode == exprBindRuntime {
			b.bindVararg(expr)
		}
	case *ast.IdentExpr:
		if mode == exprBindTypeQuery {
			b.bindTypeQueryIdent(expr)
		} else {
			b.bindReadIdent(expr)
		}
	case *ast.AttrGetExpr:
		b.bindExprMode(expr.Object, mode)
		if expr.KeySyntax != ast.AttrKeyDot {
			b.bindExprMode(expr.Key, mode)
		}
	case *ast.TableExpr:
		for _, field := range expr.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax != ast.AttrKeyDot {
				b.bindExprMode(field.Key, mode)
			}
			b.bindExprMode(field.Value, mode)
		}
	case *ast.FuncCallExpr:
		b.bindExprMode(expr.Func, mode)
		if mode == exprBindRuntime && expr.Method == "" && expr.Receiver == nil {
			if ident, ok := expr.Func.(*ast.IdentExpr); ok {
				if id, ok := b.result.SymbolOf(ident); ok && id != 0 {
					if b.result.directCalls == nil {
						b.result.directCalls = make(map[symbol.ID][]*ast.FuncCallExpr)
					}
					b.result.directCalls[id] = append(b.result.directCalls[id], expr)
				}
			}
		}
		b.bindExprMode(expr.Receiver, mode)
		for _, arg := range expr.Args {
			b.bindExprMode(arg, mode)
		}
		b.bindTypeExprs(expr.TypeArgs)
	case *ast.LogicalOpExpr:
		b.bindExprMode(expr.Lhs, mode)
		b.bindExprMode(expr.Rhs, mode)
	case *ast.RelationalOpExpr:
		b.bindExprMode(expr.Lhs, mode)
		b.bindExprMode(expr.Rhs, mode)
	case *ast.StringConcatOpExpr:
		b.bindExprMode(expr.Lhs, mode)
		b.bindExprMode(expr.Rhs, mode)
	case *ast.ArithmeticOpExpr:
		b.bindExprMode(expr.Lhs, mode)
		b.bindExprMode(expr.Rhs, mode)
	case *ast.UnaryMinusOpExpr:
		b.bindExprMode(expr.Expr, mode)
	case *ast.UnaryNotOpExpr:
		b.bindExprMode(expr.Expr, mode)
	case *ast.UnaryLenOpExpr:
		b.bindExprMode(expr.Expr, mode)
	case *ast.UnaryBNotOpExpr:
		b.bindExprMode(expr.Expr, mode)
	case *ast.FunctionExpr:
		if mode == exprBindRuntime {
			b.bindFunction(expr, false, functionOriginDetails{
				kind:       FunctionOriginLiteral,
				localIndex: -1,
			})
		} else {
			b.bindFunctionTypeSignature(expr)
		}
	case *ast.CastExpr:
		b.bindExprMode(expr.Expr, mode)
		b.bindTypeExpr(expr.Type)
	case *ast.NonNilAssertExpr:
		b.bindExprMode(expr.Expr, mode)
	default:
		if mode == exprBindRuntime {
			b.invalidateRuntimeUseScan()
		}
	}
}

func (b *binder) invalidateRuntimeUseScan() {
	if b != nil && b.result != nil {
		b.result.runtimeUseScanComplete = false
	}
}
