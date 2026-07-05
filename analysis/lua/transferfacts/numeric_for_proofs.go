package transferfacts

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse/numparse"
)

// numericForBranchPathEvidence lowers `for i = init, #xs do` into the same
// true-edge proof as an explicit `i <= #xs` guard. Consumers still need a
// separate positive floor for i before they can remove array-read nil.
func (l *lowerer) numericForBranchPathEvidence(fact cfgfacts.NumericForFact) []factflow.BranchPathEvidence {
	if fact.Role != cfgfacts.NumericForRoleCheck || !fact.HasSymbol || fact.Symbol == 0 {
		return nil
	}
	arrayPath, ok := numericForInRangeArrayPath(fact, l.bindings)
	if !ok {
		return nil
	}
	indexPath := pathdom.NewPath(fact.Symbol, fact.Name)
	return []factflow.BranchPathEvidence{
		factflow.NewBranchIndexInRangeEvidenceOnEdge(indexPath, arrayPath, true),
	}
}

func (l *lowerer) numericForBranchPathEvidenceFromWIR(point cfg.Point) []factflow.BranchPathEvidence {
	header, ok := l.numericForHeaderFromWIR(point)
	if !ok {
		return nil
	}
	indexPath, ok := l.numericForIndexPathFromWIR(header)
	if !ok {
		return nil
	}
	arrayPath, ok := l.numericForInRangeArrayPathFromWIR(header)
	if !ok {
		return nil
	}
	return []factflow.BranchPathEvidence{
		factflow.NewBranchIndexInRangeEvidenceOnEdge(indexPath, arrayPath, true),
	}
}

func (l *lowerer) numericForBranchNumFloorRefinement(fact cfgfacts.NumericForFact) (factflow.BranchNumFloorRefinement, bool) {
	if fact.Role != cfgfacts.NumericForRoleCheck || !fact.HasSymbol || fact.Symbol == 0 {
		return factflow.BranchNumFloorRefinement{}, false
	}
	floor, ok := numericForIndexFloor(fact)
	if !ok {
		return factflow.BranchNumFloorRefinement{}, false
	}
	indexPath := pathdom.NewPath(fact.Symbol, fact.Name)
	return factflow.NewBranchNumFloorRefinement(indexPath, floor), true
}

func (l *lowerer) numericForBranchNumFloorRefinementFromWIR(point cfg.Point) (factflow.BranchNumFloorRefinement, bool) {
	header, ok := l.numericForHeaderFromWIR(point)
	if !ok {
		return factflow.BranchNumFloorRefinement{}, false
	}
	indexPath, ok := l.numericForIndexPathFromWIR(header)
	if !ok {
		return factflow.BranchNumFloorRefinement{}, false
	}
	floor, ok := l.numericForIndexFloorFromWIR(header)
	if !ok {
		return factflow.BranchNumFloorRefinement{}, false
	}
	return factflow.NewBranchNumFloorRefinement(indexPath, floor), true
}

func (l *lowerer) numericForHeaderFromWIR(point cfg.Point) (wir.Instruction, bool) {
	if l == nil || l.wir == nil {
		return wir.Instruction{}, false
	}
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

func (l *lowerer) numericForInRangeArrayPathFromWIR(header wir.Instruction) (pathdom.Path, bool) {
	bounds := l.wir.Operands(header.List)
	if len(bounds) < 2 {
		return pathdom.Path{}, false
	}
	direction, ok := l.numericForStepDirectionFromWIR(bounds)
	if !ok {
		return pathdom.Path{}, false
	}
	if direction > 0 {
		return l.numericForLengthOperandPathFromWIR(bounds[1])
	}
	return l.numericForLengthOperandPathFromWIR(bounds[0])
}

func (l *lowerer) numericForIndexFloorFromWIR(header wir.Instruction) (int64, bool) {
	bounds := l.wir.Operands(header.List)
	if len(bounds) < 2 {
		return 0, false
	}
	direction, ok := l.numericForStepDirectionFromWIR(bounds)
	if !ok {
		return 0, false
	}
	if direction > 0 {
		return l.numericForPositiveFloorFromWIR(bounds[0])
	}
	return l.numericForPositiveFloorFromWIR(bounds[1])
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
	switch op.Kind {
	case wir.OperandConst:
		c := l.wir.Const(wir.ConstRef(op.Ref))
		if c.Kind != wir.ConstNumber {
			return 0, false
		}
		return numparse.ParseIntegralLiteral(c.Number)
	case wir.OperandTemp:
		inst, ok := l.wirTempDefs()[op.Ref]
		if !ok || inst.Op != wir.OpUnOp || inst.Operator != wir.UnNeg {
			return 0, false
		}
		value, ok := l.numericForIntegralLiteralFromWIR(inst.A)
		if !ok {
			return 0, false
		}
		return -value, true
	default:
		return 0, false
	}
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

func numericForInRangeArrayPath(fact cfgfacts.NumericForFact, bindings *bind.Result) (pathdom.Path, bool) {
	direction, ok := numericForStepDirection(fact.Step)
	if !ok {
		return pathdom.Path{}, false
	}
	if direction > 0 {
		return numericForLengthExprPath(fact.Limit, bindings)
	}
	return numericForLengthExprPath(fact.Init, bindings)
}

func numericForLengthExprPath(expr ast.Expr, bindings *bind.Result) (pathdom.Path, bool) {
	lenOp, ok := expr.(*ast.UnaryLenOpExpr)
	if !ok {
		return pathdom.Path{}, false
	}
	return pathexpr.Resolve(lenOp.Expr, bindings)
}

func numericForIndexFloor(fact cfgfacts.NumericForFact) (int64, bool) {
	direction, ok := numericForStepDirection(fact.Step)
	if !ok {
		return 0, false
	}
	if direction > 0 {
		return numericForPositiveFloor(fact.Init)
	}
	return numericForPositiveFloor(fact.Limit)
}

func numericForPositiveFloor(expr ast.Expr) (int64, bool) {
	value, ok := numericForIntegralLiteral(expr)
	if !ok || value < 1 {
		return 0, false
	}
	return value, true
}

func numericForStepDirection(expr ast.Expr) (int, bool) {
	if expr == nil {
		return 1, true
	}
	value, ok := numericForIntegralLiteral(expr)
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

func numericForIntegralLiteral(expr ast.Expr) (int64, bool) {
	if expr == nil {
		return 0, false
	}
	switch e := expr.(type) {
	case *ast.NumberExpr:
		return numparse.ParseIntegralLiteral(e.Value)
	case *ast.UnaryMinusOpExpr:
		value, ok := numericForIntegralLiteral(e.Expr)
		if !ok {
			return 0, false
		}
		return -value, true
	default:
		return 0, false
	}
}
