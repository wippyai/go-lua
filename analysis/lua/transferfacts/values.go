package transferfacts

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) valueSources(sources []sourceprovenance.ASTSource) []factflow.ValueSource {
	if len(sources) == 0 {
		return nil
	}
	out := make([]factflow.ValueSource, len(sources))
	for i := range sources {
		out[i] = l.valueSource(sources[i])
	}
	return out
}

func (l *lowerer) returnValueSources(sources []sourceprovenance.ASTSource, result *semantics.Result) []factflow.ValueSource {
	if len(sources) == 0 {
		return nil
	}
	out := make([]factflow.ValueSource, 0, len(sources))
	for _, source := range sources {
		for _, expanded := range l.expandTypeIsOpenTailReturnSource(source, result) {
			out = append(out, l.valueSource(expanded))
		}
	}
	return out
}

func (l *lowerer) expandTypeIsOpenTailReturnSource(source sourceprovenance.ASTSource, result *semantics.Result) []sourceprovenance.ASTSource {
	if source.Kind != factflow.ValueSourceCall || !source.OpenTail || !source.Expanded ||
		!source.HasCallPoint || result == nil {
		return []sourceprovenance.ASTSource{source}
	}
	fact, ok := result.Call(source.CallPoint)
	if !ok {
		return []sourceprovenance.ASTSource{source}
	}
	if _, _, ok := l.typeIsCall(fact); !ok {
		return []sourceprovenance.ASTSource{source}
	}
	value := source
	value.OpenTail = false
	errorSource := source
	errorSource.TargetIndex = source.TargetIndex + 1
	errorSource.ResultIndex = source.ResultIndex + 1
	errorSource.OpenTail = false
	return []sourceprovenance.ASTSource{value, errorSource}
}

func (l *lowerer) valueSource(source sourceprovenance.ASTSource) factflow.ValueSource {
	exprRef, hasExpr := l.valueSourceExprRef(source)
	if hasExpr {
		l.addExpressionPath(exprRef, source.Expr)
		l.addExpressionCondition(exprRef, source.Expr)
	}
	shape, ok := factflow.NewValueSourceShape(source.Final, source.Expanded, source.Adjusted, source.OpenTail)
	if !ok {
		panic("transferfacts: invalid value source shape")
	}
	switch source.Kind {
	case factflow.ValueSourceExpression:
		return mustValueSource(factflow.NewExpressionValueSource(exprRef, source.ExprIndex, source.TargetIndex, source.ResultIndex, shape))
	case factflow.ValueSourceCall:
		if !source.HasCallPoint {
			return factflow.NewUnknownValueSource(source.TargetIndex)
		}
		return mustValueSource(factflow.NewCallValueSource(exprRef, source.ExprIndex, source.TargetIndex, source.ResultIndex, source.CallPoint, shape))
	case factflow.ValueSourceVararg:
		return mustValueSource(factflow.NewVarargValueSource(exprRef, source.ExprIndex, source.TargetIndex, source.ResultIndex, shape))
	case factflow.ValueSourceNil:
		return factflow.NewNilValueSource(source.TargetIndex)
	case factflow.ValueSourceUnknown:
		return factflow.NewUnknownValueSource(source.TargetIndex)
	default:
		panic("transferfacts: unknown value source kind")
	}
}

func mustValueSource(source factflow.ValueSource, ok bool) factflow.ValueSource {
	if !ok {
		panic("transferfacts: invalid value source")
	}
	return source
}

type sourceExprRefKey struct {
	expr         ast.Expr
	kind         factflow.ValueSourceKind
	exprIndex    int
	targetIndex  int
	resultIndex  int
	callPoint    cfg.Point
	hasCallPoint bool
	final        bool
	expanded     bool
	adjusted     bool
	openTail     bool
}

func (l *lowerer) valueSourceExprRef(source sourceprovenance.ASTSource) (factflow.ExprRef, bool) {
	if !sourceScopedExprRef(source) {
		return l.exprRef(source.Expr)
	}
	return l.exprRef(sourceExprRefKey{
		expr:         source.Expr,
		kind:         source.Kind,
		exprIndex:    source.ExprIndex,
		targetIndex:  source.TargetIndex,
		resultIndex:  source.ResultIndex,
		callPoint:    source.CallPoint,
		hasCallPoint: source.HasCallPoint,
		final:        source.Final,
		expanded:     source.Expanded,
		adjusted:     source.Adjusted,
		openTail:     source.OpenTail,
	})
}

func sourceScopedExprRef(source sourceprovenance.ASTSource) bool {
	if !isAssertionWrapper(source.Expr) {
		return false
	}
	switch source.Kind {
	case factflow.ValueSourceCall, factflow.ValueSourceVararg:
		return true
	default:
		return false
	}
}

func isAssertionWrapper(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.CastExpr, *ast.NonNilAssertExpr:
		return true
	default:
		return false
	}
}

func (l *lowerer) addExpressionPath(ref factflow.ExprRef, expr ast.Expr) {
	if ref == 0 || expr == nil || l.bindings == nil {
		return
	}
	p, ok := pathexpr.Resolve(expr, l.bindings)
	if !ok || p.IsEmpty() {
		return
	}
	if l.expressionPaths == nil {
		l.expressionPaths = make(map[factflow.ExprRef]pathdom.Path)
	}
	l.expressionPaths[ref] = p
}

func (l *lowerer) addExpressionCondition(ref factflow.ExprRef, expr ast.Expr) {
	if ref == 0 || expr == nil || l.bindings == nil {
		return
	}
	var trueRefinements []factflow.PostconditionRefinement
	var falseRefinements []factflow.PostconditionRefinement
	for _, check := range branchcond.TruthyChecks(expr, l.bindings) {
		refinement, ok := l.branchRefinement(semantics.BranchConditionFact{Check: check})
		if !ok {
			continue
		}
		if value, ok := refinement.TrueValue(); ok {
			trueRefinements = append(trueRefinements, factflow.NewPostconditionRefinement(refinement.TargetPath(), value))
		}
	}
	if refinement, returnValue, ok := l.typeIsExpressionConditionRefinement(expr); ok {
		if returnValue {
			trueRefinements = append(trueRefinements, refinement)
		} else {
			falseRefinements = append(falseRefinements, refinement)
		}
	}
	check := branchcond.Normalize(expr, l.bindings)
	if check.Kind != branchcond.CheckNone {
		if refinement, ok := l.branchRefinement(semantics.BranchConditionFact{Check: check}); ok {
			if value, ok := refinement.FalseValue(); ok {
				falseRefinements = append(falseRefinements, factflow.NewPostconditionRefinement(refinement.TargetPath(), value))
			}
		}
	}
	var trueRelations []factflow.PostconditionPathRelation
	var falseRelations []factflow.PostconditionPathRelation
	for _, check := range branchcond.TruthyChecks(expr, l.bindings) {
		relations, ok := l.branchPathRelations(semantics.BranchConditionFact{Check: check})
		if !ok {
			continue
		}
		for _, relation := range relations.Relations() {
			if relation.Kind() != factflow.BranchPathRelationEqual || !relation.ActiveOnEdge(true) {
				continue
			}
			trueRelations = append(trueRelations, factflow.NewPostconditionPathEquality(relation.LeftPath(), relation.RightPath()))
		}
	}
	if check.Kind != branchcond.CheckNone {
		if relations, ok := l.branchPathRelations(semantics.BranchConditionFact{Check: check}); ok {
			for _, relation := range relations.Relations() {
				if relation.Kind() != factflow.BranchPathRelationEqual || !relation.ActiveOnEdge(false) {
					continue
				}
				falseRelations = append(falseRelations, factflow.NewPostconditionPathEquality(relation.LeftPath(), relation.RightPath()))
			}
		}
	}
	condition := factflow.NewExpressionCondition(trueRefinements, falseRefinements, trueRelations, falseRelations)
	if condition.IsEmpty() {
		return
	}
	l.expressionConditions[ref] = condition
}

func (l *lowerer) exprRef(expr any) (factflow.ExprRef, bool) {
	if expr == nil {
		return 0, false
	}
	if ref, ok := l.exprs[expr]; ok {
		return ref, true
	}
	ref := factflow.ExprRef(len(l.exprs) + 1)
	l.exprs[expr] = ref
	return ref, true
}

func (l *lowerer) typeRefs(types []ast.TypeExpr) []factflow.TypeRef {
	if len(types) == 0 {
		return nil
	}
	out := make([]factflow.TypeRef, len(types))
	for i := range types {
		out[i], _ = l.typeRef(types[i])
	}
	return out
}

func (l *lowerer) typeRef(typ any) (factflow.TypeRef, bool) {
	if typ == nil {
		return 0, false
	}
	if ref, ok := l.types[typ]; ok {
		return ref, true
	}
	if l.types == nil {
		l.types = make(map[any]factflow.TypeRef)
	}
	ref := factflow.TypeRef(len(l.types) + 1)
	l.types[typ] = ref
	return ref, true
}
