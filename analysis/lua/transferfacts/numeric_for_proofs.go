package transferfacts

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

// numericForBranchPathEvidence lowers `for i = init, #xs do` into the same
// true-edge proof as an explicit `i <= #xs` guard. Consumers still need a
// separate positive floor for i before they can remove array-read nil.
func (l *lowerer) numericForBranchPathEvidenceFromWIR(point cfg.Point) []factflow.BranchPathEvidence {
	info, ok := l.numericForLoopInfoFromWIR(point)
	if !ok || !info.hasArrayPath {
		return nil
	}
	return []factflow.BranchPathEvidence{
		factflow.NewBranchIndexInRangeEvidenceOnEdge(info.indexPath, info.arrayPath, true),
	}
}

func (l *lowerer) numericForBranchNumFloorRefinementFromWIR(point cfg.Point) (factflow.BranchNumFloorRefinement, bool) {
	info, ok := l.numericForLoopInfoFromWIR(point)
	if !ok || !info.hasIndexFloor {
		return factflow.BranchNumFloorRefinement{}, false
	}
	return factflow.NewBranchNumFloorRefinement(info.indexPath, info.indexFloor), true
}

func (l *lowerer) numericForHeaderFromWIR(point cfg.Point) (wir.Instruction, bool) {
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.Op == wir.OpIterate && inst.Iter == wir.IterNumeric {
			return inst, true
		}
	}
	return wir.Instruction{}, false
}

func (l *lowerer) numericForIndexPathFromWIR(header wir.Instruction) (pathdom.Path, bool) {
	results := l.wir.Operands(header.Results)
	if len(results) == 0 || results[0].Kind != wir.OperandPath {
		return pathdom.Path{}, false
	}
	indexPath := l.wir.Path(wir.PathRef(results[0].Ref))
	if indexPath.IsEmpty() || indexPath.Symbol == 0 {
		return pathdom.Path{}, false
	}
	return indexPath, true
}

func (l *lowerer) numericForLoopInfoFromWIR(point cfg.Point) (numericForLoopInfo, bool) {
	header, ok := l.numericForHeaderFromWIR(point)
	if !ok {
		return numericForLoopInfo{}, false
	}
	indexPath, ok := l.numericForIndexPathFromWIR(header)
	if !ok {
		return numericForLoopInfo{}, false
	}
	info := numericForLoopInfo{indexPath: indexPath}
	bounds := l.wir.Operands(header.List)
	if len(bounds) < 2 {
		return info, true
	}
	direction, ok := l.numericForStepDirectionFromWIR(bounds)
	if !ok {
		return info, true
	}
	var arrayOperand wir.Operand
	var floorOperand wir.Operand
	if direction > 0 {
		arrayOperand = bounds[1]
		floorOperand = bounds[0]
	} else {
		arrayOperand = bounds[0]
		floorOperand = bounds[1]
	}
	if arrayPath, ok := l.numericForLengthOperandPathFromWIR(arrayOperand); ok {
		info.arrayPath = arrayPath
		info.hasArrayPath = true
	}
	if floor, ok := l.numericForPositiveFloorFromWIR(floorOperand); ok {
		info.indexFloor = floor
		info.hasIndexFloor = true
	}
	return info, true
}

func (l *lowerer) numericForStepDirectionFromWIR(bounds []wir.Operand) (int, bool) {
	if len(bounds) < 3 {
		return 1, true
	}
	value, ok := l.numericForIntegralLiteralFromWIR(bounds[2])
	if !ok {
		return 0, false
	}
	if value < 0 {
		return -1, true
	}
	if value > 0 {
		return 1, true
	}
	return 0, false
}

func (l *lowerer) numericForPositiveFloorFromWIR(op wir.Operand) (int64, bool) {
	value, ok := l.numericForIntegralLiteralFromWIR(op)
	if !ok || value < 1 {
		return 0, false
	}
	return value, true
}

func (l *lowerer) numericForIntegralLiteralFromWIR(op wir.Operand) (int64, bool) {
	return numericForIntegralLiteralFromWIR(l.wir, l.wirTempDefs(), op)
}

func (l *lowerer) numericForLengthOperandPathFromWIR(op wir.Operand) (pathdom.Path, bool) {
	if op.Kind != wir.OperandTemp {
		return pathdom.Path{}, false
	}
	inst, ok := l.wirTempDefs()[op.Ref]
	if !ok || inst.Op != wir.OpUnOp || inst.Operator != wir.UnLen || inst.A.Kind != wir.OperandPath {
		return pathdom.Path{}, false
	}
	arrayPath := l.wir.Path(wir.PathRef(inst.A.Ref))
	if arrayPath.IsEmpty() {
		return pathdom.Path{}, false
	}
	return arrayPath, true
}

type numericForLoopInfo struct {
	indexPath pathdom.Path

	arrayPath    pathdom.Path
	hasArrayPath bool

	indexFloor    int64
	hasIndexFloor bool
}
