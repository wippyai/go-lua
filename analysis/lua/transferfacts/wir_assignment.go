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
	if op.Kind != wir.OperandConst && op.Kind != wir.OperandTemp {
		return factflow.ValueSource{}, false
	}
	source, ok := l.valueSourceFromWIROperand(
		op,
		fallback.ExprIndex,
		fallback.TargetIndex,
		fallback.Final,
		fallback.Expanded,
		fallback.OpenTail,
		l.callResultValueSourcesByTempFromWIR(),
	)
	if !ok {
		return factflow.ValueSource{}, false
	}
	if source.Kind == factflow.ValueSourceCall {
		fallbackSource := l.valueSource(fallback)
		if fallbackSource.HasExpr {
			source.ExprRef = fallbackSource.ExprRef
			source.HasExpr = true
		}
	}
	return source, true
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
		switch inst.Op {
		case wir.OpAssign, wir.OpStaticMemberWrite:
			if inst.A.Kind != wir.OperandNone {
				return inst.A, true
			}
		case wir.OpDynamicIndexWrite:
			if inst.B.Kind != wir.OperandNone {
				return inst.B, true
			}
		}
	}
	return wir.Operand{}, false
}

func (l *lowerer) hasAssignmentWriteFromWIR(point cfg.Point) bool {
	if l == nil || l.wir == nil {
		return false
	}
	for _, inst := range l.wir.PointInstructions(point) {
		if wirInstructionWritesAssignmentPoint(inst) {
			return true
		}
	}
	return false
}

func wirInstructionWritesAssignmentPoint(inst wir.Instruction) bool {
	switch inst.Op {
	case wir.OpAssign, wir.OpMakeTable, wir.OpBinOp, wir.OpUnOp, wir.OpConcat, wir.OpClaim, wir.OpSelect, wir.OpLogical, wir.OpClosure:
		return inst.Dst.Kind != wir.OperandNone
	case wir.OpStaticMemberWrite, wir.OpDynamicIndexWrite:
		return true
	default:
		return false
	}
}
