package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func (l *lowerer) assignmentSource(point cfg.Point, fallback sourceprovenance.ASTSource) factflow.ValueSource {
	if source, ok := l.assignmentSourceFromWIR(point, fallback); ok {
		return source
	}
	return l.valueSource(fallback)
}

func (l *lowerer) assignmentSourceFromWIR(point cfg.Point, fallback sourceprovenance.ASTSource) (factflow.ValueSource, bool) {
	if l == nil || l.wir == nil {
		return factflow.ValueSource{}, false
	}
	if source, ok := l.tableConstructorAssignmentSourceFromWIR(point, fallback); ok {
		return source, true
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

func (l *lowerer) tableConstructorAssignmentSourceFromWIR(point cfg.Point, fallback sourceprovenance.ASTSource) (factflow.ValueSource, bool) {
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.Op != wir.OpMakeTable || inst.Dst.Kind != wir.OperandPath {
			continue
		}
		return l.wirTableExpressionValueSource(
			inst,
			fallback.ExprIndex,
			fallback.TargetIndex,
			fallback.Final,
			fallback.Expanded,
			fallback.OpenTail,
		)
	}
	return factflow.ValueSource{}, false
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
