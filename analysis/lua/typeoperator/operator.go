package typeoperator

import (
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
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

	switch op {
	case "+", "-", "*", "/", "//", "%", "^", "&", "|", "~", "<<", ">>":
		return arithmeticBinaryOp(left, op, right)
	case "..":
		return concatBinaryOp(left, right)
	case "<", ">", "<=", ">=":
		return comparisonBinaryOp(left, op, right)
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
	return operatorOverUnion(u, depth, func(member typ.Type, depth int) (typ.Type, bool) {
		return binaryOpDepth(member, op, right, depth)
	})
}

func binaryRightUnion(left typ.Type, op string, u *typ.Union, depth int) (typ.Type, bool) {
	return operatorOverUnion(u, depth, func(member typ.Type, depth int) (typ.Type, bool) {
		return binaryOpDepth(left, op, member, depth)
	})
}

// operatorOverUnion applies a binary-operator query to each union member; every
// member must succeed, and the per-member results are normalized into one.
func operatorOverUnion(u *typ.Union, depth int, query func(member typ.Type, depth int) (typ.Type, bool)) (typ.Type, bool) {
	if u == nil || len(u.Members) == 0 {
		return nil, false
	}
	out := make([]typ.Type, 0, len(u.Members))
	for _, member := range u.Members {
		result, ok := query(member, depth+1)
		if !ok {
			return nil, false
		}
		out = append(out, result)
	}
	return normalizeOperatorResults(out...), true
}

func unaryUnion(op string, u *typ.Union, depth int) (typ.Type, bool) {
	return operatorOverUnion(u, depth, func(member typ.Type, depth int) (typ.Type, bool) {
		return unaryOpDepth(op, member, depth)
	})
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

func stopDepth(t typ.Type, depth int) bool {
	return t == nil || depth > typ.DefaultRecursionDepth
}
