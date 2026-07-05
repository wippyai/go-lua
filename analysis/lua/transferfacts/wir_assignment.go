package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
)

func (l *lowerer) assignmentSource(point cfg.Point, fallback sourceprovenance.ASTSource) factflow.ValueSource {
	if source, ok := l.assignmentSourceFromWIR(point, fallback); ok {
		// Expression sidecars are still live during the WIR migration. Allocate
		// the fallback source's refs so unrelated sidecar ref identity remains
		// stable until those lanes are deleted.
		_ = l.valueSource(fallback)
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
	if op.Kind != wir.OperandConst {
		return factflow.ValueSource{}, false
	}
	return l.valueSourceFromWIROperand(
		op,
		fallback.ExprIndex,
		fallback.TargetIndex,
		fallback.Final,
		fallback.Expanded,
		fallback.OpenTail,
		l.callResultValueSourcesByTempFromWIR(),
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
