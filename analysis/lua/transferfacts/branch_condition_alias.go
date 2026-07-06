package transferfacts

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
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

func (l *lowerer) branchAliasRefinements(condition ast.Expr) []factflow.BranchRefinement {
	aliased, trueValue, ok := l.aliasedExpressionCondition(condition)
	if !ok {
		return nil
	}
	return aliased.BranchRefinementsForValue(trueValue)
}

func (l *lowerer) branchAliasPathRelations(condition ast.Expr) []factflow.BranchPathRelation {
	aliased, trueValue, ok := l.aliasedExpressionCondition(condition)
	if !ok {
		return nil
	}
	return aliased.BranchPathRelationsForValue(trueValue)
}

func (l *lowerer) branchAliasPathEvidence(condition ast.Expr) []factflow.BranchPathEvidence {
	aliased, trueValue, ok := l.aliasedExpressionCondition(condition)
	if !ok {
		return nil
	}
	return aliased.BranchPathEvidenceForValue(trueValue)
}

func (l *lowerer) branchAliasRefinementsFromWIR(point cfg.Point) []factflow.BranchRefinement {
	if l == nil || l.wir == nil || len(l.localConditionAliases) == 0 {
		return nil
	}
	var out []factflow.BranchRefinement
	l.forEachWIRBranchCheck(point, func(check branchcond.Check) {
		aliased, trueValue, ok := l.wirAliasedExpressionCondition(check)
		if !ok {
			return
		}
		out = append(out, aliased.BranchRefinementsForValue(trueValue)...)
	}, func(branchcond.ImpliedCheck) {})
	return out
}

func (l *lowerer) branchAliasPathRelationsFromWIR(point cfg.Point) []factflow.BranchPathRelation {
	if l == nil || l.wir == nil || len(l.localConditionAliases) == 0 {
		return nil
	}
	var out []factflow.BranchPathRelation
	l.forEachWIRBranchCheck(point, func(check branchcond.Check) {
		aliased, trueValue, ok := l.wirAliasedExpressionCondition(check)
		if !ok {
			return
		}
		out = append(out, aliased.BranchPathRelationsForValue(trueValue)...)
	}, func(branchcond.ImpliedCheck) {})
	return out
}

func (l *lowerer) branchAliasPathEvidenceFromWIR(point cfg.Point) []factflow.BranchPathEvidence {
	if l == nil || l.wir == nil || len(l.localConditionAliases) == 0 {
		return nil
	}
	var out []factflow.BranchPathEvidence
	l.forEachWIRBranchCheck(point, func(check branchcond.Check) {
		aliased, trueValue, ok := l.wirAliasedExpressionCondition(check)
		if !ok {
			return
		}
		out = append(out, aliased.BranchPathEvidenceForValue(trueValue)...)
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

func (l *lowerer) aliasedExpressionCondition(expr ast.Expr) (factflow.ExpressionCondition, bool, bool) {
	if expr == nil || l.bindings == nil {
		return factflow.ExpressionCondition{}, false, false
	}
	trueValue := true
	for {
		not, ok := expr.(*ast.UnaryNotOpExpr)
		if !ok || not == nil {
			break
		}
		trueValue = !trueValue
		expr = not.Expr
	}
	p, ok := pathexpr.Resolve(expr, l.bindings)
	if !ok || p.Symbol == 0 || len(p.Segments) != 0 {
		return factflow.ExpressionCondition{}, false, false
	}
	if l.bindings.HasWrite(p.Symbol) {
		return factflow.ExpressionCondition{}, false, false
	}
	origin, ok := l.bindings.LocalOrigin(p.Symbol)
	if !ok || origin.Stmt == nil || origin.Index < 0 || origin.Index >= len(origin.Stmt.Exprs) {
		return factflow.ExpressionCondition{}, false, false
	}
	originExpr := origin.Stmt.Exprs[origin.Index]
	source := sourceprovenance.SourceForExpr(originExpr, sourceprovenance.NoSourceIndex, sourceprovenance.NoSourceIndex, 0, true, false, l.callPointForExpr)
	originRef := l.valueSource(source)
	if !originRef.HasExpr {
		return factflow.ExpressionCondition{}, false, false
	}
	condition, ok := l.expressionConditions[originRef.ExprRef]
	return condition, trueValue, ok
}
