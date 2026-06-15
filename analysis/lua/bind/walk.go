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
	}
}

func (b *binder) bindLocalAssign(stmt *ast.LocalAssignStmt) {
	b.bindTypeExprs(stmt.Types)

	ids := make([]symbol.ID, len(stmt.Names))
	pending := make(map[string]symbol.ID, len(stmt.Names))
	for i, name := range stmt.Names {
		id := b.newSymbol(name, symbol.Local)
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

func (b *binder) bindExpr(expr ast.Expr) {
	switch expr := expr.(type) {
	case nil:
	case *ast.TrueExpr, *ast.FalseExpr, *ast.NilExpr, *ast.NumberExpr, *ast.StringExpr:
	case *ast.Comma3Expr:
		b.bindVararg()
	case *ast.IdentExpr:
		b.bindReadIdent(expr)
	case *ast.AttrGetExpr:
		b.bindExpr(expr.Object)
		if expr.KeySyntax != ast.AttrKeyDot {
			b.bindExpr(expr.Key)
		}
	case *ast.TableExpr:
		for _, field := range expr.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax != ast.AttrKeyDot {
				b.bindExpr(field.Key)
			}
			b.bindExpr(field.Value)
		}
	case *ast.FuncCallExpr:
		b.bindExpr(expr.Func)
		b.bindExpr(expr.Receiver)
		b.bindExprs(expr.Args)
		b.bindTypeExprs(expr.TypeArgs)
	case *ast.LogicalOpExpr:
		b.bindExpr(expr.Lhs)
		b.bindExpr(expr.Rhs)
	case *ast.RelationalOpExpr:
		b.bindExpr(expr.Lhs)
		b.bindExpr(expr.Rhs)
	case *ast.StringConcatOpExpr:
		b.bindExpr(expr.Lhs)
		b.bindExpr(expr.Rhs)
	case *ast.ArithmeticOpExpr:
		b.bindExpr(expr.Lhs)
		b.bindExpr(expr.Rhs)
	case *ast.UnaryMinusOpExpr:
		b.bindExpr(expr.Expr)
	case *ast.UnaryNotOpExpr:
		b.bindExpr(expr.Expr)
	case *ast.UnaryLenOpExpr:
		b.bindExpr(expr.Expr)
	case *ast.UnaryBNotOpExpr:
		b.bindExpr(expr.Expr)
	case *ast.FunctionExpr:
		b.bindFunction(expr, false, functionOriginDetails{
			kind:       FunctionOriginLiteral,
			localIndex: -1,
		})
	case *ast.CastExpr:
		b.bindExpr(expr.Expr)
		b.bindTypeExpr(expr.Type)
	case *ast.NonNilAssertExpr:
		b.bindExpr(expr.Expr)
	}
}
