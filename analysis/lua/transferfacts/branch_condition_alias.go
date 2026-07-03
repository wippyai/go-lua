package transferfacts

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) branchAliasRefinements(condition ast.Expr) []factflow.BranchRefinement {
	aliased, trueValue, ok := l.aliasedExpressionCondition(condition)
	if !ok {
		return nil
	}
	trueFacts := aliased.FactsForValue(trueValue)
	falseFacts := aliased.FactsForValue(!trueValue)
	return branchAliasRefinementsFromFacts(trueFacts.Refinements(), falseFacts.Refinements())
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

func branchAliasRefinementsFromFacts(
	trueRefinements []factflow.PostconditionRefinement,
	falseRefinements []factflow.PostconditionRefinement,
) []factflow.BranchRefinement {
	byPath := make(map[pathdom.PathKey]*branchAliasRefinementBuilder, len(trueRefinements)+len(falseRefinements))
	var order []pathdom.PathKey
	add := func(ref factflow.PostconditionRefinement, edge bool) {
		key := ref.TargetPath().Key()
		builder := byPath[key]
		if builder == nil {
			builder = &branchAliasRefinementBuilder{target: ref.TargetPath()}
			byPath[key] = builder
			order = append(order, key)
		}
		if edge {
			builder.trueValue = ref.Value()
			builder.hasTrue = true
		} else {
			builder.falseValue = ref.Value()
			builder.hasFalse = true
		}
	}
	for _, ref := range trueRefinements {
		add(ref, true)
	}
	for _, ref := range falseRefinements {
		add(ref, false)
	}
	out := make([]factflow.BranchRefinement, 0, len(order))
	for _, key := range order {
		builder := byPath[key]
		out = append(out, factflow.NewBranchRefinement(
			builder.target,
			builder.trueValue,
			builder.hasTrue,
			builder.falseValue,
			builder.hasFalse,
		))
	}
	return out
}

type branchAliasRefinementBuilder struct {
	target     pathdom.Path
	trueValue  factflow.ValueRefinement
	hasTrue    bool
	falseValue factflow.ValueRefinement
	hasFalse   bool
}
