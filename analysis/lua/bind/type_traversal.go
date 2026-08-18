package bind

import "github.com/wippyai/go-lua/compiler/ast"

// The type lane owns static syntax traversal and function/type declaration
// phases. It shares the one binder state machine with expressions and
// statements; keeping it as a same-package lane avoids a second binder or a
// cross-package adapter for private lexical scope state.

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
		position := ast.Position{}
		if origin, ok := b.result.functionOrigins[fn]; ok {
			position = origin.MethodPosition
		}
		id := b.newSymbol("self", SymbolParam)
		b.define("self", id)
		slots = append(slots, ParamSlot{
			Symbol: id, Name: "self", Position: position, SourceIndex: -1, ImplicitSelf: true,
		})
	}
	for i, name := range names {
		id := b.newSymbol(name, SymbolParam)
		position := positionAt(fn.ParList, i)
		annotation := typeAt(types, i)
		b.result.setSymbolTypeAnnotation(id, annotation)
		b.define(name, id)
		slots = append(slots, ParamSlot{
			Symbol: id, Name: name, Position: position, Type: annotation, SourceIndex: i,
		})
	}
	if hasVargs {
		id := b.newSymbol("...", SymbolParam)
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
