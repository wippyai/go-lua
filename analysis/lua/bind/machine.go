package bind

import (
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
	ids := make([]symbol.ID, len(stmt.Names))
	pending := make(map[string]symbol.ID, len(stmt.Names))
	for i, name := range stmt.Names {
		id := b.newSymbol(name, symbol.Local)
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
	id := b.newSymbol(stmt.Name, symbol.Local)
	b.result.numForSymbols[stmt] = id
	b.control.enterLoop()
	b.push(bindStep{kind: stepLeaveLoop})
	b.pushScope()
	b.define(stmt.Name, id)
	b.push(bindStep{kind: stepLeaveScope})
	b.scheduleStmtList(stmt, phaseBody, mode)
}

func (b *binder) enterGenericFor(stmt *ast.GenericForStmt, mode exprBindMode) {
	ids := make([]symbol.ID, len(stmt.Names))
	b.control.enterLoop()
	b.push(bindStep{kind: stepLeaveLoop})
	b.pushScope()
	for i, name := range stmt.Names {
		id := b.newSymbol(name, symbol.Local)
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
		if name := receiverTypeName(stmt.Name.Receiver); name != "" {
			if decl, ok := b.lookupType(name); ok {
				details.receiverType = decl
				details.hasReceiverType = true
			}
		}
	}
	b.enterFunction(stmt.Func, method, details, mode)
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

func (b *binder) visitLValue(expr ast.Expr, mode exprBindMode) {
	switch e := expr.(type) {
	case nil:
	case *ast.IdentExpr:
		if mode == exprBindTypeQuery {
			b.bindTypeQueryWriteIdent(e)
		} else {
			b.bindWriteIdent(e)
		}
	case *ast.AttrGetExpr:
		if e.KeySyntax != ast.AttrKeyDot {
			b.scheduleExpr(e.Key, mode)
		}
		b.scheduleExpr(e.Object, mode)
	default:
		b.scheduleExpr(expr, mode)
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
		if value, ok := b.runtimeTypeCallBase(e); ok {
			b.bindRuntimeTypeValue(value)
			// The marked base is the only call component omitted from ordinary
			// binding. Arguments and explicit type arguments retain the current
			// traversal mode, so static queries keep authority without creating
			// executable read evidence.
			b.push(bindStep{kind: stepTypeList, node: e, phase: phaseCallTypes})
			b.push(bindStep{kind: stepExprList, node: e, mode: mode})
			return
		}
		b.push(bindStep{kind: stepTypeList, node: e, phase: phaseCallTypes})
		b.push(bindStep{kind: stepExprList, node: e, mode: mode})
		b.scheduleExpr(e.Receiver, mode)
		b.push(bindStep{kind: stepDirectGlobalCall, node: e})
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
		b.enterFunction(e, false, functionOriginDetails{
			kind:       FunctionOriginLiteral,
			localIndex: -1,
		}, mode)
	case *ast.CastExpr:
		b.scheduleType(e.Type)
		b.scheduleExpr(e.Expr, mode)
	case *ast.NonNilAssertExpr:
		b.scheduleExpr(e.Expr, mode)
	}
}

// recordDirectGlobalCall runs immediately after a normal call's function
// expression has been bound and before its receiver/arguments. It records
// generic syntactic/binding evidence for both runtime and static-query calls;
// containment later decides whether a literal direct require is executable.
func (b *binder) recordDirectGlobalCall(call *ast.FuncCallExpr) {
	if b == nil || b.result == nil || call == nil || call.Method != "" || call.Receiver != nil {
		return
	}
	ident, ok := call.Func.(*ast.IdentExpr)
	if !ok || ident == nil {
		return
	}
	identity, ok := b.result.GlobalIdentity(ident)
	if !ok {
		return
	}
	b.result.directGlobalCalls = append(b.result.directGlobalCalls, DirectGlobalCall{
		Call: call, Global: identity,
	})
}

func runtimeTypeMethodName(name string) bool {
	switch name {
	case "is", "kind", "name", "elem", "key", "val", "inner", "ret",
		"fields", "variants", "params", "tparams":
		return true
	default:
		return false
	}
}

// runtimeTypeCallBase recognizes the exact call shapes whose base compiles to
// OP_LOADTYPE. It intentionally does not classify a plain value, a dynamic
// member key, or an unrecognized method spelling.
func (b *binder) runtimeTypeCallBase(call *ast.FuncCallExpr) (RuntimeTypeValue, bool) {
	if b == nil || b.result == nil || call == nil {
		return RuntimeTypeValue{}, false
	}
	var base *ast.IdentExpr
	switch {
	case call.Method == "" && call.Receiver == nil:
		if ident, ok := call.Func.(*ast.IdentExpr); ok {
			base = ident
		} else if member, ok := call.Func.(*ast.AttrGetExpr); ok {
			key, keyOK := member.Key.(*ast.StringExpr)
			ident, identOK := member.Object.(*ast.IdentExpr)
			if !keyOK || !identOK ||
				(member.KeySyntax != ast.AttrKeyDot && member.KeySyntax != ast.AttrKeyIndex) {
				return RuntimeTypeValue{}, false
			}
			if !runtimeTypeMethodName(key.Value) {
				return RuntimeTypeValue{}, false
			}
			base = ident
		} else {
			return RuntimeTypeValue{}, false
		}
	case call.Func == nil:
		if !runtimeTypeMethodName(call.Method) {
			return RuntimeTypeValue{}, false
		}
		ident, ok := call.Receiver.(*ast.IdentExpr)
		if !ok {
			return RuntimeTypeValue{}, false
		}
		base = ident
	default:
		return RuntimeTypeValue{}, false
	}
	return b.runtimeTypeValueAuthority(base)
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
	case *ast.FunctionTypeExpr:
		return n.Returns
	default:
		return nil
	}
}

func (b *binder) visitType(expr ast.TypeExpr) {
	switch e := expr.(type) {
	case nil:
	case *ast.AnnotatedTypeExpr:
		b.scheduleAnnotationArgs(e.Annotations)
		b.scheduleType(e.Inner)
	case *ast.PrimitiveTypeExpr:
		b.bindPrimitiveTypeRef(e)
	case *ast.LiteralTypeExpr:
	case *ast.OptionalTypeExpr:
		b.scheduleType(e.Inner)
	case *ast.UnionTypeExpr:
		b.push(bindStep{kind: stepTypeList, node: e, phase: phaseUnion})
	case *ast.IntersectionTypeExpr:
		b.push(bindStep{kind: stepTypeList, node: e, phase: phaseIntersection})
	case *ast.ArrayTypeExpr:
		b.scheduleType(e.Element)
	case *ast.MapTypeExpr:
		b.scheduleType(e.Value)
		b.scheduleType(e.Key)
	case *ast.RecordTypeExpr:
		b.push(bindStep{kind: stepRecordFields, node: e})
	case *ast.FunctionTypeExpr:
		b.pushTypeScope()
		fnTypeParams := b.defineTypeParams(e.TypeParams)
		if len(fnTypeParams) > 0 {
			b.result.functionTypeParams[e] = fnTypeParams
		}
		b.push(bindStep{kind: stepFunctionTypeAfterConstraints, node: e})
		b.push(bindStep{kind: stepTypeParamConstraints, node: e})
	case *ast.AssertsTypeExpr:
		b.scheduleType(e.NarrowTo)
	case *ast.TypeRefExpr:
		b.bindTypeRef(e)
	case *ast.GenericTypeExpr:
		b.push(bindStep{kind: stepTypeList, node: e, phase: phaseGenericArgs})
		b.bindTypeRef(e.Base)
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

func (b *binder) enterFunction(fn *ast.FunctionExpr, method bool, origin functionOriginDetails, mode exprBindMode) {
	if fn == nil {
		return
	}
	origin.static = mode == exprBindTypeQuery
	parent := b.currentFunction()
	b.result.registerFunction(fn, parent, origin)
	oldVisible := b.visiblePending
	b.functions = append(b.functions, functionFrame{fn: fn, visiblePending: oldVisible})
	b.visiblePending = len(b.pending)
	b.control.enterFunction()
	b.pushScope()
	fnTypeParams := b.defineTypeParams(fn.TypeParams)
	if len(fnTypeParams) > 0 {
		b.result.functionTypeParams[fn] = fnTypeParams
	}
	if origin.hasReceiverType {
		b.result.methodReceiverTypes[fn] = origin.receiverType
	}
	b.push(bindStep{kind: stepFunctionAfterConstraints, node: fn, method: method, mode: mode})
	b.push(bindStep{kind: stepTypeParamConstraints, node: fn})
}

func (b *binder) finishFunctionEntry(fn *ast.FunctionExpr, method bool, mode exprBindMode) {
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
		params = append(params, id)
		b.define("self", id)
		slots = append(slots, ParamSlot{
			Symbol: id, Name: "self", SourceIndex: -1, ImplicitSelf: true,
		})
	}
	for i, name := range names {
		id := b.newSymbol(name, symbol.Param)
		position := positionAt(fn.ParList, i)
		annotation := typeAt(types, i)
		b.result.setSymbolTypeAnnotation(id, annotation)
		params = append(params, id)
		b.define(name, id)
		slots = append(slots, ParamSlot{
			Symbol: id, Name: name, Position: position, Type: annotation, SourceIndex: i,
		})
	}
	if hasVargs {
		id := b.newSymbol("...", symbol.Param)
		var position ast.Position
		if fn.ParList != nil {
			position = fn.ParList.VarargPosition
		}
		b.result.setSymbolTypeAnnotation(id, varargType)
		b.result.varargSymbols[fn] = id
		slots = append(slots, ParamSlot{
			Symbol: id, Name: "...", Position: position, Type: varargType,
			SourceIndex: len(names), Vararg: true,
		})
	}
	b.result.paramSlots[fn] = slots
	b.recordFunctionAssertedParams(fn, slots)

	b.push(bindStep{kind: stepFunctionLeave, node: fn, mode: mode})
	b.scheduleStmtList(fn, phaseBody, mode)
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

func (b *binder) beginTypeDef(stmt *ast.TypeDefStmt) {
	if stmt == nil {
		return
	}
	b.declareTypeDef(stmt)
	b.pushTypeScope()
	params := b.defineTypeParams(stmt.TypeParams)
	if len(params) > 0 {
		b.result.typeDefParams[stmt] = params
	}
	b.push(bindStep{kind: stepTypeDefAfterConstraints, node: stmt})
	b.push(bindStep{kind: stepTypeParamConstraints, node: stmt})
}

func (b *binder) finishTypeDef(stmt *ast.TypeDefStmt) {
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
	if step.index >= len(stmt.Members) {
		return
	}
	member := stmt.Members[step.index]
	step.index++
	b.push(step)
	switch member.Kind {
	case ast.InterfaceFieldMember:
		b.scheduleType(member.Type)
	case ast.InterfaceMethodMember:
		b.scheduleType(member.Type)
	}
}

func (b *binder) finishFunctionType(expr *ast.FunctionTypeExpr) {
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
