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
	if input == nil {
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
	if input == nil || inst.Iter != wir.IterNumeric {
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
	return numericForLoopVariableTypeFromWIR(l.wir, l.symbolTypes, l.wirTempDefs(), inst)
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
		} else if declared, ok := l.wirInstructionDeclaredType(inst); ok {
			assignment = factflow.NewRootAssignmentWithDeclaredContractValue(kind, target.Symbol, target, source, l.declaredTypeClaimValue(declared))
		} else if declared, ok := l.wirSymbolDeclaredContract(target.Symbol, source); ok {
			assignment = factflow.NewRootAssignmentWithDeclaredContractValue(kind, target.Symbol, target, source, l.declaredTypeClaimValue(declared))
		} else if inst.Type != 0 {
			if declared, ok := l.wirAssignmentDeclaredObjectType(inst, target.Symbol); ok {
				assignment = factflow.NewRootAssignmentWithDeclaredOverlayValue(kind, target.Symbol, target, source, l.declaredTypeClaimValue(declared))
			}
		}
		if assignment.DeclaredValueContracts() || assignment.DeclaredValueOverlays() {
			// The explicit annotation branches above have already selected the
			// assignment contract form.
		} else if declared, ok := l.declaredReturnLocalContractForSymbol(target.Symbol); ok {
			assignment = factflow.NewRootAssignmentWithDeclaredContractValue(kind, target.Symbol, target, source, l.declaredTypeClaimValue(declared))
		} else if declared, ok := l.wirAssignmentDeclaredObjectType(inst, target.Symbol); ok {
			assignment = factflow.NewRootAssignmentWithDeclaredOverlayValue(kind, target.Symbol, target, source, l.declaredTypeClaimValue(declared))
		}
		if declared, ok := l.wirAssignmentAnnotationType(inst); ok {
			assignment = assignment.WithDeclaredAnnotationValue(l.declaredTypeClaimValue(declared))
		} else if declaredValue, ok := assignment.DeclaredValue(); ok {
			assignment = assignment.WithDeclaredAnnotationValue(declaredValue)
		}
	}
	if inst.TargetSpan.Valid() {
		assignment = assignment.WithTargetSpan(sourceSpanFromWIR(inst.TargetSpan))
	}
	input.RootAssignments[point] = assignment
	l.addCastExposureFromWIR(input, point, inst)
	l.addRootAssignmentConditionAliasFromWIR(assignment)
	l.addRootAssignmentExposureFromWIR(input, point, assignment)
	l.addRootAssignmentObjectLiteralExpectedTypeFromWIR(input, assignment)
	if declared, ok := l.wirAssignmentObjectLiteralExpectedType(inst, target.Symbol); ok {
		l.addObjectLiteralFieldExposuresFromWIR(input, point, inst, declared)
	}
}

func (l *lowerer) wirAssignmentAnnotationType(inst wir.Instruction) (typ.Type, bool) {
	if l == nil || inst.Type == 0 {
		return nil, false
	}
	declared := l.wir.Type(inst.Type)
	return declared, declared != nil
}

func (l *lowerer) addCastExposureFromWIR(input *factflow.FactsInput, point cfg.Point, inst wir.Instruction) {
	if input == nil || inst.Op != wir.OpClaim || inst.Claim != wir.ClaimCast || inst.Type == 0 {
		return
	}
	operandPath, ok := l.wirOperandPath(inst.A)
	if !ok || operandPath.Symbol == 0 {
		return
	}
	target := l.wir.Type(inst.Type)
	if target == nil || typ.IsAny(target) || typ.IsUnknown(target) {
		return
	}
	sourceType, ok := l.aliasPathType(operandPath)
	if !ok || !aliasStrictlyWidens(sourceType, target) {
		return
	}
	l.addCovariantExposureType(input, point, operandPath, target)
}

func (l *lowerer) wirOperandPath(op wir.Operand) (path.Path, bool) {
	if op.Kind != wir.OperandPath {
		return path.Path{}, false
	}
	p := l.wir.Path(wir.PathRef(op.Ref))
	return p, !p.IsEmpty()
}

func (l *lowerer) addRootAssignmentConditionAliasFromWIR(assignment factflow.RootAssignment) {
	if assignment.Kind() != factflow.RootAssignmentLocalDeclaration {
		return
	}
	l.addLocalConditionAlias(assignment.TargetSymbol(), assignment.Source())
}

func (l *lowerer) addRootAssignmentObjectLiteralExpectedTypeFromWIR(input *factflow.FactsInput, assignment factflow.RootAssignment) {
	expected, ok := l.symbolTypes[assignment.TargetSymbol()]
	if !ok {
		return
	}
	l.addObjectLiteralExpectedTypeFromValueSource(input, assignment.Source(), expected)
}

func (l *lowerer) addRootAssignmentExposureFromWIR(input *factflow.FactsInput, point cfg.Point, assignment factflow.RootAssignment) {
	if assignment.Kind() != factflow.RootAssignmentLocalDeclaration && assignment.Kind() != factflow.RootAssignmentOrdinaryRootWrite {
		return
	}
	contract, ok := l.symbolTypes[assignment.TargetSymbol()]
	if !ok {
		return
	}
	l.addAliasExposureValueSourceToContractType(input, point, assignment.Source(), contract)
}

func (l *lowerer) wirInstructionDeclaredType(inst wir.Instruction) (typ.Type, bool) {
	if inst.Type == 0 {
		return nil, false
	}
	if inst.Op == wir.OpClaim {
		return nil, false
	}
	if inst.Op == wir.OpAssign && inst.A.Kind == wir.OperandPath {
		return nil, false
	}
	if inst.Op == wir.OpAssign && inst.A.Kind == wir.OperandConst {
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
	if inst.Type != 0 {
		if declared := l.wir.Type(inst.Type); declared != nil {
			return declared, luatypeprojection.ReachesTableContract(declared) && !reachesArray(declared) && !recordWithCallableField(declared)
		}
	}
	if inst.Op == wir.OpAssign && inst.A.Kind == wir.OperandPath {
		return nil, false
	}
	declared, ok := l.symbolTypes[target]
	return declared, ok && declared != nil && luatypeprojection.ReachesTableContract(declared) && !reachesArray(declared) && !recordWithCallableField(declared)
}

func (l *lowerer) wirAssignmentObjectLiteralExpectedType(inst wir.Instruction, target symbol.ID) (typ.Type, bool) {
	if l == nil {
		return nil, false
	}
	if inst.Type != 0 {
		if declared := l.wir.Type(inst.Type); declared != nil {
			return declared, luatypeprojection.ReachesTableContract(declared)
		}
	}
	declared, ok := l.symbolTypes[target]
	return declared, ok && declared != nil && luatypeprojection.ReachesTableContract(declared)
}

func (l *lowerer) declaredTypeClaimValue(t typ.Type) product.Value {
	return product.Set(l.registry, l.valueFromTypeWithWitness(t), assertion.Key, assertion.Type())
}

func (l *lowerer) wirExplicitTopDeclaredContract(inst wir.Instruction) (typ.Type, bool) {
	if inst.Type == 0 {
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

func (l *lowerer) rootAssignmentValueSourceFromWIR(point cfg.Point, inst wir.Instruction) (factflow.ValueSource, bool) {
	if source, ok := l.wirAssignmentExpressionValueSource(
		inst,
		sourceprovenance.NoSourceIndex,
		0,
		true,
		false,
		false,
		nil,
	); ok {
		return source, true
	}
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
	assignment := factflow.NewPathAssignment(target, source)
	if inst.TargetSpan.Valid() {
		assignment = assignment.WithTargetSpan(sourceSpanFromWIR(inst.TargetSpan))
	}
	if inst.ContainerSpan.Valid() {
		assignment = assignment.WithContainerSpan(sourceSpanFromWIR(inst.ContainerSpan))
	}
	input.PathAssignments[point] = assignment
	input.PathStaticMemberWrites[point] = factflow.NewPathStaticMemberWrite(target, source)
	l.addPathStoreExposureFromWIR(input, point, target, source)
	if targetSymbol, targetPath, ok := l.globalTableFieldRootTargetPath(target); ok {
		rootAssignment := factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, targetSymbol, targetPath, source)
		if inst.TargetSpan.Valid() {
			rootAssignment = rootAssignment.WithTargetSpan(sourceSpanFromWIR(inst.TargetSpan))
		}
		input.RootAssignments[point] = rootAssignment
	}
}

func (l *lowerer) addPathStoreExposureFromWIR(input *factflow.FactsInput, point cfg.Point, target path.Path, source factflow.ValueSource) {
	if target.Symbol == 0 || len(target.Segments) == 0 {
		return
	}
	containerType, ok := l.symbolTypes[target.Symbol]
	if !ok {
		return
	}
	slotType, ok := luatypeprojection.ApplySegments(containerType, target.Segments)
	if !ok || slotType == nil {
		return
	}
	l.addAliasExposureValueSourceToContractType(input, point, source, slotType)
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
	if inst.TargetSpan.Valid() {
		write = write.WithTargetSpan(sourceSpanFromWIR(inst.TargetSpan))
	}
	if inst.ContainerSpan.Valid() {
		write = write.WithContainerSpan(sourceSpanFromWIR(inst.ContainerSpan))
	}
	if keyPath, ok := l.wirAssignmentSourcePath(inst.A); ok {
		write = write.WithKeyPath(keyPath)
	}
	if valuePath, ok := l.wirAssignmentSourcePath(inst.B); ok {
		write = write.WithValuePath(valuePath)
	}
	input.DynamicIndexWrites[point] = write
	l.addDynamicIndexObjectLiteralExpectedTypeFromWIR(input, write)
	input.PathDescendantInvalidations[point] = factflow.NewPathDescendantInvalidation(tablePath).
		WithDynamicTarget(tablePath, keySource, l.wir.Segments(inst.DynamicSuffix))
}

func (l *lowerer) wirAssignmentSourcePath(op wir.Operand) (path.Path, bool) {
	if p, ok := l.wirAssignmentPath(op); ok {
		return p, true
	}
	if op.Kind != wir.OperandTemp {
		return path.Path{}, false
	}
	def, ok := l.wirTempDefs()[op.Ref]
	if !ok || def.Op != wir.OpClaim {
		return path.Path{}, false
	}
	if def.Claim == wir.ClaimCast && def.Type != 0 {
		t := l.wir.Type(def.Type)
		if t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
			return path.Path{}, false
		}
	}
	return l.wirAssignmentSourcePath(def.A)
}

func (l *lowerer) addDynamicIndexObjectLiteralExpectedTypeFromWIR(input *factflow.FactsInput, write factflow.DynamicIndexWrite) {
	container, ok := l.aliasPathType(write.TablePathRef())
	if !ok {
		return
	}
	expected, ok := dynamicIndexMapValueType(container)
	if !ok {
		return
	}
	l.addObjectLiteralExpectedTypeFromValueSource(input, write.Source(), expected)
}

func (l *lowerer) wirAssignmentPath(op wir.Operand) (path.Path, bool) {
	if op.Kind != wir.OperandPath {
		return path.Path{}, false
	}
	p := l.wir.Path(wir.PathRef(op.Ref))
	if p.IsEmpty() || p.Symbol == 0 {
		return path.Path{}, false
	}
	return p.Clone(), true
}

func (l *lowerer) globalTableFieldRootTargetPath(target path.Path) (symbol.ID, path.Path, bool) {
	if l == nil || l.wir == nil || target.Symbol == 0 {
		return 0, path.Path{}, false
	}
	if !l.wir.SymbolResolvesToGlobal(target.Symbol, "_G") {
		return 0, path.Path{}, false
	}
	name, ok := target.DirectFieldName()
	if !ok {
		return 0, path.Path{}, false
	}
	global, ok := l.wir.GlobalSymbol(name)
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
		wir.SymbolLocal,
		wir.SymbolParam,
		wir.SymbolGlobal,
		wir.SymbolUpvalue,
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
		wir.SymbolLocal,
		wir.SymbolParam,
		wir.SymbolGlobal,
		wir.SymbolUpvalue,
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
