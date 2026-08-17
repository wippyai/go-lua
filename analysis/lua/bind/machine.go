package bind

import (
	"github.com/wippyai/go-lua/compiler/ast"
)

type exprBindMode uint8

const (
	exprBindRuntime exprBindMode = iota
	exprBindTypeQuery
)

type stepKind uint8

const (
	stepStmtList stepKind = iota + 1
	stepStmt
	stepExprList
	stepExpr
	stepDirectGlobalCall
	stepTypeList
	stepType
	stepLValue
	stepLeaveScope
	stepAssignTargets
	stepLocalAfterTypes
	stepLocalValues
	stepLocalFinish
	stepWhileBody
	stepLeaveLoop
	stepIfThen
	stepIfElse
	stepNumberForBody
	stepGenericForBody
	stepFuncDefFunction
	stepTableFields
	stepFunctionAfterConstraints
	stepFunctionLeave
	stepTypeScopeLeave
	stepTypeDefAfterConstraints
	stepFunctionTypeAfterConstraints
	stepRecordFields
	stepTypeParamConstraints
	stepFunctionParamTypes
	stepInterfaceMembers
)

type stepPhase uint8

const (
	phaseChunk stepPhase = iota
	phaseBody
	phaseThen
	phaseElse
	phaseFunctionParams
	phaseFunctionReturns
	phaseUnion
	phaseIntersection
	phaseGenericArgs
	phaseFunctionTypeReturns
	phaseCallTypes
	phaseLocalTypes
)

// bindStep is private traversal scratch. It is neither an AST abstraction nor
// a persisted control-flow form: every step is consumed during one bind.
type bindStep struct {
	kind   stepKind
	phase  stepPhase
	mode   exprBindMode
	node   ast.PositionHolder
	index  int
	mark   int
	method bool
}

type functionFrame struct {
	fn             *ast.FunctionExpr
	visiblePending int
}

func (b *binder) push(step bindStep) {
	b.work = append(b.work, step)
}

func (b *binder) run() {
	for len(b.work) != 0 {
		index := len(b.work) - 1
		step := b.work[index]
		b.work = b.work[:index]
		b.execute(step)
	}
}

func (b *binder) execute(step bindStep) {
	switch step.kind {
	case stepStmtList:
		b.visitStmtList(step)
	case stepStmt:
		b.visitStmt(step)
	case stepExprList:
		b.visitExprList(step)
	case stepExpr:
		b.visitExpr(step.node.(ast.Expr), step.mode)
	case stepDirectGlobalCall:
		b.recordDirectGlobalCall(step.node.(*ast.FuncCallExpr))
	case stepTypeList:
		b.visitTypeList(step)
	case stepType:
		b.visitType(step.node.(ast.TypeExpr))
	case stepLValue:
		b.visitLValue(step.node.(ast.Expr), step.mode)
	case stepLeaveScope:
		b.popScope()
	case stepAssignTargets:
		b.visitAssignTargets(step)
	case stepLocalAfterTypes:
		b.beginLocal(step.node.(*ast.LocalAssignStmt), step.mode)
	case stepLocalValues:
		b.visitLocalValues(step)
	case stepLocalFinish:
		b.finishLocal(step.node.(*ast.LocalAssignStmt), step.mark)
	case stepWhileBody:
		b.enterLoopBody(step.node, step.mode)
	case stepLeaveLoop:
		b.control.leaveLoop()
	case stepIfThen:
		b.enterIfThen(step.node.(*ast.IfStmt), step.mode)
	case stepIfElse:
		b.enterIfElse(step.node.(*ast.IfStmt), step.mode)
	case stepNumberForBody:
		b.enterNumberFor(step.node.(*ast.NumberForStmt), step.mode)
	case stepGenericForBody:
		b.enterGenericFor(step.node.(*ast.GenericForStmt), step.mode)
	case stepFuncDefFunction:
		b.enterFuncDef(step.node.(*ast.FuncDefStmt), step.mode)
	case stepTableFields:
		b.visitTableFields(step)
	case stepFunctionAfterConstraints:
		b.finishFunctionEntry(step.node.(*ast.FunctionExpr), step.method, step.mode)
	case stepFunctionLeave:
		b.leaveFunction()
	case stepTypeScopeLeave:
		b.popTypeScope()
	case stepTypeDefAfterConstraints:
		b.finishTypeDef(step.node.(*ast.TypeDefStmt))
	case stepFunctionTypeAfterConstraints:
		b.finishFunctionType(step.node.(*ast.FunctionTypeExpr))
	case stepRecordFields:
		b.visitRecordFields(step)
	case stepTypeParamConstraints:
		b.visitTypeParamConstraints(step)
	case stepFunctionParamTypes:
		b.visitFunctionParamTypes(step)
	case stepInterfaceMembers:
		b.visitInterfaceMembers(step)
	}
}

func (b *binder) scheduleStmtList(node ast.PositionHolder, phase stepPhase, mode exprBindMode) {
	b.push(bindStep{kind: stepStmtList, node: node, phase: phase, mode: mode, index: -1})
}

func (b *binder) scheduleExpr(expr ast.Expr, mode exprBindMode) {
	if expr == nil {
		return
	}
	b.push(bindStep{kind: stepExpr, node: expr, mode: mode})
}

func (b *binder) scheduleType(expr ast.TypeExpr) {
	if expr == nil {
		return
	}
	b.push(bindStep{kind: stepType, node: expr})
}

func (b *binder) scheduleLValue(expr ast.Expr, mode exprBindMode) {
	if expr == nil {
		return
	}
	b.push(bindStep{kind: stepLValue, node: expr, mode: mode})
}
