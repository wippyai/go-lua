package bind

import (
	"reflect"

	"github.com/wippyai/go-lua/analysis/symbol"
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
	stepTypeList
	stepType
	stepLValue
	stepLeaveScope
	stepAssignTargets
	stepLocalAfterTypes
	stepLocalValues
	stepLocalFinish
	stepWhileBody
	stepIfThen
	stepIfElse
	stepNumberForBody
	stepGenericForBody
	stepFuncDefFunction
	stepTableFields
	stepCallRecord
	stepFunctionAfterConstraints
	stepFunctionLeave
	stepFunctionSignatureAfterConstraints
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
	phaseTuple
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
		b.visitStmt(step.node.(ast.Stmt))
	case stepExprList:
		b.visitExprList(step)
	case stepExpr:
		b.visitExpr(step.node.(ast.Expr), step.mode)
	case stepTypeList:
		b.visitTypeList(step)
	case stepType:
		b.visitType(step.node.(ast.TypeExpr))
	case stepLValue:
		b.visitLValue(step.node.(ast.Expr))
	case stepLeaveScope:
		b.popScope()
	case stepAssignTargets:
		b.visitAssignTargets(step)
	case stepLocalAfterTypes:
		b.beginLocal(step.node.(*ast.LocalAssignStmt))
	case stepLocalValues:
		b.visitLocalValues(step)
	case stepLocalFinish:
		b.finishLocal(step.node.(*ast.LocalAssignStmt), step.mark)
	case stepWhileBody:
		b.enterBody(step.node)
	case stepIfThen:
		b.enterIfThen(step.node.(*ast.IfStmt))
	case stepIfElse:
		b.enterIfElse(step.node.(*ast.IfStmt))
	case stepNumberForBody:
		b.enterNumberFor(step.node.(*ast.NumberForStmt))
	case stepGenericForBody:
		b.enterGenericFor(step.node.(*ast.GenericForStmt))
	case stepFuncDefFunction:
		b.enterFuncDef(step.node.(*ast.FuncDefStmt))
	case stepTableFields:
		b.visitTableFields(step)
	case stepCallRecord:
		b.recordCall(step.node.(*ast.FuncCallExpr), step.mode)
	case stepFunctionAfterConstraints:
		b.finishFunctionEntry(step.node.(*ast.FunctionExpr), step.method)
	case stepFunctionLeave:
		b.leaveFunction()
	case stepFunctionSignatureAfterConstraints:
		b.finishFunctionSignature(step.node.(*ast.FunctionExpr))
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

func (b *binder) scheduleStmtList(node ast.PositionHolder, phase stepPhase) {
	if node != nil && !nodePresent(node) {
		b.invalidateRuntimeUseScan()
		return
	}
	b.push(bindStep{kind: stepStmtList, node: node, phase: phase, index: -1})
}

func (b *binder) scheduleExpr(expr ast.Expr, mode exprBindMode) {
	if expr == nil {
		return
	}
	if !nodePresent(expr) {
		b.invalidateRuntimeUseScan()
		return
	}
	b.push(bindStep{kind: stepExpr, node: expr, mode: mode})
}

func (b *binder) scheduleType(expr ast.TypeExpr) {
	if expr == nil {
		return
	}
	if !nodePresent(expr) {
		b.invalidateRuntimeUseScan()
		return
	}
	b.push(bindStep{kind: stepType, node: expr})
}

func (b *binder) scheduleLValue(expr ast.Expr) {
	if expr == nil {
		return
	}
	if !nodePresent(expr) {
		b.invalidateRuntimeUseScan()
		return
	}
	b.push(bindStep{kind: stepLValue, node: expr})
}

func (b *binder) visitStmtList(step bindStep) {
	stmts := b.statementList(step.node, step.phase)
	if step.index < 0 {
		b.hoistTypeDecls(stmts)
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
	if !nodePresent(stmt) {
		b.invalidateRuntimeUseScan()
		return
	}
	b.push(bindStep{kind: stepStmt, node: stmt})
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

func (b *binder) visitStmt(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		b.push(bindStep{kind: stepAssignTargets, node: s})
		b.push(bindStep{kind: stepExprList, node: s, mode: exprBindRuntime})
	case *ast.LocalAssignStmt:
		b.push(bindStep{kind: stepLocalAfterTypes, node: s})
		b.push(bindStep{kind: stepTypeList, node: s, phase: phaseLocalTypes})
	case *ast.FuncCallStmt:
		b.scheduleExpr(s.Expr, exprBindRuntime)
	case *ast.DoBlockStmt:
		b.pushScope()
		b.push(bindStep{kind: stepLeaveScope})
		b.scheduleStmtList(s, phaseBody)
	case *ast.WhileStmt:
		b.push(bindStep{kind: stepWhileBody, node: s})
		b.scheduleExpr(s.Condition, exprBindRuntime)
	case *ast.RepeatStmt:
		b.pushScope()
		b.push(bindStep{kind: stepLeaveScope})
		b.scheduleExpr(s.Condition, exprBindRuntime)
		b.scheduleStmtList(s, phaseBody)
	case *ast.IfStmt:
		b.push(bindStep{kind: stepIfThen, node: s})
		b.scheduleExpr(s.Condition, exprBindRuntime)
	case *ast.NumberForStmt:
		b.push(bindStep{kind: stepNumberForBody, node: s})
		b.scheduleExpr(s.Step, exprBindRuntime)
		b.scheduleExpr(s.Limit, exprBindRuntime)
		b.scheduleExpr(s.Init, exprBindRuntime)
	case *ast.GenericForStmt:
		b.push(bindStep{kind: stepGenericForBody, node: s})
		b.push(bindStep{kind: stepExprList, node: s, mode: exprBindRuntime})
	case *ast.FuncDefStmt:
		b.push(bindStep{kind: stepFuncDefFunction, node: s})
		if s.Name != nil {
			b.scheduleExpr(s.Name.Receiver, exprBindRuntime)
			b.scheduleLValue(s.Name.Func)
		}
	case *ast.ReturnStmt:
		b.push(bindStep{kind: stepExprList, node: s, mode: exprBindRuntime})
	case *ast.BreakStmt:
	case *ast.LabelStmt:
		b.control.visitLabel(s)
	case *ast.GotoStmt:
		b.control.visitGoto(s)
	case *ast.TypeDefStmt:
		b.beginTypeDef(s)
	case *ast.InterfaceDefStmt:
		b.beginInterface(s)
	default:
		b.invalidateRuntimeUseScan()
	}
}

func (b *binder) visitAssignTargets(step bindStep) {
	stmt := step.node.(*ast.AssignStmt)
	if step.index >= len(stmt.Lhs) {
		b.recordQualifiedTypeAliases(stmt)
		return
	}
	target := stmt.Lhs[step.index]
	step.index++
	b.push(step)
	b.scheduleLValue(target)
}

func (b *binder) beginLocal(stmt *ast.LocalAssignStmt) {
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
	mark := len(b.pending)
	if stmt.LocalFunction {
		mark = b.pushPending(pending)
	}
	b.push(bindStep{kind: stepLocalFinish, node: stmt, mark: mark})
	b.push(bindStep{kind: stepLocalValues, node: stmt})
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
			details.targetSymbol = b.result.localSymbols[stmt][slot]
			details.hasTargetSymbol = details.targetSymbol != 0
		}
		b.enterFunction(fn, false, details)
		return
	}
	b.scheduleExpr(expr, exprBindRuntime)
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

func (b *binder) enterBody(node ast.PositionHolder) {
	b.pushScope()
	b.push(bindStep{kind: stepLeaveScope})
	b.scheduleStmtList(node, phaseBody)
}

func (b *binder) enterIfThen(stmt *ast.IfStmt) {
	b.push(bindStep{kind: stepIfElse, node: stmt})
	b.pushScope()
	b.push(bindStep{kind: stepLeaveScope})
	b.scheduleStmtList(stmt, phaseThen)
}

func (b *binder) enterIfElse(stmt *ast.IfStmt) {
	if len(stmt.Else) == 0 {
		return
	}
	b.pushScope()
	b.push(bindStep{kind: stepLeaveScope})
	b.scheduleStmtList(stmt, phaseElse)
}

func (b *binder) enterNumberFor(stmt *ast.NumberForStmt) {
	id := b.newSymbol(stmt.Name, symbol.Local)
	b.result.setDeclaration(id, declarationForPosition(stmt.NamePosition, stmt.Name, false))
	b.result.numForSymbols[stmt] = id
	b.pushScope()
	b.define(stmt.Name, id)
	b.push(bindStep{kind: stepLeaveScope})
	b.scheduleStmtList(stmt, phaseBody)
}

func (b *binder) enterGenericFor(stmt *ast.GenericForStmt) {
	ids := make([]symbol.ID, len(stmt.Names))
	b.pushScope()
	for i, name := range stmt.Names {
		id := b.newSymbol(name, symbol.Local)
		b.result.setDeclaration(id, declarationForPosition(namePosition(stmt.NamePositions, i), name, false))
		ids[i] = id
		b.define(name, id)
	}
	b.result.genericForSymbols[stmt] = ids
	b.push(bindStep{kind: stepLeaveScope})
	b.scheduleStmtList(stmt, phaseBody)
}

func (b *binder) enterFuncDef(stmt *ast.FuncDefStmt) {
	details := functionOriginDetails{
		kind:       FunctionOriginDeclaration,
		stmt:       stmt,
		localIndex: -1,
	}
	method := stmt.Name != nil && stmt.Name.Method != ""
	if method {
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
	b.enterFunction(stmt.Func, method, details)
}

func (b *binder) visitExprList(step bindStep) {
	exprs := expressionList(step.node)
	if step.index >= len(exprs) {
		return
	}
	expr := exprs[step.index]
	step.index++
	b.push(step)
	b.scheduleExpr(expr, step.mode)
}

func expressionList(node ast.PositionHolder) []ast.Expr {
	switch n := node.(type) {
	case *ast.AssignStmt:
		return n.Rhs
	case *ast.ReturnStmt:
		return n.Exprs
	case *ast.GenericForStmt:
		return n.Exprs
	case *ast.FuncCallExpr:
		return n.Args
	default:
		return nil
	}
}

func (b *binder) visitLValue(expr ast.Expr) {
	switch e := expr.(type) {
	case nil:
	case *ast.IdentExpr:
		b.bindWriteIdent(e)
	case *ast.AttrGetExpr:
		if e.KeySyntax != ast.AttrKeyDot {
			b.scheduleExpr(e.Key, exprBindRuntime)
		}
		b.scheduleExpr(e.Object, exprBindRuntime)
	default:
		b.scheduleExpr(expr, exprBindRuntime)
	}
}

func (b *binder) visitExpr(expr ast.Expr, mode exprBindMode) {
	switch e := expr.(type) {
	case nil:
	case *ast.TrueExpr, *ast.FalseExpr, *ast.NilExpr, *ast.NumberExpr, *ast.StringExpr:
	case *ast.Comma3Expr:
		if mode == exprBindRuntime {
			b.bindVararg(e)
		}
	case *ast.IdentExpr:
		if mode == exprBindTypeQuery {
			b.bindTypeQueryIdent(e)
		} else {
			b.bindReadIdent(e)
		}
	case *ast.AttrGetExpr:
		if e.KeySyntax != ast.AttrKeyDot {
			b.scheduleExpr(e.Key, mode)
		}
		b.scheduleExpr(e.Object, mode)
	case *ast.TableExpr:
		b.push(bindStep{kind: stepTableFields, node: e, mode: mode})
	case *ast.FuncCallExpr:
		b.push(bindStep{kind: stepTypeList, node: e, phase: phaseCallTypes})
		b.push(bindStep{kind: stepExprList, node: e, mode: mode})
		b.scheduleExpr(e.Receiver, mode)
		b.push(bindStep{kind: stepCallRecord, node: e, mode: mode})
		b.scheduleExpr(e.Func, mode)
	case *ast.LogicalOpExpr:
		b.scheduleExpr(e.Rhs, mode)
		b.scheduleExpr(e.Lhs, mode)
	case *ast.RelationalOpExpr:
		b.scheduleExpr(e.Rhs, mode)
		b.scheduleExpr(e.Lhs, mode)
	case *ast.StringConcatOpExpr:
		b.scheduleExpr(e.Rhs, mode)
		b.scheduleExpr(e.Lhs, mode)
	case *ast.ArithmeticOpExpr:
		b.scheduleExpr(e.Rhs, mode)
		b.scheduleExpr(e.Lhs, mode)
	case *ast.UnaryMinusOpExpr:
		b.scheduleExpr(e.Expr, mode)
	case *ast.UnaryNotOpExpr:
		b.scheduleExpr(e.Expr, mode)
	case *ast.UnaryLenOpExpr:
		b.scheduleExpr(e.Expr, mode)
	case *ast.UnaryBNotOpExpr:
		b.scheduleExpr(e.Expr, mode)
	case *ast.FunctionExpr:
		if mode == exprBindRuntime {
			b.enterFunction(e, false, functionOriginDetails{
				kind:       FunctionOriginLiteral,
				localIndex: -1,
			})
		} else {
			b.enterFunctionSignature(e)
		}
	case *ast.CastExpr:
		b.scheduleType(e.Type)
		b.scheduleExpr(e.Expr, mode)
	case *ast.NonNilAssertExpr:
		b.scheduleExpr(e.Expr, mode)
	default:
		if mode == exprBindRuntime {
			b.invalidateRuntimeUseScan()
		}
	}
}

func (b *binder) visitTableFields(step bindStep) {
	table := step.node.(*ast.TableExpr)
	if step.index >= len(table.Fields) {
		return
	}
	field := table.Fields[step.index]
	step.index++
	b.push(step)
	if field == nil {
		return
	}
	b.scheduleExpr(field.Value, step.mode)
	if field.KeySyntax != ast.AttrKeyDot {
		b.scheduleExpr(field.Key, step.mode)
	}
}

func (b *binder) recordCall(call *ast.FuncCallExpr, mode exprBindMode) {
	if mode != exprBindRuntime || call.Method != "" || call.Receiver != nil {
		return
	}
	ident, ok := call.Func.(*ast.IdentExpr)
	if !ok {
		return
	}
	id, ok := b.result.SymbolOf(ident)
	if !ok || id == 0 {
		return
	}
	if b.result.directCalls == nil {
		b.result.directCalls = make(map[symbol.ID][]*ast.FuncCallExpr)
	}
	b.result.directCalls[id] = append(b.result.directCalls[id], call)
}

func (b *binder) visitTypeList(step bindStep) {
	types := typeList(step.node, step.phase)
	if step.index >= len(types) {
		return
	}
	expr := types[step.index]
	step.index++
	b.push(step)
	b.scheduleType(expr)
}

func typeList(node ast.PositionHolder, phase stepPhase) []ast.TypeExpr {
	switch n := node.(type) {
	case *ast.LocalAssignStmt:
		return n.Types
	case *ast.FuncCallExpr:
		return n.TypeArgs
	case *ast.FunctionExpr:
		if phase == phaseFunctionParams && n.ParList != nil {
			return n.ParList.Types
		}
		return n.ReturnTypes
	case *ast.UnionTypeExpr:
		return n.Types
	case *ast.IntersectionTypeExpr:
		return n.Types
	case *ast.GenericTypeExpr:
		return n.Args
	case *ast.TupleTypeExpr:
		return n.Elements
	case *ast.FunctionTypeExpr:
		return n.Returns
	default:
		return nil
	}
}

func (b *binder) visitType(expr ast.TypeExpr) {
	switch e := expr.(type) {
	case nil:
	case *ast.PrimitiveTypeExpr:
		b.scheduleAnnotationArgs(e.Annotations)
		b.bindPrimitiveTypeRef(e)
	case *ast.SelfTypeExpr, *ast.LiteralTypeExpr:
	case *ast.OptionalTypeExpr:
		b.scheduleType(e.Inner)
	case *ast.UnionTypeExpr:
		b.push(bindStep{kind: stepTypeList, node: e, phase: phaseUnion})
	case *ast.IntersectionTypeExpr:
		b.push(bindStep{kind: stepTypeList, node: e, phase: phaseIntersection})
	case *ast.ArrayTypeExpr:
		b.scheduleAnnotationArgs(e.ArrayAnnotations)
		b.scheduleType(e.Element)
	case *ast.MapTypeExpr:
		b.scheduleType(e.Value)
		b.scheduleType(e.Key)
	case *ast.RecordTypeExpr:
		b.push(bindStep{kind: stepRecordFields, node: e})
	case *ast.FunctionTypeExpr:
		b.push(bindStep{kind: stepFunctionTypeAfterConstraints, node: e})
		b.push(bindStep{kind: stepTypeParamConstraints, node: e})
	case *ast.AssertsTypeExpr:
		b.scheduleType(e.NarrowTo)
	case *ast.TypeRefExpr:
		b.bindTypeRef(e)
	case *ast.GenericTypeExpr:
		b.push(bindStep{kind: stepTypeList, node: e, phase: phaseGenericArgs})
		b.bindTypeRef(e.Base)
	case *ast.MetaTypeExpr:
		b.scheduleType(e.Inner)
	case *ast.TupleTypeExpr:
		b.push(bindStep{kind: stepTypeList, node: e, phase: phaseTuple})
	case *ast.TypeOfExpr:
		b.scheduleExpr(e.Expr, exprBindTypeQuery)
	case *ast.KeyOfExpr:
		b.scheduleType(e.Inner)
	case *ast.IndexAccessExpr:
		b.scheduleType(e.Index)
		b.scheduleType(e.Object)
	case *ast.ConditionalTypeExpr:
		b.scheduleType(e.Else)
		b.scheduleType(e.Then)
		b.scheduleType(e.Extends)
		b.scheduleType(e.Check)
	}
}

func (b *binder) visitRecordFields(step bindStep) {
	record := step.node.(*ast.RecordTypeExpr)
	if step.index >= len(record.Fields) {
		return
	}
	field := record.Fields[step.index]
	step.index++
	b.push(step)
	b.scheduleAnnotationArgs(field.Annotations)
	b.scheduleType(field.Type)
}

// scheduleAnnotationArgs binds validation-annotation expressions as static
// queries. Annotations are attached to type syntax, never executable syntax:
// their expressions receive lexical identities without contributing runtime
// reads, captures, or call evidence. Scheduling backwards preserves source
// argument order under the binder's LIFO work stack without recursion.
func (b *binder) scheduleAnnotationArgs(annotations []ast.AnnotationExpr) {
	for annotationIndex := len(annotations) - 1; annotationIndex >= 0; annotationIndex-- {
		args := annotations[annotationIndex].Args
		for argumentIndex := len(args) - 1; argumentIndex >= 0; argumentIndex-- {
			b.scheduleExpr(args[argumentIndex], exprBindTypeQuery)
		}
	}
}

func (b *binder) visitTypeParamConstraints(step bindStep) {
	var params []ast.TypeParamExpr
	switch node := step.node.(type) {
	case *ast.FunctionExpr:
		params = node.TypeParams
	case *ast.FunctionTypeExpr:
		params = node.TypeParams
	case *ast.TypeDefStmt:
		params = node.TypeParams
	}
	if step.index >= len(params) {
		return
	}
	param := params[step.index]
	step.index++
	b.push(step)
	b.scheduleType(param.Constraint)
}

func (b *binder) enterFunction(fn *ast.FunctionExpr, method bool, origin functionOriginDetails) {
	if fn == nil {
		return
	}
	parent := b.currentFunction()
	b.result.registerFunction(fn, parent, origin)
	oldVisible := b.visiblePending
	b.functions = append(b.functions, functionFrame{fn: fn, visiblePending: oldVisible})
	b.visiblePending = len(b.pending)
	b.control.enterFunction()
	b.pushScope()
	if origin.hasReceiverType {
		b.result.methodReceiverTypes[fn] = origin.receiverType
	}
	b.push(bindStep{kind: stepFunctionAfterConstraints, node: fn, method: method})
	b.push(bindStep{kind: stepTypeParamConstraints, node: fn})
}

func (b *binder) finishFunctionEntry(fn *ast.FunctionExpr, method bool) {
	fnTypeParams := b.defineTypeParams(fn.TypeParams)
	if len(fnTypeParams) > 0 {
		b.result.functionTypeParams[fn] = fnTypeParams
	}

	params := make([]symbol.ID, 0)
	slots := make([]ParamSlot, 0)
	var names []string
	var types []ast.TypeExpr
	var hasVargs bool
	var varargType ast.TypeExpr
	if fn.ParList != nil {
		names = fn.ParList.Names
		types = fn.ParList.Types
		hasVargs = fn.ParList.HasVargs
		varargType = fn.ParList.VarargType
	}
	if method && (len(names) == 0 || names[0] != "self") {
		id := b.newSymbol("self", symbol.Param)
		b.result.setDeclaration(id, Declaration{Synthetic: true})
		params = append(params, id)
		b.define("self", id)
		slots = append(slots, ParamSlot{
			Symbol: id, Name: "self", SourceIndex: -1, ImplicitSelf: true,
		})
	}
	for i, name := range names {
		id := b.newSymbol(name, symbol.Param)
		position := positionAt(fn.ParList, i)
		b.result.setDeclaration(id, declarationForPosition(position, name, false))
		params = append(params, id)
		b.define(name, id)
		slots = append(slots, ParamSlot{
			Symbol: id, Name: name, Position: position, Type: typeAt(types, i), SourceIndex: i,
		})
	}
	b.result.paramSymbols[fn] = params
	if hasVargs {
		id := b.newSymbol("...", symbol.Param)
		var position ast.Position
		if fn.ParList != nil {
			position = fn.ParList.VarargPosition
		}
		b.result.setDeclaration(id, declarationForPosition(position, "...", true))
		b.result.varargSymbols[fn] = id
		slots = append(slots, ParamSlot{
			Symbol: id, Name: "...", Position: position, Type: varargType,
			SourceIndex: len(names), Vararg: true,
		})
	}
	b.result.paramSlots[fn] = slots
	b.recordFunctionAssertedParams(fn, slots)

	b.push(bindStep{kind: stepFunctionLeave, node: fn})
	b.scheduleStmtList(fn, phaseBody)
	b.push(bindStep{kind: stepTypeList, node: fn, phase: phaseFunctionReturns})
	b.scheduleType(varargType)
	b.push(bindStep{kind: stepTypeList, node: fn, phase: phaseFunctionParams})
}

func (b *binder) leaveFunction() {
	b.popScope()
	b.control.leaveFunction()
	if len(b.functions) == 0 {
		return
	}
	frame := b.functions[len(b.functions)-1]
	b.functions = b.functions[:len(b.functions)-1]
	b.visiblePending = frame.visiblePending
}

func (b *binder) enterFunctionSignature(fn *ast.FunctionExpr) {
	if fn == nil {
		return
	}
	b.push(bindStep{kind: stepFunctionSignatureAfterConstraints, node: fn})
	b.push(bindStep{kind: stepTypeParamConstraints, node: fn})
}

func (b *binder) finishFunctionSignature(fn *ast.FunctionExpr) {
	// A function literal appearing in a type query has a source-only
	// signature.  Its body is intentionally never entered, but the signature
	// still has the same formal-name visibility as an executable function once
	// its type-parameter constraints have been checked.  Keep this scope out of
	// b.functions: that table is runtime function-origin evidence.
	b.pushScope()
	fnTypeParams := b.defineTypeParams(fn.TypeParams)
	if len(fnTypeParams) > 0 {
		b.result.functionTypeParams[fn] = fnTypeParams
	}

	params := make([]symbol.ID, 0)
	slots := make([]ParamSlot, 0)
	if fn.ParList != nil {
		for i, name := range fn.ParList.Names {
			// Do not use b.newSymbol here: static-signature declarations must
			// not be attributed to any enclosing runtime function.
			id := b.result.newSymbol(name, symbol.Param)
			position := positionAt(fn.ParList, i)
			annotation := typeAt(fn.ParList.Types, i)
			b.result.setDeclaration(id, declarationForPosition(position, name, false))
			if annotation != nil {
				b.result.symbolAnnotations[id] = annotation
			}
			params = append(params, id)
			b.define(name, id)
			slots = append(slots, ParamSlot{
				Symbol: id, Name: name, Position: position,
				Type: annotation, SourceIndex: i,
			})
		}
		if fn.ParList.HasVargs {
			id := b.result.newSymbol("...", symbol.Param)
			b.result.setDeclaration(id, declarationForPosition(fn.ParList.VarargPosition, "...", true))
			if fn.ParList.VarargType != nil {
				b.result.symbolAnnotations[id] = fn.ParList.VarargType
			}
			b.result.varargSymbols[fn] = id
			b.define("...", id)
			slots = append(slots, ParamSlot{
				Symbol: id, Name: "...", Position: fn.ParList.VarargPosition,
				Type: fn.ParList.VarargType, SourceIndex: len(fn.ParList.Names), Vararg: true,
			})
		}
	}
	b.result.paramSymbols[fn] = params
	b.result.paramSlots[fn] = slots
	b.recordFunctionAssertedParams(fn, slots)

	b.push(bindStep{kind: stepLeaveScope})
	b.push(bindStep{kind: stepTypeList, node: fn, phase: phaseFunctionReturns})
	if fn.ParList != nil {
		b.scheduleType(fn.ParList.VarargType)
	}
	b.push(bindStep{kind: stepTypeList, node: fn, phase: phaseFunctionParams})
}

func (b *binder) beginTypeDef(stmt *ast.TypeDefStmt) {
	if stmt == nil {
		return
	}
	b.declareTypeDef(stmt)
	b.push(bindStep{kind: stepTypeDefAfterConstraints, node: stmt})
	b.push(bindStep{kind: stepTypeParamConstraints, node: stmt})
}

func (b *binder) finishTypeDef(stmt *ast.TypeDefStmt) {
	b.pushTypeScope()
	params := b.defineTypeParams(stmt.TypeParams)
	if len(params) > 0 {
		b.result.typeDefParams[stmt] = params
	}
	b.push(bindStep{kind: stepTypeScopeLeave})
	b.scheduleType(stmt.Type)
}

func (b *binder) beginInterface(stmt *ast.InterfaceDefStmt) {
	if stmt == nil {
		return
	}
	b.declareInterfaceDef(stmt)
	for _, ref := range stmt.Extends {
		b.bindTypeRef(ref)
	}
	b.push(bindStep{kind: stepInterfaceMembers, node: stmt})
}

func (b *binder) visitInterfaceMembers(step bindStep) {
	stmt := step.node.(*ast.InterfaceDefStmt)
	if step.phase == 0 {
		if step.index < len(stmt.Fields) {
			field := stmt.Fields[step.index]
			step.index++
			b.push(step)
			b.scheduleType(field.Type)
			return
		}
		step.phase = 1
		step.index = 0
	}
	if step.index >= len(stmt.Methods) {
		return
	}
	method := stmt.Methods[step.index]
	step.index++
	b.push(step)
	b.scheduleType(method.Type)
}

func (b *binder) finishFunctionType(expr *ast.FunctionTypeExpr) {
	b.pushTypeScope()
	fnTypeParams := b.defineTypeParams(expr.TypeParams)
	if len(fnTypeParams) > 0 {
		b.result.functionTypeParams[expr] = fnTypeParams
	}
	b.recordFunctionTypeAssertedParams(expr)
	b.push(bindStep{kind: stepTypeScopeLeave})
	b.push(bindStep{kind: stepTypeList, node: expr, phase: phaseFunctionTypeReturns})
	b.scheduleType(expr.Variadic)
	b.push(bindStep{kind: stepFunctionParamTypes, node: expr})
}

func (b *binder) recordFunctionAssertedParams(fn *ast.FunctionExpr, slots []ParamSlot) {
	if fn == nil {
		return
	}
	for _, returnType := range fn.ReturnTypes {
		assertion, ok := returnType.(*ast.AssertsTypeExpr)
		if !ok {
			continue
		}
		for ordinal := len(slots) - 1; ordinal >= 0; ordinal-- {
			if slots[ordinal].Name == assertion.ParamName {
				b.recordAssertedParam(assertion, ordinal)
				break
			}
		}
	}
}

func (b *binder) recordFunctionTypeAssertedParams(fn *ast.FunctionTypeExpr) {
	if fn == nil {
		return
	}
	for _, returnType := range fn.Returns {
		assertion, ok := returnType.(*ast.AssertsTypeExpr)
		if !ok {
			continue
		}
		for ordinal := len(fn.Params) - 1; ordinal >= 0; ordinal-- {
			if fn.Params[ordinal].Name == assertion.ParamName {
				b.recordAssertedParam(assertion, ordinal)
				break
			}
		}
	}
}

func (b *binder) recordAssertedParam(assertion *ast.AssertsTypeExpr, ordinal int) {
	if assertion == nil || ordinal < 0 {
		return
	}
	if b.result.assertedParams == nil {
		b.result.assertedParams = make(map[*ast.AssertsTypeExpr]int)
	}
	b.result.assertedParams[assertion] = ordinal
}

func (b *binder) visitFunctionParamTypes(step bindStep) {
	expr := step.node.(*ast.FunctionTypeExpr)
	if step.index >= len(expr.Params) {
		return
	}
	param := expr.Params[step.index]
	step.index++
	b.push(step)
	b.scheduleType(param.Type)
}

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

func nodePresent(node ast.PositionHolder) bool {
	if node == nil {
		return false
	}
	value := reflect.ValueOf(node)
	return value.Kind() != reflect.Pointer || !value.IsNil()
}

func (b *binder) invalidateRuntimeUseScan() {
	if b != nil && b.result != nil {
		b.result.runtimeUseScanComplete = false
	}
}
