package bind

import "github.com/wippyai/go-lua/compiler/ast"

// The statement lane owns ordered statement traversal, scope entry, local
// declaration timing, and loop/function body entry. It deliberately remains
// part of bind's single state machine so declaration order and pending-scope
// visibility cannot diverge across packages.

func (b *binder) visitStmtList(step bindStep) {
	stmts := b.statementList(step.node, step.phase)
	if step.index < 0 {
		b.hoistTypeDecls(stmts)
		if step.node == nil && step.phase == phaseChunk {
			b.recordChunkRuntimeTypeNames(stmts)
		}
		_, repeatBody := step.node.(*ast.RepeatStmt)
		b.control.indexLabels(stmts, !repeatBody)
		step.index = 0
	}
	if step.index >= len(stmts) {
		return
	}
	stmt := stmts[step.index]
	step.index++
	b.push(step)
	b.push(bindStep{kind: stepStmt, node: stmt, mode: step.mode})
}

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

func (b *binder) statementList(node ast.PositionHolder, phase stepPhase) []ast.Stmt {
	if node == nil {
		return b.rootStmts
	}
	switch n := node.(type) {
	case *ast.FunctionExpr:
		return n.Stmts
	case *ast.DoBlockStmt:
		return n.Stmts
	case *ast.WhileStmt:
		return n.Stmts
	case *ast.RepeatStmt:
		return n.Stmts
	case *ast.IfStmt:
		if phase == phaseElse {
			return n.Else
		}
		return n.Then
	case *ast.NumberForStmt:
		return n.Stmts
	case *ast.GenericForStmt:
		return n.Stmts
	default:
		return nil
	}
}

func (b *binder) visitStmt(step bindStep) {
	stmt := step.node.(ast.Stmt)
	mode := step.mode
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		b.result.beginGlobalOrderSegment()
		b.push(bindStep{kind: stepAssignTargets, node: s, mode: mode})
		b.push(bindStep{kind: stepExprList, node: s, mode: mode})
	case *ast.LocalAssignStmt:
		b.push(bindStep{kind: stepLocalAfterTypes, node: s, mode: mode})
		b.push(bindStep{kind: stepTypeList, node: s, phase: phaseLocalTypes})
	case *ast.FuncCallStmt:
		b.scheduleExpr(s.Expr, mode)
	case *ast.DoBlockStmt:
		b.pushScope()
		b.push(bindStep{kind: stepLeaveScope})
		b.scheduleStmtList(s, phaseBody, mode)
	case *ast.WhileStmt:
		b.push(bindStep{kind: stepWhileBody, node: s, mode: mode})
		b.scheduleExpr(s.Condition, mode)
	case *ast.RepeatStmt:
		b.control.enterLoop()
		b.push(bindStep{kind: stepLeaveLoop})
		b.pushScope()
		b.push(bindStep{kind: stepLeaveScope})
		b.scheduleExpr(s.Condition, mode)
		b.scheduleStmtList(s, phaseBody, mode)
	case *ast.IfStmt:
		b.push(bindStep{kind: stepIfThen, node: s, mode: mode})
		b.scheduleExpr(s.Condition, mode)
	case *ast.NumberForStmt:
		b.push(bindStep{kind: stepNumberForBody, node: s, mode: mode})
		b.scheduleExpr(s.Step, mode)
		b.scheduleExpr(s.Limit, mode)
		b.scheduleExpr(s.Init, mode)
	case *ast.GenericForStmt:
		b.push(bindStep{kind: stepGenericForBody, node: s, mode: mode})
		b.push(bindStep{kind: stepExprList, node: s, mode: mode})
	case *ast.FuncDefStmt:
		b.push(bindStep{kind: stepFuncDefFunction, node: s, mode: mode})
		if s.Name != nil {
			b.scheduleExpr(s.Name.Receiver, mode)
			b.scheduleLValue(s.Name.Func, mode)
		}
	case *ast.ReturnStmt:
		b.push(bindStep{kind: stepExprList, node: s, mode: mode})
	case *ast.BreakStmt:
		b.control.visitBreak(s)
	case *ast.LabelStmt:
		b.control.visitLabel(s)
	case *ast.GotoStmt:
		b.control.visitGoto(s)
	case *ast.TypeDefStmt:
		b.beginTypeDef(s)
	case *ast.InterfaceDefStmt:
		b.beginInterface(s)
	}
}

func (b *binder) visitAssignTargets(step bindStep) {
	stmt := step.node.(*ast.AssignStmt)
	if step.index == 0 {
		b.result.beginGlobalOrderTargets()
	}
	if step.index >= len(stmt.Lhs) {
		b.recordStaticTypePublications(stmt)
		b.result.endGlobalOrderSegment()
		return
	}
	target := stmt.Lhs[step.index]
	step.index++
	b.push(step)
	b.scheduleLValue(target, step.mode)
}

func (b *binder) beginLocal(stmt *ast.LocalAssignStmt, mode exprBindMode) {
	ids := make([]Symbol, len(stmt.Names))
	pending := make(map[string]Symbol, len(stmt.Names))
	for i, name := range stmt.Names {
		id := b.newSymbol(name, SymbolLocal)
		b.result.setSymbolTypeAnnotation(id, typeAt(stmt.Types, i))
		ids[i] = id
		if name != "" {
			pending[name] = id
		}
	}
	b.result.localSymbols[stmt] = ids
	mark := len(b.pending)
	// Only the Lua declaration form `local function f` installs f for its own
	// Function body. An ordinary local initializer is evaluated before its new
	// local enters scope, even when that initializer is a Function literal.
	if stmt.LocalFunction && len(stmt.Names) == 1 && len(stmt.Exprs) == 1 {
		if fn, ok := stmt.Exprs[0].(*ast.FunctionExpr); ok && fn != nil {
			mark = b.pushPending(pending)
		}
	}
	b.push(bindStep{kind: stepLocalFinish, node: stmt, mark: mark, mode: mode})
	b.push(bindStep{kind: stepLocalValues, node: stmt, mode: mode})
}

func (b *binder) visitLocalValues(step bindStep) {
	stmt := step.node.(*ast.LocalAssignStmt)
	if step.index >= len(stmt.Exprs) {
		return
	}
	expr := stmt.Exprs[step.index]
	slot := step.index
	step.index++
	b.push(step)
	if fn, ok := expr.(*ast.FunctionExpr); ok {
		details := functionOriginDetails{kind: FunctionOriginLiteral, localIndex: -1}
		if slot < len(stmt.Names) {
			details.kind = FunctionOriginLocalAssignment
			details.stmt = stmt
			details.localIndex = slot
		}
		b.enterFunction(fn, false, details, step.mode)
		return
	}
	b.scheduleExpr(expr, step.mode)
}

func (b *binder) finishLocal(stmt *ast.LocalAssignStmt, mark int) {
	b.popPending(mark)
	ids := b.result.localSymbols[stmt]
	for i, name := range stmt.Names {
		if i < len(ids) {
			b.define(name, ids[i])
		}
	}
}

func (b *binder) enterBody(node ast.PositionHolder, mode exprBindMode) {
	b.pushScope()
	b.push(bindStep{kind: stepLeaveScope})
	b.scheduleStmtList(node, phaseBody, mode)
}

func (b *binder) enterLoopBody(node ast.PositionHolder, mode exprBindMode) {
	b.control.enterLoop()
	b.push(bindStep{kind: stepLeaveLoop})
	b.enterBody(node, mode)
}

func (b *binder) enterIfThen(stmt *ast.IfStmt, mode exprBindMode) {
	b.push(bindStep{kind: stepIfElse, node: stmt, mode: mode})
	b.pushScope()
	b.push(bindStep{kind: stepLeaveScope})
	b.scheduleStmtList(stmt, phaseThen, mode)
}

func (b *binder) enterIfElse(stmt *ast.IfStmt, mode exprBindMode) {
	if len(stmt.Else) == 0 {
		return
	}
	b.pushScope()
	b.push(bindStep{kind: stepLeaveScope})
	b.scheduleStmtList(stmt, phaseElse, mode)
}

func (b *binder) enterNumberFor(stmt *ast.NumberForStmt, mode exprBindMode) {
	id := b.newSymbol(stmt.Name, SymbolLocal)
	b.result.numForSymbols[stmt] = id
	b.control.enterLoop()
	b.push(bindStep{kind: stepLeaveLoop})
	b.pushScope()
	b.define(stmt.Name, id)
	b.push(bindStep{kind: stepLeaveScope})
	b.scheduleStmtList(stmt, phaseBody, mode)
}

func (b *binder) enterGenericFor(stmt *ast.GenericForStmt, mode exprBindMode) {
	ids := make([]Symbol, len(stmt.Names))
	b.control.enterLoop()
	b.push(bindStep{kind: stepLeaveLoop})
	b.pushScope()
	for i, name := range stmt.Names {
		id := b.newSymbol(name, SymbolLocal)
		ids[i] = id
		b.define(name, id)
	}
	b.result.genericForSymbols[stmt] = ids
	b.push(bindStep{kind: stepLeaveScope})
	b.scheduleStmtList(stmt, phaseBody, mode)
}

func (b *binder) enterFuncDef(stmt *ast.FuncDefStmt, mode exprBindMode) {
	details := functionOriginDetails{
		kind:       FunctionOriginDeclaration,
		stmt:       stmt,
		localIndex: -1,
	}
	method := stmt.Name != nil && stmt.Name.Method != ""
	if method {
		details.kind = FunctionOriginMethod
		details.method = stmt.Name.Method
		details.methodPosition = stmt.Name.MethodPosition
		if name := receiverTypeName(stmt.Name.Receiver); name != "" {
			if decl, ok := b.lookupType(name); ok {
				details.receiverType = decl
				details.hasReceiverType = true
			}
		}
	}
	b.enterFunction(stmt.Func, method, details, mode)
}
