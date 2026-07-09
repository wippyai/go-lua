package body

import (
	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
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
// expressions like i, i+1, 2*i, and 2*i+1, plus Lua modulo-by-length terms of
// the form (integer_expr % #container) + 1.
func (r *Result) IndexReadSafeForExpressionAtBoundary(point cfg.Point, index ast.Expr, containerPath pathdom.Path) bool {
	proof := r.arrayIndexProofAtBoundary(point, containerPath)
	if r.indexModuloLengthMatches(point, index, containerPath) {
		proof.IsModuloArrayLength = true
		return readexpr.ProveArrayIndexInBounds(proof)
	}
	if constant, ok := indexConstOperand(index); ok {
		proof.HasConstant = true
		proof.Constant = constant
		return readexpr.ProveArrayIndexInBounds(proof)
	}
	basePath, coeff, offset, ok := r.indexLinearTerm(index)
	if !ok || basePath.IsEmpty() {
		return false
	}
	proof.HasTerm = true
	proof.Term = readexpr.ArrayIndexTerm{Path: basePath, Coeff: coeff, Offset: offset}
	return readexpr.ProveArrayIndexInBounds(proof)
}

func (r *Result) indexModuloLengthMatches(point cfg.Point, index ast.Expr, containerPath pathdom.Path) bool {
	mod, ok := moduloExprPlusOne(index)
	if !ok || mod.Operator != "%" {
		return false
	}
	lenExpr, ok := mod.Rhs.(*ast.UnaryLenOpExpr)
	if !ok || lenExpr.Expr == nil {
		return false
	}
	lenPath, ok := r.ExpressionPath(lenExpr.Expr)
	if !ok || !lenPath.Equal(containerPath) {
		return false
	}
	return r.indexExpressionHasIntegerType(point, mod.Lhs)
}

func (r *Result) arrayIndexProofAtBoundary(point cfg.Point, containerPath pathdom.Path) readexpr.ArrayIndexProof {
	return readexpr.ArrayIndexProof{
		LengthKnownAtLeastOne: func() bool {
			return r.indexContainerLengthKnownAtLeastOne(point, containerPath)
		},
		LengthAtLeast: func(floor int64) bool {
			if known, ok := r.LengthFloorAtBoundary(point, containerPath); ok && known >= floor {
				return true
			}
			return r.indexContainerStaticLengthAtLeast(point, containerPath, floor)
		},
		UpperBoundLengthAtLeast: func(floor int64) bool {
			return r.indexContainerStaticLengthAtLeast(point, containerPath, floor)
		},
		NumericFloor: func(path pathdom.Path) (int64, bool) {
			return r.NumericFloorAtBoundary(point, path)
		},
		NumericCeil: func(path pathdom.Path) (int64, bool) {
			return r.NumericCeilAtBoundary(point, path)
		},
		DiffProvesLELength: func(term readexpr.ArrayIndexTerm) bool {
			return r.DiffProvesIndexLELength(point, term.Path, term.Coeff, term.Offset, containerPath)
		},
		IndexInRange: func(path pathdom.Path) bool {
			return r.IndexInRangeAtBoundary(point, path, containerPath)
		},
	}
}

func (r *Result) indexContainerLengthKnownAtLeastOne(point cfg.Point, containerPath pathdom.Path) bool {
	if floor, ok := r.LengthFloorAtBoundary(point, containerPath); ok && floor >= 1 {
		return true
	}
	value, ok := r.PathValueBeforeBoundary(point, containerPath)
	if !ok {
		return false
	}
	t, ok := typevalue.TypeOf(r.registry, value)
	return ok && readexpr.SequenceKnownNonEmpty(t)
}

func (r *Result) indexContainerStaticLengthAtLeast(point cfg.Point, containerPath pathdom.Path, floor int64) bool {
	if floor <= 0 {
		return true
	}
	value, ok := r.PathValueBeforeBoundary(point, containerPath)
	if !ok {
		return false
	}
	t, ok := typevalue.TypeOf(r.registry, value)
	return ok && readexpr.SequenceLengthKnownAtLeast(t, floor)
}

func (r *Result) indexContainerInRangeElementsNonNil(point cfg.Point, containerPath pathdom.Path) bool {
	value, ok := r.PathValueBeforeBoundary(point, containerPath)
	if !ok {
		return false
	}
	t, ok := typevalue.TypeOf(r.registry, value)
	return ok && readexpr.ArrayIndexElementsNonNil(t)
}

func (r *Result) indexExpressionHasIntegerType(point cfg.Point, expr ast.Expr) bool {
	if value, ok := r.ExpressionValueBeforeBoundary(point, expr); ok {
		return typevalue.HasIntegerType(r.registry, value)
	}
	basePath, _, _, ok := r.indexLinearTerm(expr)
	if !ok || basePath.IsEmpty() {
		return false
	}
	value, ok := r.PathValueBeforeBoundary(point, basePath)
	return ok && typevalue.HasIntegerType(r.registry, value)
}

func moduloExprPlusOne(index ast.Expr) (*ast.ArithmeticOpExpr, bool) {
	add, ok := index.(*ast.ArithmeticOpExpr)
	if !ok || add.Operator != "+" {
		return nil, false
	}
	if c, ok := indexConstOperand(add.Rhs); ok && c == 1 {
		if mod, ok := add.Lhs.(*ast.ArithmeticOpExpr); ok && mod.Operator == "%" {
			return mod, true
		}
	}
	if c, ok := indexConstOperand(add.Lhs); ok && c == 1 {
		if mod, ok := add.Rhs.(*ast.ArithmeticOpExpr); ok && mod.Operator == "%" {
			return mod, true
		}
	}
	return nil, false
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
