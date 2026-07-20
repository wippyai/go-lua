package body

import (
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// NonNilAssertionOccurrence is the body-owned, syntax-free projection of one
// runtime non-nil assertion expression.
type NonNilAssertionOccurrence struct {
	Point            cfg.Point
	OperandLabel     string
	OperandKey       string
	Value            product.Value
	HasValue         bool
	TypeWithPresence typ.Type
	OperandSpan      SourceSpan
	AssertionSpan    SourceSpan
}

// ForEachNonNilAssertionOccurrence visits runtime non-nil assertions in
// deterministic RPO order.
func (r *Result) ForEachNonNilAssertionOccurrence(visit func(NonNilAssertionOccurrence) bool) bool {
	if r == nil || visit == nil || r.Graph() == nil {
		return false
	}
	seen := make(map[nonNilAssertionKey]struct{})
	visited := false
	r.ForEachReachableExpressionUse(func(use ExpressionUse) bool {
		return r.forEachNonNilAssertionInExpr(use.Point, use.Expr, seen, visit, &visited)
	})
	return visited
}

type nonNilAssertionKey struct {
	point cfg.Point
	expr  *ast.NonNilAssertExpr
}

func (r *Result) forEachNonNilAssertionInExpr(
	point cfg.Point,
	expr ast.Expr,
	seen map[nonNilAssertionKey]struct{},
	visit func(NonNilAssertionOccurrence) bool,
	visited *bool,
) bool {
	type frame struct {
		expr ast.Expr
		exit bool
	}
	stack := []frame{{expr: expr}}
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current.expr == nil {
			continue
		}
		if !current.exit {
			stack = append(stack, frame{expr: current.expr, exit: true})
			children := adviceClaimChildren(current.expr)
			for i := len(children) - 1; i >= 0; i-- {
				stack = append(stack, frame{expr: children[i]})
			}
			continue
		}
		assert, ok := current.expr.(*ast.NonNilAssertExpr)
		if !ok {
			continue
		}
		key := nonNilAssertionKey{point: point, expr: assert}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		if item, present := r.nonNilAssertionOccurrence(point, assert); present {
			*visited = true
			if !visit(item) {
				return false
			}
		}
	}
	return true
}

func (r *Result) nonNilAssertionOccurrence(point cfg.Point, assert *ast.NonNilAssertExpr) (NonNilAssertionOccurrence, bool) {
	if assert == nil || assert.Expr == nil {
		return NonNilAssertionOccurrence{}, false
	}
	value, valueOK := r.ExpressionValueAtBoundary(point, assert.Expr)
	t, typeOK := r.nonNilAssertionOperandType(point, assert.Expr, value, valueOK)
	if !typeOK || t == nil {
		return NonNilAssertionOccurrence{}, false
	}
	return NonNilAssertionOccurrence{
		Point:            point,
		OperandLabel:     ExpressionLabel(assert.Expr),
		OperandKey:       expressionKey(point, assert.Expr),
		Value:            value,
		HasValue:         valueOK,
		TypeWithPresence: t,
		OperandSpan:      sourceSpanFromAST(ast.SpanOf(assert.Expr)),
		AssertionSpan:    sourceSpanFromAST(ast.SpanOf(assert)),
	}, true
}

func (r *Result) nonNilAssertionOperandType(point cfg.Point, operand ast.Expr, value product.Value, valueOK bool) (typ.Type, bool) {
	if valueOK {
		if t, ok := r.ValueTypeWithPresence(value); ok && t != nil {
			return t, true
		}
	}
	return r.ExpressionTypeBeforeBoundary(point, operand)
}
