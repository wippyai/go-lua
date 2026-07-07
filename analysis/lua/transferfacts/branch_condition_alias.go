package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func (l *lowerer) addLocalConditionAlias(sym symbol.ID, source factflow.ValueSource) {
	if l == nil || sym == 0 || l.bindings == nil || l.bindings.HasWrite(sym) || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return
	}
	condition, ok := l.expressionConditions[source.ExprRef]
	if !ok || condition.IsEmpty() {
		return
	}
	if l.localConditionAliases == nil {
		l.localConditionAliases = make(map[symbol.ID]factflow.ExpressionCondition)
	}
	l.localConditionAliases[sym] = condition
}

func (l *lowerer) branchAliasRefinementsFromWIR(point cfg.Point) []factflow.BranchRefinement {
	return collectWIRBranchAliases(l, point, func(aliased factflow.ExpressionCondition, trueValue bool) []factflow.BranchRefinement {
		return aliased.BranchRefinementsForValue(trueValue)
	})
}

func (l *lowerer) branchAliasPathRelationsFromWIR(point cfg.Point) []factflow.BranchPathRelation {
	return collectWIRBranchAliases(l, point, func(aliased factflow.ExpressionCondition, trueValue bool) []factflow.BranchPathRelation {
		return aliased.BranchPathRelationsForValue(trueValue)
	})
}

func (l *lowerer) branchAliasPathEvidenceFromWIR(point cfg.Point) []factflow.BranchPathEvidence {
	return collectWIRBranchAliases(l, point, func(aliased factflow.ExpressionCondition, trueValue bool) []factflow.BranchPathEvidence {
		return aliased.BranchPathEvidenceForValue(trueValue)
	})
}

func collectWIRBranchAliases[T any](
	l *lowerer,
	point cfg.Point,
	selector func(factflow.ExpressionCondition, bool) []T,
) []T {
	if l == nil || l.wir == nil || len(l.localConditionAliases) == 0 {
		return nil
	}
	var out []T
	l.forEachWIRBranchCheck(point, func(check branchcond.Check) {
		aliased, trueValue, ok := l.wirAliasedExpressionCondition(check)
		if !ok {
			return
		}
		out = append(out, selector(aliased, trueValue)...)
	}, func(branchcond.ImpliedCheck) {})
	return out
}

func (l *lowerer) wirAliasedExpressionCondition(check branchcond.Check) (factflow.ExpressionCondition, bool, bool) {
	trueValue := true
	switch check.Kind {
	case branchcond.CheckTruthy:
	case branchcond.CheckFalsy:
		trueValue = false
	default:
		return factflow.ExpressionCondition{}, false, false
	}
	if check.Path.Symbol == 0 || len(check.Path.Segments) != 0 {
		return factflow.ExpressionCondition{}, false, false
	}
	condition, ok := l.localConditionAliases[check.Path.Symbol]
	return condition, trueValue, ok
}
