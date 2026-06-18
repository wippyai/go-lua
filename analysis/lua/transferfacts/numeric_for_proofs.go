package transferfacts

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
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
