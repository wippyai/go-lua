package application

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func isNilOrOptional(t typ.Type) bool {
	t = operatorSurface(t)
	if t == nil {
		return false
	}
	if _, ok := t.(*typ.Optional); ok {
		return true
	}
	return t.Kind() == kind.Nil
}

func isIntegerish(t typ.Type) bool {
	t = operatorSurface(t)
	return t != nil && subtype.IsSubtype(t, typ.Integer)
}

func isNumericType(t typ.Type) bool {
	t = operatorSurface(t)
	return t != nil && subtype.IsSubtype(t, typ.Number)
}

func isArithmeticNumeric(t typ.Type) bool {
	return isNumericType(t) || isNumericStringLiteral(t)
}

func isIntegerConvertible(t typ.Type) bool {
	t = operatorSurface(t)
	if t == nil {
		return false
	}
	if isIntegerish(t) {
		return true
	}
	if lit, ok := t.(*typ.Literal); ok {
		switch lit.Base {
		case kind.Number:
			v, ok := lit.Value.(float64)
			return ok && isIntegralFloat(v)
		case kind.String:
			v, ok := numericStringLiteral(lit)
			return ok && isIntegralFloat(v)
		}
	}
	return false
}

func isStringLike(t typ.Type) bool {
	t = operatorSurface(t)
	return t != nil && subtype.IsSubtype(t, typ.String)
}

func isConcatOperand(t typ.Type) bool {
	return isStringLike(t) || isNumericType(t)
}

func isTableLike(t typ.Type) bool {
	t = operatorSurface(t)
	if t == nil {
		return false
	}
	switch t.(type) {
	case *typ.Record, *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple:
		return true
	default:
		return typ.IsBuiltinTableTopMarker(t)
	}
}
