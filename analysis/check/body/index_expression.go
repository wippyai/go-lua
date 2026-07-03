package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse/numparse"
)

// NumericIndexExpressionTypeAtBoundary reports when expr is known to be numeric
// enough for Lua index reasoning at point. Numeric-for indices and paths with a
// numeric floor are both canonical body facts.
func (r *Result) NumericIndexExpressionTypeAtBoundary(point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	indexPath, ok := r.indexExpressionPath(expr)
	if !ok || indexPath.Symbol == 0 {
		return nil, false
	}
	graph := r.Graph()
	if graph != nil {
		for _, loopPoint := range graph.RPO() {
			fact, ok := r.NumericFor(loopPoint)
			if ok && fact.HasSymbol && fact.Symbol == indexPath.Symbol {
				return typ.Number, true
			}
		}
	}
	if _, ok := r.NumericFloorAtBoundary(point, indexPath); ok {
		return typ.Number, true
	}
	return nil, false
}

// IndexReadSafeForExpressionAtBoundary reports whether index is proven both
// positive and in range for containerPath at point. It accepts affine index
// expressions like i, i+1, 2*i, and 2*i+1.
func (r *Result) IndexReadSafeForExpressionAtBoundary(point cfg.Point, index ast.Expr, containerPath pathdom.Path) bool {
	basePath, coeff, offset, ok := r.indexLinearTerm(index)
	if !ok || basePath.IsEmpty() {
		return false
	}
	return r.IndexReadSafeAtBoundary(point, basePath, coeff, offset, containerPath)
}

func (r *Result) indexLinearTerm(index ast.Expr) (pathdom.Path, int64, int64, bool) {
	if e, ok := index.(*ast.ArithmeticOpExpr); ok {
		switch e.Operator {
		case "+", "-":
			if base, coeff, ok := r.indexScaledPath(e.Lhs); ok {
				if c, ok := indexConstOperand(e.Rhs); ok {
					if e.Operator == "-" {
						c = -c
					}
					return base, coeff, c, true
				}
			}
			if e.Operator == "+" {
				if base, coeff, ok := r.indexScaledPath(e.Rhs); ok {
					if c, ok := indexConstOperand(e.Lhs); ok {
						return base, coeff, c, true
					}
				}
			}
			return pathdom.Path{}, 0, 0, false
		case "*":
			if base, coeff, ok := r.indexScaledPath(e); ok {
				return base, coeff, 0, true
			}
			return pathdom.Path{}, 0, 0, false
		}
		return pathdom.Path{}, 0, 0, false
	}
	if p, ok := r.indexExpressionPath(index); ok && !p.IsEmpty() {
		return p, 1, 0, true
	}
	return pathdom.Path{}, 0, 0, false
}

func (r *Result) indexScaledPath(expr ast.Expr) (pathdom.Path, int64, bool) {
	if e, ok := expr.(*ast.ArithmeticOpExpr); ok && e.Operator == "*" {
		if c, ok := indexConstOperand(e.Lhs); ok && c > 0 {
			if p, ok := r.indexExpressionPath(e.Rhs); ok && !p.IsEmpty() {
				return p, c, true
			}
		}
		if c, ok := indexConstOperand(e.Rhs); ok && c > 0 {
			if p, ok := r.indexExpressionPath(e.Lhs); ok && !p.IsEmpty() {
				return p, c, true
			}
		}
		return pathdom.Path{}, 0, false
	}
	if p, ok := r.indexExpressionPath(expr); ok && !p.IsEmpty() {
		return p, 1, true
	}
	return pathdom.Path{}, 0, false
}

func (r *Result) indexExpressionPath(expr ast.Expr) (pathdom.Path, bool) {
	if r == nil || expr == nil {
		return pathdom.Path{}, false
	}
	return r.ExpressionPath(expr)
}

func indexConstOperand(expr ast.Expr) (int64, bool) {
	num, ok := expr.(*ast.NumberExpr)
	if !ok {
		return 0, false
	}
	return numparse.ParseIntegerLiteral(num.Value)
}
