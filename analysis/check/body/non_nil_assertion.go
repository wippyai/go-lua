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
	for _, point := range r.Graph().RPO() {
		if !r.PointNormallyReachable(point) {
			continue
		}
		emit := func(expr ast.Expr) bool {
			return r.forEachNonNilAssertionInExpr(point, expr, seen, visit, &visited, 0)
		}
		if fact, ok := r.LocalAssignment(point); ok {
			for _, expr := range fact.Exprs {
				if !emit(expr) {
					return true
				}
			}
		}
		if fact, ok := r.OrdinaryAssignment(point); ok {
			if !emit(fact.Value) || !emit(fact.Target) {
				return true
			}
		}
		if fact, ok := r.Call(point); ok {
			if !emit(fact.Call) {
				return true
			}
		}
		if fact, ok := r.ReturnFact(point); ok {
			for _, expr := range fact.Exprs {
				if !emit(expr) {
					return true
				}
			}
		}
		if fact, ok := r.BranchCondition(point); ok {
			if !emit(fact.Condition) {
				return true
			}
		}
	}
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
	depth int,
) bool {
	if expr == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	if assert, ok := expr.(*ast.NonNilAssertExpr); ok {
		r.forEachNonNilAssertionInExpr(point, assert.Expr, seen, visit, visited, depth+1)
		key := nonNilAssertionKey{point: point, expr: assert}
		if _, ok := seen[key]; ok {
			return true
		}
		seen[key] = struct{}{}
		item, ok := r.nonNilAssertionOccurrence(point, assert)
		if !ok {
			return true
		}
		*visited = true
		return visit(item)
	}
	return r.walkNonNilAssertionExprChildren(point, expr, seen, visit, visited, depth)
}

func (r *Result) walkNonNilAssertionExprChildren(
	point cfg.Point,
	expr ast.Expr,
	seen map[nonNilAssertionKey]struct{},
	visit func(NonNilAssertionOccurrence) bool,
	visited *bool,
	depth int,
) bool {
	next := func(child ast.Expr) bool {
		return r.forEachNonNilAssertionInExpr(point, child, seen, visit, visited, depth+1)
	}
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		if !next(e.Object) {
			return false
		}
		if e.KeySyntax == ast.AttrKeyIndex {
			return next(e.Key)
		}
	case *ast.FuncCallExpr:
		if !next(e.Func) || !next(e.Receiver) {
			return false
		}
		for _, arg := range e.Args {
			if !next(arg) {
				return false
			}
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax == ast.AttrKeyIndex && !next(field.Key) {
				return false
			}
			if !next(field.Value) {
				return false
			}
		}
	case *ast.LogicalOpExpr:
		return next(e.Lhs) && next(e.Rhs)
	case *ast.RelationalOpExpr:
		return next(e.Lhs) && next(e.Rhs)
	case *ast.StringConcatOpExpr:
		return next(e.Lhs) && next(e.Rhs)
	case *ast.ArithmeticOpExpr:
		return next(e.Lhs) && next(e.Rhs)
	case *ast.UnaryMinusOpExpr:
		return next(e.Expr)
	case *ast.UnaryNotOpExpr:
		return next(e.Expr)
	case *ast.UnaryLenOpExpr:
		return next(e.Expr)
	case *ast.UnaryBNotOpExpr:
		return next(e.Expr)
	case *ast.CastExpr:
		return next(e.Expr)
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
