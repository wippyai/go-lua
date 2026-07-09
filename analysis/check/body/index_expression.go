package body

import (
	"math"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
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
	if r.indexModuloLengthSafe(point, index, containerPath) {
		return true
	}
	if constant, ok := indexConstOperand(index); ok {
		return r.constantIndexReadSafeAtBoundary(point, constant, containerPath)
	}
	basePath, coeff, offset, ok := r.indexLinearTerm(index)
	if !ok || basePath.IsEmpty() {
		return false
	}
	return r.IndexReadSafeAtBoundary(point, basePath, coeff, offset, containerPath)
}

func (r *Result) constantIndexReadSafeAtBoundary(point cfg.Point, index int64, containerPath pathdom.Path) bool {
	if index < 1 {
		return false
	}
	if floor, ok := r.LengthFloorAtBoundary(point, containerPath); ok && floor >= index {
		return true
	}
	return r.indexContainerStaticLengthAtLeast(point, containerPath, index)
}

func (r *Result) indexModuloLengthSafe(point cfg.Point, index ast.Expr, containerPath pathdom.Path) bool {
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
	if !r.indexContainerLengthKnownAtLeastOne(point, containerPath) {
		return false
	}
	return r.indexExpressionHasIntegerType(point, mod.Lhs)
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
	return ok && indexContainerStaticLengthAtLeastOne(t, 0)
}

func indexContainerStaticLengthAtLeastOne(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch tt := unwrap.Alias(t).(type) {
	case *typ.Tuple:
		return len(tt.Elements) > 0
	case *typ.Record:
		member := tt.GetStaticIntIndex(1)
		return member != nil && !member.Optional
	case *typ.Optional:
		return indexContainerStaticLengthAtLeastOne(tt.Inner, depth+1)
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false
		}
		for _, member := range tt.Members {
			if !indexContainerStaticLengthAtLeastOne(member, depth+1) {
				return false
			}
		}
		return true
	default:
		return false
	}
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
	return ok && indexContainerStaticLengthAtLeast(t, floor, 0)
}

func (r *Result) indexContainerInRangeElementsNonNil(point cfg.Point, containerPath pathdom.Path) bool {
	value, ok := r.PathValueBeforeBoundary(point, containerPath)
	if !ok {
		return false
	}
	t, ok := typevalue.TypeOf(r.registry, value)
	return ok && indexContainerInRangeElementsNonNil(t, 0)
}

func indexContainerInRangeElementsNonNil(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch tt := unwrap.Alias(t).(type) {
	case *typ.Array:
		elem := tt.Element
		if elem == nil {
			elem = typ.Unknown
		}
		return !typevalue.TypeIncludesNil(elem)
	case *typ.Tuple:
		if len(tt.Elements) == 0 {
			return false
		}
		for _, elem := range tt.Elements {
			if elem == nil {
				elem = typ.Unknown
			}
			if typevalue.TypeIncludesNil(elem) {
				return false
			}
		}
		return true
	case *typ.Record:
		var found bool
		for i := int64(1); ; i++ {
			member := tt.GetStaticIntIndex(i)
			if member == nil || member.Optional {
				return found
			}
			found = true
			if typevalue.TypeIncludesNil(member.Type) {
				return false
			}
		}
	case *typ.Optional:
		return indexContainerInRangeElementsNonNil(tt.Inner, depth+1)
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false
		}
		reachable := false
		for _, member := range tt.Members {
			if !indexContainerCanHaveElement(member, depth+1) {
				continue
			}
			reachable = true
			if !indexContainerInRangeElementsNonNil(member, depth+1) {
				return false
			}
		}
		return reachable
	default:
		return false
	}
}

func indexContainerCanHaveElement(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	switch tt := unwrap.Alias(t).(type) {
	case *typ.Array:
		return true
	case *typ.Tuple:
		return len(tt.Elements) > 0
	case *typ.Record:
		member := tt.GetStaticIntIndex(1)
		return member != nil && !member.Optional
	case *typ.Optional:
		return indexContainerCanHaveElement(tt.Inner, depth+1)
	case *typ.Union:
		for _, member := range tt.Members {
			if indexContainerCanHaveElement(member, depth+1) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func indexContainerStaticLengthAtLeast(t typ.Type, floor int64, depth int) bool {
	if floor <= 0 {
		return true
	}
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch tt := unwrap.Alias(t).(type) {
	case *typ.Tuple:
		return int64(len(tt.Elements)) >= floor
	case *typ.Record:
		for i := int64(1); i <= floor; i++ {
			member := tt.GetStaticIntIndex(i)
			if member == nil || member.Optional {
				return false
			}
		}
		return true
	case *typ.Optional:
		return indexContainerStaticLengthAtLeast(tt.Inner, floor, depth+1)
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false
		}
		for _, member := range tt.Members {
			if !indexContainerStaticLengthAtLeast(member, floor, depth+1) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func checkedAffineInt64(coeff, value, offset int64) (int64, bool) {
	if coeff != 0 && (value > math.MaxInt64/coeff || value < math.MinInt64/coeff) {
		return 0, false
	}
	product := coeff * value
	if (offset > 0 && product > math.MaxInt64-offset) || (offset < 0 && product < math.MinInt64-offset) {
		return 0, false
	}
	return product + offset, true
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
