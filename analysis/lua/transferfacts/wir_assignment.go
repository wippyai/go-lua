package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/expressionid"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
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
		default:
			l.addRootAssignmentFromWIR(input, point, inst)
		}
	}
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
		if declared, ok := l.declaredReturnLocalContractForSymbol(target.Symbol); ok {
			assignment = factflow.NewRootAssignmentWithDeclaredContractValue(kind, target.Symbol, target, source, l.valueFromTypeWithWitness(declared))
		} else if declared, ok := l.returnLocalObjectLiteralContractForSymbol(target.Symbol); ok && recordWithCallableField(declared) {
			assignment = factflow.NewRootAssignmentWithDeclaredContractValue(kind, target.Symbol, target, source, l.valueFromTypeWithWitness(declared))
		}
	}
	input.RootAssignments[point] = assignment
	if declared, ok := l.wirAssignmentDeclaredObjectType(inst, target.Symbol); ok {
		l.addObjectLiteralFieldExposuresFromWIR(input, point, inst, declared)
	}
}

func (l *lowerer) wirAssignmentDeclaredObjectType(inst wir.Instruction, target symbol.ID) (typ.Type, bool) {
	if l == nil {
		return nil, false
	}
	if l.wir != nil && inst.Type != 0 {
		if declared := l.wir.Type(inst.Type); declared != nil {
			return declared, true
		}
	}
	declared, ok := l.symbolTypes[target]
	return declared, ok && declared != nil
}

func (l *lowerer) rootAssignmentValueSourceFromWIR(point cfg.Point, inst wir.Instruction) (factflow.ValueSource, bool) {
	if sourceOp, ok := inst.AssignmentSourceOperand(); ok {
		return l.assignmentValueSourceFromWIROperand(point, sourceOp)
	}
	switch inst.Op {
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
			l.resultValueSourcesByTempFromWIR(),
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
	default:
		return factflow.ValueSource{}, false
	}
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
	input.PathDescendantInvalidations[point] = factflow.NewPathDescendantInvalidation(tablePath).WithDynamicTarget(tablePath, keySource, nil)
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
	return l.valueSourceFromWIROperand(
		op,
		sourceprovenance.NoSourceIndex,
		0,
		true,
		false,
		false,
		l.resultValueSourcesByTempFromWIR(),
	)
}

func (l *lowerer) assignmentSource(point cfg.Point, fallback sourceprovenance.ASTSource) factflow.ValueSource {
	if source, ok := l.assignmentSourceFromWIR(point, fallback); ok {
		return source
	}
	if l.hasMalformedSimpleWIRAssignmentSource(point) {
		return factflow.NewUnknownValueSource(fallback.TargetIndex)
	}
	return l.valueSource(fallback)
}

func (l *lowerer) assignmentSourceFromWIR(point cfg.Point, fallback sourceprovenance.ASTSource) (factflow.ValueSource, bool) {
	if l == nil || l.wir == nil {
		return factflow.ValueSource{}, false
	}
	for _, inst := range l.wir.PointInstructions(point) {
		if source, ok := l.assignmentProducerSourceFromWIR(inst, fallback); ok {
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
		fallback.ExprIndex,
		fallback.TargetIndex,
		fallback.Final,
		fallback.Expanded,
		fallback.OpenTail,
	); ok {
		return source, true
	}
	if source, ok := l.pathExpressionSourceFromWIR(
		"assignment-source",
		point,
		op,
		fallback.ExprIndex,
		fallback.TargetIndex,
		fallback.Final,
		fallback.Expanded,
		fallback.OpenTail,
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
	resultSources := l.resultValueSourcesByTempFromWIR()
	source, ok := l.valueSourceFromWIROperand(
		op,
		fallback.ExprIndex,
		fallback.TargetIndex,
		fallback.Final,
		fallback.Expanded,
		fallback.OpenTail,
		resultSources,
	)
	if !ok {
		return factflow.ValueSource{}, false
	}
	if source.Kind == factflow.ValueSourceCall {
		if fallbackSource := l.valueSource(fallback); fallbackSource.HasExpr {
			source.ExprRef = fallbackSource.ExprRef
			source.HasExpr = true
			return source, true
		}
		l.addWIRCallResultExprRef(&source, op, fallback, resultSources)
	}
	return source, true
}

func (l *lowerer) assignmentProducerSourceFromWIR(inst wir.Instruction, fallback sourceprovenance.ASTSource) (factflow.ValueSource, bool) {
	if !assignmentProducerMatchesSource(inst, fallback) {
		return factflow.ValueSource{}, false
	}
	switch inst.Op {
	case wir.OpClosure:
		return l.wirClosureExpressionValueSource(
			inst,
			fallback.ExprIndex,
			fallback.TargetIndex,
			fallback.Final,
			fallback.Expanded,
			fallback.OpenTail,
		)
	case wir.OpMakeTable:
		return l.wirTableExpressionValueSource(
			inst,
			fallback.ExprIndex,
			fallback.TargetIndex,
			fallback.Final,
			fallback.Expanded,
			fallback.OpenTail,
		)
	default:
		return factflow.ValueSource{}, false
	}
}

func assignmentProducerMatchesSource(inst wir.Instruction, fallback sourceprovenance.ASTSource) bool {
	if inst.ExprID == 0 || fallback.Expr == nil {
		return false
	}
	expr := fallback.Expr
	if inner, ok := sourceprovenance.ProofInner(expr); ok {
		expr = inner
	}
	return inst.ExprID == expressionid.Of(expr)
}

func (l *lowerer) addWIRCallResultExprRef(source *factflow.ValueSource, op wir.Operand, fallback sourceprovenance.ASTSource, resultSources map[uint32]wirResultSource) {
	if l == nil || source == nil || op.Kind != wir.OperandTemp {
		return
	}
	if _, ok := claimKindForAssertionSource(fallback.Expr); ok {
		return
	}
	result, ok := resultSources[op.Ref]
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

func (l *lowerer) hasMalformedSimpleWIRAssignmentSource(point cfg.Point) bool {
	if l == nil || l.wir == nil {
		return false
	}
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.Op != wir.OpAssign {
			continue
		}
		if _, ok := inst.AssignmentSourceOperand(); !ok {
			return true
		}
	}
	return false
}

func (l *lowerer) hasAssignmentWriteFromWIR(point cfg.Point) bool {
	if l == nil || l.wir == nil {
		return false
	}
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.WritesAssignmentPoint() {
			return true
		}
	}
	return false
}
