package typeoperator

import (
	"math"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/lua/typeaccess"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// BinaryOp projects the result type for a Lua binary operator.
func BinaryOp(left typ.Type, op string, right typ.Type) (typ.Type, bool) {
	return binaryOpDepth(left, op, right, 0)
}

// UnaryOp projects the result type for a Lua unary operator.
func UnaryOp(op string, operand typ.Type) (typ.Type, bool) {
	return unaryOpDepth(op, operand, 0)
}

func binaryOpDepth(left typ.Type, op string, right typ.Type, depth int) (typ.Type, bool) {
	if stopDepth(left, depth) || stopDepth(right, depth) {
		return nil, false
	}
	if op == "==" || op == "~=" {
		return typ.Boolean, true
	}

	left = operatorSurface(left, depth+1)
	right = operatorSurface(right, depth+1)
	if stopDepth(left, depth) || stopDepth(right, depth) {
		return nil, false
	}

	if isLogicalOp(op) {
		return logicalBinaryOp(left, op, right, depth+1)
	}
	if u, ok := left.(*typ.Union); ok {
		return binaryLeftUnion(u, op, right, depth+1)
	}
	if u, ok := right.(*typ.Union); ok {
		return binaryRightUnion(left, op, u, depth+1)
	}
	if isNilOrOptional(left) || isNilOrOptional(right) {
		return nil, false
	}
	if result, ok := dynamicBinaryResult(left, right); ok {
		return result, true
	}
	if result, ok := primitiveBinaryOp(left, op, right); ok {
		return result, true
	}
	if name, ok := binaryMetamethodName(op); ok {
		if op == ">" || op == ">=" {
			return binaryMetamethodReturn(right, name, left)
		}
		return binaryMetamethodReturn(left, name, right)
	}
	return nil, false
}

func unaryOpDepth(op string, operand typ.Type, depth int) (typ.Type, bool) {
	if stopDepth(operand, depth) {
		return nil, false
	}
	if op == "not" {
		return typ.Boolean, true
	}

	operand = operatorSurface(operand, depth+1)
	if stopDepth(operand, depth) {
		return nil, false
	}

	if u, ok := operand.(*typ.Union); ok {
		return unaryUnion(op, u, depth+1)
	}
	if isNilOrOptional(operand) {
		return nil, false
	}
	if result, ok := dynamicUnaryResult(operand); ok {
		return result, true
	}

	switch op {
	case "-":
		if isIntegerish(operand) {
			return typ.Integer, true
		}
		if isArithmeticNumeric(operand) {
			return typ.Number, true
		}
	case "~":
		if isIntegerConvertible(operand) {
			return typ.Integer, true
		}
	case "#":
		if result, found, ok := unaryMetamethodReturn(operand, "__len"); found {
			return result, ok
		}
		if isStringLike(operand) || isTableLike(operand) {
			return typ.Integer, true
		}
		return nil, false
	default:
		return nil, false
	}

	if name, ok := unaryMetamethodName(op); ok {
		result, found, ok := unaryMetamethodReturn(operand, name)
		if found {
			return result, ok
		}
	}
	return nil, false
}

func operatorSurface(t typ.Type, depth int) typ.Type {
	for !stopDepth(t, depth) {
		t = unwrap.Annotated(t)
		switch v := t.(type) {
		case *typ.Alias:
			next := v.UnaliasedTarget()
			if next == nil || next == t {
				return next
			}
			t = next
		case *typ.Instantiated:
			expanded := subst.ExpandInstantiated(v)
			if expanded == nil || expanded == t {
				return t
			}
			t = expanded
		default:
			return t
		}
		depth++
	}
	return nil
}

func binaryLeftUnion(u *typ.Union, op string, right typ.Type, depth int) (typ.Type, bool) {
	if u == nil || len(u.Members) == 0 {
		return nil, false
	}
	out := make([]typ.Type, 0, len(u.Members))
	for _, member := range u.Members {
		result, ok := binaryOpDepth(member, op, right, depth+1)
		if !ok {
			return nil, false
		}
		out = append(out, result)
	}
	return normalizeOperatorResults(out...), true
}

func binaryRightUnion(left typ.Type, op string, u *typ.Union, depth int) (typ.Type, bool) {
	if u == nil || len(u.Members) == 0 {
		return nil, false
	}
	out := make([]typ.Type, 0, len(u.Members))
	for _, member := range u.Members {
		result, ok := binaryOpDepth(left, op, member, depth+1)
		if !ok {
			return nil, false
		}
		out = append(out, result)
	}
	return normalizeOperatorResults(out...), true
}

func unaryUnion(op string, u *typ.Union, depth int) (typ.Type, bool) {
	if u == nil || len(u.Members) == 0 {
		return nil, false
	}
	out := make([]typ.Type, 0, len(u.Members))
	for _, member := range u.Members {
		result, ok := unaryOpDepth(op, member, depth+1)
		if !ok {
			return nil, false
		}
		out = append(out, result)
	}
	return normalizeOperatorResults(out...), true
}

func logicalBinaryOp(left typ.Type, op string, right typ.Type, depth int) (typ.Type, bool) {
	variants, ok := logicalLeftVariants(left, depth+1)
	if !ok {
		return nil, false
	}
	out := make([]typ.Type, 0, len(variants))
	for _, variant := range variants {
		result, ok := logicalVariantResult(variant, op, right)
		if !ok {
			return nil, false
		}
		out = append(out, result)
	}
	return normalizeOperatorResults(out...), true
}

func logicalLeftVariants(t typ.Type, depth int) ([]typ.Type, bool) {
	if stopDepth(t, depth) {
		return nil, false
	}
	t = operatorSurface(t, depth+1)
	switch v := t.(type) {
	case *typ.Optional:
		inner, ok := logicalLeftVariants(v.Inner, depth+1)
		if !ok {
			return nil, false
		}
		return append([]typ.Type{typ.Nil}, inner...), true
	case *typ.Union:
		out := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			variants, ok := logicalLeftVariants(member, depth+1)
			if !ok {
				return nil, false
			}
			out = append(out, variants...)
		}
		return out, true
	default:
		return []typ.Type{t}, true
	}
}

func logicalVariantResult(left typ.Type, op string, right typ.Type) (typ.Type, bool) {
	switch {
	case typ.IsNever(left):
		return typ.Never, true
	case typ.IsAny(left):
		return typ.Any, true
	case typ.IsUnknown(left):
		return typ.Unknown, true
	}

	switch truthinessOf(left) {
	case truthFalse:
		if op == "and" {
			return left, true
		}
		return right, true
	case truthTrue:
		if op == "and" {
			return right, true
		}
		return left, true
	case truthBoolean:
		if op == "and" {
			return normalizeOperatorResults(typ.False, right), true
		}
		return normalizeOperatorResults(typ.True, right), true
	default:
		return typ.Unknown, true
	}
}

type truthiness uint8

const (
	truthUnknown truthiness = iota
	truthFalse
	truthTrue
	truthBoolean
)

func truthinessOf(t typ.Type) truthiness {
	t = operatorSurface(t, 0)
	if t == nil {
		return truthUnknown
	}
	if lit, ok := t.(*typ.Literal); ok && lit.Base == kind.Boolean {
		v, ok := lit.Value.(bool)
		if !ok {
			return truthUnknown
		}
		if v {
			return truthTrue
		}
		return truthFalse
	}
	switch t.Kind() {
	case kind.Nil:
		return truthFalse
	case kind.Boolean:
		return truthBoolean
	case kind.Any, kind.Unknown, kind.Never, kind.Optional, kind.Union, kind.TypeParam, kind.Ref:
		return truthUnknown
	default:
		return truthTrue
	}
}

func isLogicalOp(op string) bool {
	return op == "and" || op == "or"
}

func normalizeOperatorResults(results ...typ.Type) typ.Type {
	hasAny := false
	hasUnknown := false
	for _, result := range results {
		result = operatorSurface(result, 0)
		switch {
		case typ.IsAny(result):
			hasAny = true
		case typ.IsUnknown(result):
			hasUnknown = true
		}
	}
	if hasAny {
		return typ.Any
	}
	if hasUnknown {
		return typ.Unknown
	}
	return normalize.UnionForEvidence(results...)
}

func dynamicBinaryResult(left, right typ.Type) (typ.Type, bool) {
	switch {
	case typ.IsNever(left) || typ.IsNever(right):
		return typ.Never, true
	case typ.IsAny(left) || typ.IsAny(right):
		return typ.Unknown, true
	case typ.IsUnknown(left) || typ.IsUnknown(right):
		return typ.Unknown, true
	default:
		return nil, false
	}
}

func dynamicUnaryResult(operand typ.Type) (typ.Type, bool) {
	switch {
	case typ.IsNever(operand):
		return typ.Never, true
	case typ.IsAny(operand):
		return typ.Unknown, true
	case typ.IsUnknown(operand):
		return typ.Unknown, true
	default:
		return nil, false
	}
}

func primitiveBinaryOp(left typ.Type, op string, right typ.Type) (typ.Type, bool) {
	switch op {
	case "+", "-", "*", "%":
		if isIntegerish(left) && isIntegerish(right) {
			return typ.Integer, true
		}
		if isArithmeticNumeric(left) && isArithmeticNumeric(right) {
			return typ.Number, true
		}
	case "/", "^":
		if isArithmeticNumeric(left) && isArithmeticNumeric(right) {
			return typ.Number, true
		}
	case "//":
		if isArithmeticNumeric(left) && isArithmeticNumeric(right) {
			return typ.Integer, true
		}
	case "&", "|", "~", "<<", ">>":
		if isIntegerConvertible(left) && isIntegerConvertible(right) {
			return typ.Integer, true
		}
	case "..":
		if isConcatOperand(left) && isConcatOperand(right) {
			return typ.String, true
		}
	case "<", ">", "<=", ">=":
		if (isNumericType(left) && isNumericType(right)) ||
			(isStringLike(left) && isStringLike(right)) {
			return typ.Boolean, true
		}
	}
	return nil, false
}

func binaryMetamethodName(op string) (string, bool) {
	switch op {
	case "+":
		return "__add", true
	case "-":
		return "__sub", true
	case "*":
		return "__mul", true
	case "/":
		return "__div", true
	case "//":
		return "__idiv", true
	case "%":
		return "__mod", true
	case "^":
		return "__pow", true
	case "&":
		return "__band", true
	case "|":
		return "__bor", true
	case "~":
		return "__bxor", true
	case "<<":
		return "__shl", true
	case ">>":
		return "__shr", true
	case "..":
		return "__concat", true
	case "<", ">":
		return "__lt", true
	case "<=", ">=":
		return "__le", true
	default:
		return "", false
	}
}

func unaryMetamethodName(op string) (string, bool) {
	switch op {
	case "-":
		return "__unm", true
	case "~":
		return "__bnot", true
	case "#":
		return "__len", true
	default:
		return "", false
	}
}

func binaryMetamethodReturn(first typ.Type, name string, second typ.Type) (typ.Type, bool) {
	if result, found, ok := metamethodReturn(first, name); found {
		return result, ok
	}
	if result, found, ok := metamethodReturn(second, name); found {
		return result, ok
	}
	return nil, false
}

func unaryMetamethodReturn(operand typ.Type, name string) (typ.Type, bool, bool) {
	return metamethodReturn(operand, name)
}

func metamethodReturn(t typ.Type, name string) (typ.Type, bool, bool) {
	mt, found := typeaccess.GetMetamethod(t, name)
	if !found {
		return nil, false, false
	}
	result, ok := typeaccess.CallableReturn(mt)
	if !ok {
		return nil, true, false
	}
	return result, true, true
}

func isNilOrOptional(t typ.Type) bool {
	t = operatorSurface(t, 0)
	if t == nil {
		return false
	}
	if _, ok := t.(*typ.Optional); ok {
		return true
	}
	return t.Kind() == kind.Nil
}

func isIntegerish(t typ.Type) bool {
	t = operatorSurface(t, 0)
	return t != nil && subtype.IsSubtype(t, typ.Integer)
}

func isNumericType(t typ.Type) bool {
	t = operatorSurface(t, 0)
	return t != nil && subtype.IsSubtype(t, typ.Number)
}

func isArithmeticNumeric(t typ.Type) bool {
	return isNumericType(t) || isNumericStringLiteral(t)
}

func isIntegerConvertible(t typ.Type) bool {
	t = operatorSurface(t, 0)
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
	t = operatorSurface(t, 0)
	return t != nil && subtype.IsSubtype(t, typ.String)
}

func isConcatOperand(t typ.Type) bool {
	return isStringLike(t) || isNumericType(t)
}

func isTableLike(t typ.Type) bool {
	t = operatorSurface(t, 0)
	if t == nil {
		return false
	}
	switch t.(type) {
	case *typ.Record, *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple:
		return true
	default:
		return typetable.IsBuiltinTopMarker(t)
	}
}

func isNumericStringLiteral(t typ.Type) bool {
	t = operatorSurface(t, 0)
	lit, ok := t.(*typ.Literal)
	if !ok || lit.Base != kind.String {
		return false
	}
	_, ok = numericStringLiteral(lit)
	return ok
}

func numericStringLiteral(lit *typ.Literal) (float64, bool) {
	if lit == nil || lit.Base != kind.String {
		return 0, false
	}
	value, ok := lit.Value.(string)
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return n, true
}

func isIntegralFloat(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v == math.Trunc(v)
}

func stopDepth(t typ.Type, depth int) bool {
	return t == nil || depth > typ.DefaultRecursionDepth
}
