package transferfacts

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeoperator"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

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

func (l *lowerer) addExpressionValue(ref factflow.ExprRef, expr ast.Expr) {
	if ref == 0 || expr == nil {
		return
	}
	value, ok := l.expressionValue(expr)
	if !ok {
		return
	}
	l.expressionValues[ref] = value
}

func (l *lowerer) expressionValue(expr ast.Expr) (product.Value, bool) {
	if t, ok := valueexpr.LiteralType(expr); ok {
		value := typevalue.FromType(l.registry, t)
		return typevalue.WithWitness(l.registry, value, t), true
	}
	kind, ok := valueexpr.RuntimeKind(expr)
	if ok {
		value := product.NewWithPresence(l.registry, product.ShapeTop, presence.Present())
		return product.Set(l.registry, value, runtimekind.Key, kind), true
	}
	if ident, ok := expr.(*ast.IdentExpr); ok {
		if t, ok := l.identType(ident); ok {
			return typevalue.FromType(l.registry, t), true
		}
	}
	if t, ok := l.scalarOperationType(expr); ok {
		return typevalue.FromType(l.registry, t), true
	}
	return product.Value{}, false
}

func (l *lowerer) scalarOperationType(expr ast.Expr) (typ.Type, bool) {
	switch expr := expr.(type) {
	case *ast.ArithmeticOpExpr:
		return l.binaryOperationType(expr.Lhs, expr.Operator, expr.Rhs)
	case *ast.RelationalOpExpr:
		return l.binaryOperationType(expr.Lhs, expr.Operator, expr.Rhs)
	case *ast.StringConcatOpExpr:
		return l.binaryOperationType(expr.Lhs, "..", expr.Rhs)
	case *ast.LogicalOpExpr:
		return l.binaryOperationType(expr.Lhs, expr.Operator, expr.Rhs)
	case *ast.UnaryMinusOpExpr:
		return l.unaryOperationType("-", expr.Expr)
	case *ast.UnaryNotOpExpr:
		return l.unaryOperationType("not", expr.Expr)
	case *ast.UnaryLenOpExpr:
		return l.unaryOperationType("#", expr.Expr)
	case *ast.UnaryBNotOpExpr:
		return l.unaryOperationType("~", expr.Expr)
	case *ast.CastExpr:
		return l.scalarOperationType(expr.Expr)
	case *ast.NonNilAssertExpr:
		return l.scalarOperationType(expr.Expr)
	default:
		return nil, false
	}
}

func (l *lowerer) expressionOperandType(expr ast.Expr) (typ.Type, bool) {
	if t, ok := valueexpr.LiteralType(expr); ok {
		return t, true
	}
	if ident, ok := expr.(*ast.IdentExpr); ok {
		return l.identType(ident)
	}
	return l.scalarOperationType(expr)
}

func (l *lowerer) identType(expr *ast.IdentExpr) (typ.Type, bool) {
	if l == nil || l.bindings == nil || expr == nil {
		return nil, false
	}
	id, ok := l.bindings.SymbolOf(expr)
	if !ok || id == 0 {
		return nil, false
	}
	t, ok := l.symbolTypes[id]
	if ok && t != nil {
		return t, true
	}
	exprType, ok := l.bindings.SymbolTypeAnnotation(id)
	if !ok {
		return nil, false
	}
	return typeresolve.New(l.bindings).Type(exprType)
}

func (l *lowerer) binaryOperationType(leftExpr ast.Expr, op string, rightExpr ast.Expr) (typ.Type, bool) {
	left, ok := l.expressionOperandType(leftExpr)
	if !ok {
		return nil, false
	}
	right, ok := l.expressionOperandType(rightExpr)
	if !ok {
		return nil, false
	}
	return typeoperator.BinaryOp(left, op, right)
}

func (l *lowerer) unaryOperationType(op string, expr ast.Expr) (typ.Type, bool) {
	operand, ok := l.expressionOperandType(expr)
	if !ok {
		return nil, false
	}
	return typeoperator.UnaryOp(op, operand)
}
