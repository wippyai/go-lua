package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (l *lowerer) addAssignmentWritesFromWIR(input *factflow.FactsInput, point cfg.Point) {
	if l == nil || l.wir == nil || input == nil {
		return
	}
	for _, inst := range l.wir.PointInstructions(point) {
		switch inst.Op {
		case wir.OpStaticMemberWrite:
			l.addStaticMemberWriteFromWIR(input, point, inst)
		case wir.OpDynamicIndexWrite:
			l.addDynamicIndexWriteFromWIR(input, point, inst)
		case wir.OpIterate:
			l.addNumericForRootAssignmentFromWIR(input, point, inst)
		default:
			l.addRootAssignmentFromWIR(input, point, inst)
		}
	}
}

func (l *lowerer) addNumericForRootAssignmentFromWIR(input *factflow.FactsInput, point cfg.Point, inst wir.Instruction) {
	if l == nil || input == nil || l.wir == nil || inst.Iter != wir.IterNumeric {
		return
	}
	results := l.wir.Operands(inst.Results)
	if len(results) == 0 {
		return
	}
	target, ok := l.wirAssignmentPath(results[0])
	if !ok || target.Symbol == 0 || len(target.Segments) != 0 {
		return
	}
	declared, ok := l.symbolTypes[target.Symbol]
	if !ok || declared == nil {
		declared = l.numericForLoopVariableTypeFromWIR(inst)
	}
	if declared == nil {
		return
	}
	source := factflow.NewUnknownValueSource(factflow.NoValueSourceIndex)
	input.RootAssignments[point] = factflow.NewRootAssignmentWithDeclaredContractValue(
		factflow.RootAssignmentLocalDeclaration,
		target.Symbol,
		target,
		source,
		l.valueFromTypeWithWitness(declared),
	)
}

func (l *lowerer) numericForLoopVariableTypeFromWIR(inst wir.Instruction) typ.Type {
	if l == nil || l.wir == nil || inst.Iter != wir.IterNumeric {
		return nil
	}
	bounds := l.wir.Operands(inst.List)
	if len(bounds) == 0 {
		return typ.Number
	}
	for _, bound := range bounds {
		if _, ok := l.numericForIntegralLiteralFromWIR(bound); !ok {
			return typ.Number
		}
	}
	return typ.Integer
}

func (l *lowerer) addRootAssignmentFromWIR(input *factflow.FactsInput, point cfg.Point, inst wir.Instruction) {
	kind, ok := rootAssignmentKindFromWIR(inst.Assign)
	if !ok {
		return
	}
	target, ok := l.wirAssignmentPath(inst.Dst)
	if !ok || len(target.Segments) != 0 {
		return
	}
	source, ok := l.rootAssignmentValueSourceFromWIR(point, inst)
	if !ok {
		return
	}
	assignment := factflow.NewRootAssignment(kind, target.Symbol, target, source)
	if kind == factflow.RootAssignmentLocalDeclaration {
		if declared, ok := l.wirExplicitTopDeclaredContract(inst); ok {
			assignment = factflow.NewRootAssignmentWithDeclaredContractValue(kind, target.Symbol, target, source, l.declaredTypeClaimValue(declared))
		} else if declared, ok := l.wirSymbolExplicitTopDeclaredContract(target.Symbol); ok {
			assignment = factflow.NewRootAssignmentWithDeclaredContractValue(kind, target.Symbol, target, source, l.declaredTypeClaimValue(declared))
		} else if declared, ok := l.wirInstructionDeclaredType(inst); ok {
			assignment = factflow.NewRootAssignmentWithDeclaredContractValue(kind, target.Symbol, target, source, l.declaredTypeClaimValue(declared))
		} else if declared, ok := l.wirSymbolDeclaredContract(target.Symbol, source); ok {
			assignment = factflow.NewRootAssignmentWithDeclaredContractValue(kind, target.Symbol, target, source, l.declaredTypeClaimValue(declared))
		} else if declared, ok := l.declaredReturnLocalContractForSymbol(target.Symbol); ok {
			assignment = factflow.NewRootAssignmentWithDeclaredContractValue(kind, target.Symbol, target, source, l.declaredTypeClaimValue(declared))
		} else if declared, ok := l.returnLocalObjectLiteralContractForSymbol(target.Symbol); ok && recordWithCallableField(declared) {
			assignment = factflow.NewRootAssignmentWithDeclaredContractValue(kind, target.Symbol, target, source, l.declaredTypeClaimValue(declared))
		} else if declared, ok := l.wirAssignmentDeclaredObjectType(inst, target.Symbol); ok {
			assignment = factflow.NewRootAssignmentWithDeclaredOverlayValue(kind, target.Symbol, target, source, l.declaredTypeClaimValue(declared))
		}
	}
	input.RootAssignments[point] = assignment
	if declared, ok := l.wirAssignmentDeclaredObjectType(inst, target.Symbol); ok {
		l.addObjectLiteralFieldExposuresFromWIR(input, point, inst, declared)
	}
}

func (l *lowerer) wirInstructionDeclaredType(inst wir.Instruction) (typ.Type, bool) {
	if l == nil || l.wir == nil || inst.Type == 0 {
		return nil, false
	}
	if inst.Op == wir.OpClaim {
		return nil, false
	}
	declared := l.wir.Type(inst.Type)
	if inst.Op == wir.OpMakeTable &&
		declared != nil &&
		luatypeprojection.ReachesTableContract(declared) &&
		!reachesArray(declared) &&
		!recordWithCallableField(declared) {
		return nil, false
	}
	return declared, declared != nil
}

func (l *lowerer) wirSymbolDeclaredContract(target symbol.ID, source factflow.ValueSource) (typ.Type, bool) {
	if l == nil || target == 0 {
		return nil, false
	}
	if source.Kind != factflow.ValueSourceCall && source.Kind != factflow.ValueSourceUnknown {
		return nil, false
	}
	declared, ok := l.symbolTypes[target]
	if !ok || declared == nil || typ.IsAny(declared) || typ.IsUnknown(declared) || luatypeprojection.ReachesTableContract(declared) {
		return nil, false
	}
	return declared, true
}

func (l *lowerer) wirAssignmentDeclaredObjectType(inst wir.Instruction, target symbol.ID) (typ.Type, bool) {
	if l == nil {
		return nil, false
	}
	if l.wir != nil && inst.Type != 0 {
		if declared := l.wir.Type(inst.Type); declared != nil {
			return declared, luatypeprojection.ReachesTableContract(declared) && !reachesArray(declared) && !recordWithCallableField(declared)
		}
	}
	declared, ok := l.symbolTypes[target]
	return declared, ok && declared != nil && luatypeprojection.ReachesTableContract(declared) && !reachesArray(declared) && !recordWithCallableField(declared)
}

func (l *lowerer) declaredTypeClaimValue(t typ.Type) product.Value {
	return product.Set(l.registry, l.valueFromTypeWithWitness(t), assertion.Key, assertion.Type())
}

func (l *lowerer) wirExplicitTopDeclaredContract(inst wir.Instruction) (typ.Type, bool) {
	if l == nil {
		return nil, false
	}
	if l.wir == nil || inst.Type == 0 {
		return nil, false
	}
	if inst.Op == wir.OpClaim {
		return nil, false
	}
	declared := l.wir.Type(inst.Type)
	if declared == nil || (!typ.IsAny(declared) && !typ.IsUnknown(declared)) {
		return nil, false
	}
	return declared, true
}

func (l *lowerer) wirSymbolExplicitTopDeclaredContract(target symbol.ID) (typ.Type, bool) {
	if l == nil || target == 0 {
		return nil, false
	}
	declared, ok := l.explicitTopLocalTypes[target]
	if !ok || declared == nil {
		return nil, false
	}
	return declared, true
}

func (l *lowerer) rootAssignmentValueSourceFromWIR(point cfg.Point, inst wir.Instruction) (factflow.ValueSource, bool) {
	if sourceOp, ok := inst.AssignmentSourceOperand(); ok {
		return l.assignmentValueSourceFromWIROperand(point, sourceOp)
	}
	switch inst.Op {
	case wir.OpAssign:
		return factflow.NewUnknownValueSource(factflow.NoValueSourceIndex), true
	case wir.OpClosure:
		return l.wirClosureExpressionValueSource(
			inst,
			sourceprovenance.NoSourceIndex,
			0,
			true,
			false,
			false,
		)
	case wir.OpDynamicIndexRead:
		return l.wirDynamicIndexReadExpressionValueSource(
			inst,
			sourceprovenance.NoSourceIndex,
			0,
			true,
			false,
			false,
			nil,
		)
	case wir.OpConcat:
		exprRef, ok := l.exprRef(wirAssignmentProducerExprRefKey{point: point, op: inst.Op})
		if !ok {
			return factflow.ValueSource{}, false
		}
		return l.wirConcatTempExpressionValueSourceWithRef(
			0,
			inst,
			exprRef,
			sourceprovenance.NoSourceIndex,
			0,
			true,
			false,
			false,
			nil,
		)
	case wir.OpBinOp, wir.OpLogical:
		exprRef, ok := l.exprRef(wirAssignmentProducerExprRefKey{point: point, op: inst.Op})
		if !ok {
			return factflow.ValueSource{}, false
		}
		return l.wirBinaryTempExpressionValueSourceWithRef(
			0,
			inst,
			exprRef,
			sourceprovenance.NoSourceIndex,
			0,
			true,
			false,
			false,
			nil,
		)
	case wir.OpUnOp:
		exprRef, ok := l.exprRef(wirAssignmentProducerExprRefKey{point: point, op: inst.Op})
		if !ok {
			return factflow.ValueSource{}, false
		}
		return l.wirUnaryTempExpressionValueSourceWithRef(
			inst,
			exprRef,
			sourceprovenance.NoSourceIndex,
			0,
			true,
			false,
			false,
			nil,
		)
	case wir.OpClaim:
		inner, innerOK := l.wirClaimInnerValueSource(
			inst,
			sourceprovenance.NoSourceIndex,
			0,
			true,
			false,
			false,
			nil,
		)
		l.addWIRCallResultExprRefFromID(&inner, inst.A)
		if innerOK && inner.Kind == factflow.ValueSourceCall && inner.HasExpr {
			if result, ok := l.resultValueSourcesByTempFromWIR()[inst.A.Ref]; ok && result.exprID != 0 {
				if exprRef, ok := l.exprRef(wirCallResultSlotExprRefKey{id: result.exprID, resultIndex: result.resultIndex}); ok {
					inner.ExprRef = exprRef
				}
			}
			rawInner := inner
			rawInner.ExprRef = 0
			rawInner.HasExpr = false
			l.recordExpressionRefinementFromWIRClaim(inner, rawInner, inst)
			return inner, true
		}
		exprRef, ok := l.exprRef(wirAssignmentProducerExprRefKey{point: point, op: inst.Op})
		if !ok {
			return factflow.ValueSource{}, false
		}
		return l.wirClaimTempExpressionValueSourceWithRef(
			inst,
			exprRef,
			sourceprovenance.NoSourceIndex,
			0,
			true,
			false,
			false,
			nil,
		)
	case wir.OpMakeTable:
		return l.wirTableExpressionValueSource(
			inst,
			sourceprovenance.NoSourceIndex,
			0,
			true,
			false,
			false,
		)
	case wir.OpSelect:
		shape, ok := factflow.NewValueSourceShape(true, false, false, false)
		if !ok {
			return factflow.ValueSource{}, false
		}
		return factflow.NewCallValueSource(0, sourceprovenance.NoSourceIndex, 0, 0, point, shape)
	default:
		return factflow.ValueSource{}, false
	}
}

type wirAssignmentProducerExprRefKey struct {
	point cfg.Point
	op    wir.Op
}

type wirCallResultSlotExprRefKey struct {
	id          wir.ExpressionID
	resultIndex int
}

func rootAssignmentKindFromWIR(kind wir.AssignKind) (factflow.RootAssignmentKind, bool) {
	switch kind {
	case wir.AssignLocalDeclaration:
		return factflow.RootAssignmentLocalDeclaration, true
	case wir.AssignOrdinaryRootWrite:
		return factflow.RootAssignmentOrdinaryRootWrite, true
	default:
		return 0, false
	}
}

func (l *lowerer) addStaticMemberWriteFromWIR(input *factflow.FactsInput, point cfg.Point, inst wir.Instruction) {
	target, ok := l.wirAssignmentPath(inst.Dst)
	if !ok || len(target.Segments) == 0 {
		return
	}
	source, ok := l.assignmentValueSourceFromWIROperand(point, inst.A)
	if !ok {
		return
	}
	input.PathAssignments[point] = factflow.NewPathAssignment(target, source)
	input.PathStaticMemberWrites[point] = factflow.NewPathStaticMemberWrite(target, source)
	if targetSymbol, targetPath, ok := l.globalTableFieldRootTargetPath(target); ok {
		input.RootAssignments[point] = factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, targetSymbol, targetPath, source)
	}
}

func (l *lowerer) addDynamicIndexWriteFromWIR(input *factflow.FactsInput, point cfg.Point, inst wir.Instruction) {
	tablePath, ok := l.wirAssignmentPath(inst.Dst)
	if !ok {
		return
	}
	source, ok := l.assignmentValueSourceFromWIROperand(point, inst.B)
	if !ok {
		return
	}
	keySource, readKey := l.assignmentValueSourceFromWIROperand(point, inst.A)
	if !readKey {
		keySource = factflow.NewUnknownValueSource(factflow.NoValueSourceIndex)
	}
	write := factflow.NewDynamicIndexWrite(
		tablePath,
		keySource,
		source,
		dynamicindex.AdmissionUnknown,
		dynamicIndexReadbackIntent(readKey, true),
	)
	if keyPath, ok := l.wirAssignmentPath(inst.A); ok {
		write = write.WithKeyPath(keyPath)
	}
	if valuePath, ok := l.wirAssignmentPath(inst.B); ok {
		write = write.WithValuePath(valuePath)
	}
	input.DynamicIndexWrites[point] = write
	input.PathDescendantInvalidations[point] = factflow.NewPathDescendantInvalidation(tablePath).
		WithDynamicTarget(tablePath, keySource, l.wir.Segments(inst.DynamicSuffix))
}

func (l *lowerer) wirAssignmentPath(op wir.Operand) (path.Path, bool) {
	if l == nil || l.wir == nil || op.Kind != wir.OperandPath {
		return path.Path{}, false
	}
	p := l.wir.Path(wir.PathRef(op.Ref))
	if p.IsEmpty() || p.Symbol == 0 {
		return path.Path{}, false
	}
	return p.Clone(), true
}

func (l *lowerer) globalTableFieldRootTargetPath(target path.Path) (symbol.ID, path.Path, bool) {
	if l == nil || l.bindings == nil || target.Symbol == 0 {
		return 0, path.Path{}, false
	}
	if l.bindings.Name(target.Symbol) != "_G" {
		return 0, path.Path{}, false
	}
	kind, ok := l.bindings.Kind(target.Symbol)
	if !ok || kind != symbol.Global {
		return 0, path.Path{}, false
	}
	name, ok := target.DirectFieldName()
	if !ok {
		return 0, path.Path{}, false
	}
	global, ok := l.bindings.GlobalSymbol(name)
	if !ok || global == 0 {
		return 0, path.Path{}, false
	}
	return global, path.NewPath(global, name), true
}

func (l *lowerer) assignmentValueSourceFromWIROperand(point cfg.Point, op wir.Operand) (factflow.ValueSource, bool) {
	if source, ok := l.valueSourceFromWIRRootPathOperand(
		op,
		sourceprovenance.NoSourceIndex,
		0,
		true,
		symbol.Local,
		symbol.Param,
		symbol.Global,
		symbol.Upvalue,
	); ok {
		return source, true
	}
	if source, ok := l.pathExpressionSourceFromWIR(
		"assignment-write",
		point,
		op,
		sourceprovenance.NoSourceIndex,
		0,
		true,
		false,
		false,
		symbol.Local,
		symbol.Param,
		symbol.Global,
		symbol.Upvalue,
	); ok {
		return source, true
	}
	source, ok := l.valueSourceFromWIROperand(
		op,
		sourceprovenance.NoSourceIndex,
		0,
		true,
		false,
		false,
	)
	if !ok {
		return factflow.ValueSource{}, false
	}
	l.addWIRCallResultExprRefFromID(&source, op)
	return source, true
}

func (l *lowerer) addWIRCallResultExprRefFromID(source *factflow.ValueSource, op wir.Operand) {
	if l == nil || source == nil || source.Kind != factflow.ValueSourceCall || source.HasExpr || op.Kind != wir.OperandTemp {
		return
	}
	result, ok := l.resultValueSourcesByTempFromWIR()[op.Ref]
	if !ok || result.exprID == 0 {
		return
	}
	exprRef, ok := l.exprRef(wirCallExprRefKey{id: result.exprID})
	if !ok {
		return
	}
	source.ExprRef = exprRef
	source.HasExpr = true
}

func (l *lowerer) assignmentSource(point cfg.Point, fallback sourceprovenance.ASTSource) factflow.ValueSource {
	if source, ok := l.assignmentSourceFromWIR(point, fallback); ok {
		return source
	}
	if l != nil && l.wir != nil {
		return factflow.NewUnknownValueSource(fallback.TargetIndex)
	}
	return l.valueSource(fallback)
}

func (l *lowerer) assignmentSourceFromWIR(point cfg.Point, fallback sourceprovenance.ASTSource) (factflow.ValueSource, bool) {
	if l == nil || l.wir == nil {
		return factflow.ValueSource{}, false
	}
	shape := valueSourceShapeFromASTSource(fallback)
	for _, inst := range l.wir.PointInstructions(point) {
		if source, ok := l.assignmentProducerSourceFromWIR(inst, shape); ok {
			return source, true
		}
	}
	op, ok := l.assignmentSourceOperandFromWIR(point)
	if !ok {
		return factflow.ValueSource{}, false
	}
	if source, ok := l.localRootPathExpressionSourceFromWIR(
		"assignment-source",
		point,
		op,
		shape.exprIndex,
		shape.targetIndex,
		shape.final,
		shape.expanded,
		shape.openTail,
	); ok {
		return source, true
	}
	if source, ok := l.pathExpressionSourceFromWIR(
		"assignment-source",
		point,
		op,
		shape.exprIndex,
		shape.targetIndex,
		shape.final,
		shape.expanded,
		shape.openTail,
		symbol.Local,
		symbol.Param,
		symbol.Global,
		symbol.Upvalue,
	); ok {
		return source, true
	}
	if op.Kind != wir.OperandConst && op.Kind != wir.OperandTemp {
		return factflow.ValueSource{}, false
	}
	source, ok := l.valueSourceFromWIROperand(
		op,
		shape.exprIndex,
		shape.targetIndex,
		shape.final,
		shape.expanded,
		shape.openTail,
	)
	if !ok {
		return factflow.ValueSource{}, false
	}
	if source.Kind == factflow.ValueSourceCall {
		l.addWIRCallResultExprRef(&source, op, fallback)
	}
	return source, true
}

func (l *lowerer) assignmentProducerSourceFromWIR(inst wir.Instruction, shape valueSourceShape) (factflow.ValueSource, bool) {
	if !assignmentProducerOwnsAssignment(inst) {
		return factflow.ValueSource{}, false
	}
	switch inst.Op {
	case wir.OpClosure:
		return l.wirClosureExpressionValueSource(
			inst,
			shape.exprIndex,
			shape.targetIndex,
			shape.final,
			shape.expanded,
			shape.openTail,
		)
	case wir.OpMakeTable:
		return l.wirTableExpressionValueSource(
			inst,
			shape.exprIndex,
			shape.targetIndex,
			shape.final,
			shape.expanded,
			shape.openTail,
		)
	default:
		return factflow.ValueSource{}, false
	}
}

func assignmentProducerOwnsAssignment(inst wir.Instruction) bool {
	return inst.Assign != wir.AssignNone && inst.WritesAssignmentPoint()
}

func valueSourceShapeFromASTSource(source sourceprovenance.ASTSource) valueSourceShape {
	return valueSourceShape{
		exprIndex:   source.ExprIndex,
		targetIndex: source.TargetIndex,
		final:       source.Final,
		expanded:    source.Expanded,
		openTail:    source.OpenTail,
	}
}

func (l *lowerer) addWIRCallResultExprRef(source *factflow.ValueSource, op wir.Operand, fallback sourceprovenance.ASTSource) {
	if l == nil || source == nil || op.Kind != wir.OperandTemp {
		return
	}
	if _, ok := claimKindForAssertionSource(fallback.Expr); ok {
		return
	}
	result, ok := l.resultValueSourcesByTempFromWIR()[op.Ref]
	if !ok || result.exprID == 0 {
		return
	}
	exprRef, ok := l.exprRef(wirCallExprRefKey{id: result.exprID})
	if !ok {
		return
	}
	source.ExprRef = exprRef
	source.HasExpr = true
}

func (l *lowerer) ordinaryAssignmentSource(point cfg.Point, fallback sourceprovenance.ASTSource) factflow.ValueSource {
	if source, ok := l.ordinaryAssignmentSourceFromWIR(point, fallback); ok {
		return source
	}
	return l.assignmentSource(point, fallback)
}

func (l *lowerer) ordinaryAssignmentSourceFromWIR(point cfg.Point, fallback sourceprovenance.ASTSource) (factflow.ValueSource, bool) {
	if l == nil || l.wir == nil {
		return factflow.ValueSource{}, false
	}
	op, ok := l.assignmentSourceOperandFromWIR(point)
	if !ok {
		return factflow.ValueSource{}, false
	}
	return l.valueSourceFromWIRRootPathOperand(
		op,
		fallback.ExprIndex,
		fallback.TargetIndex,
		fallback.Final,
		symbol.Local,
		symbol.Param,
	)
}

func (l *lowerer) assignmentSourceOperandFromWIR(point cfg.Point) (wir.Operand, bool) {
	for _, inst := range l.wir.PointInstructions(point) {
		if op, ok := inst.AssignmentSourceOperand(); ok {
			return op, true
		}
	}
	return wir.Operand{}, false
}
