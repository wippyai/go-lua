package body

import (
	"github.com/wippyai/go-lua/__legacy/analysis/domain/indexform"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	enginesourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
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
	request, ok := r.boundIndexReadForExpressionAtBoundary(point, index, containerPath)
	if !ok {
		return false
	}
	proved, projected := enginesourcevalue.BoundDynamicReadInRange(request)
	return projected && proved
}

func (r *Result) indexReadValueForExpressionAtBoundary(point cfg.Point, index ast.Expr, containerPath pathdom.Path) (product.Value, bool) {
	request, ok := r.boundIndexReadForExpressionAtBoundary(point, index, containerPath)
	if !ok {
		return product.Value{}, false
	}
	return enginesourcevalue.ReadBoundDynamicValue(request)
}

func (r *Result) boundIndexReadForExpressionAtBoundary(point cfg.Point, index ast.Expr, containerPath pathdom.Path) (enginesourcevalue.BoundDynamicRead, bool) {
	if r == nil || index == nil || r.registry == nil || r.visibility == nil || containerPath.IsEmpty() {
		return enginesourcevalue.BoundDynamicRead{}, false
	}
	form := indexform.IndexForm{}
	moduloInteger := false
	if length, ok := index.(*ast.UnaryLenOpExpr); ok && length.Expr != nil {
		if lengthPath, pathOK := r.ExpressionPath(length.Expr); pathOK && lengthPath.Equal(containerPath) {
			form, _ = indexform.NewArrayLengthIndex(containerPath)
		}
	}
	if !form.Valid() && r.indexModuloLengthMatches(point, index, containerPath) {
		form, _ = indexform.NewModuloLengthIndex(containerPath)
		moduloInteger = true
	}
	if !form.Valid() {
		if constant, constantOK := indexConstOperand(index); constantOK {
			form = indexform.NewConstantIndex(constant)
		}
	}
	if !form.Valid() {
		basePath, coeff, offset, affineOK := r.indexLinearTerm(index)
		if affineOK && !basePath.IsEmpty() {
			form, _ = indexform.NewAffineIndex(basePath, coeff, offset)
		}
	}
	if !form.Valid() {
		return enginesourcevalue.BoundDynamicRead{}, false
	}
	keyValue, ok := r.ExpressionValueAtBoundary(point, index)
	if !ok {
		keyValue = product.Top()
	}
	return r.boundIndexReadAtBoundary(point, containerPath, keyValue, form, moduloInteger)
}

func (r *Result) boundIndexReadAtBoundary(point cfg.Point, containerPath pathdom.Path, keyValue product.Value, form indexform.IndexForm, moduloInteger bool) (enginesourcevalue.BoundDynamicRead, bool) {
	if r == nil || r.registry == nil || r.visibility == nil || !form.Valid() || containerPath.IsEmpty() {
		return enginesourcevalue.BoundDynamicRead{}, false
	}
	tableValue, ok := r.PathValueAtBoundary(point, containerPath)
	if !ok {
		return enginesourcevalue.BoundDynamicRead{}, false
	}
	in, ok := r.boundaryStateAt(point)
	if !ok {
		return enginesourcevalue.BoundDynamicRead{}, false
	}
	return enginesourcevalue.BoundDynamicRead{
		Registry: r.registry, TypeValues: r.typeValues, KeySpace: r.visibility.KeySpace(),
		Visibility: r.visibility, ProofVisibility: r.visibility, Point: point,
		TablePath: containerPath, TableValue: tableValue, KeyValue: keyValue,
		ValueInput: in, IndexForm: form, ModuloInteger: moduloInteger,
	}, true
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
