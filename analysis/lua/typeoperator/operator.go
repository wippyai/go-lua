package typeoperator

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// BinaryOp projects the result type for a Lua binary operator.
func BinaryOp(left typ.Type, op string, right typ.Type) (typ.Type, bool) {
	if left == nil || right == nil {
		return nil, false
	}
	if op == "==" || op == "~=" {
		return typ.Boolean, true
	}

	left = operatorSurface(left)
	right = operatorSurface(right)
	if left == nil || right == nil {
		return nil, false
	}
	if isLogicalOp(op) {
		return logicalBinaryOp(left, op, right)
	}

	// Concatenation result type does not depend on operand presence: `a .. b`
	// always yields string when both operands are string- or number-like. A
	// possibly-nil operand (a runtime-error risk reported separately) must not
	// make the result type unresolvable, which would silently drop the enclosing
	// assignment and leave the target reading as its pre-assignment value. Drop
	// operand optionality so concat resolves; the inner-type concat-operand check
	// still rejects non-concatenable operands such as optional tables.
	if op == ".." {
		left = dropNilForConcat(left)
		right = dropNilForConcat(right)
	}

	leftVariants, ok := operatorUnionVariants(left)
	if !ok {
		return nil, false
	}
	rightVariants, ok := operatorUnionVariants(right)
	if !ok {
		return nil, false
	}
	results := make([]typ.Type, 0, len(leftVariants)*len(rightVariants))
	for _, leftVariant := range leftVariants {
		for _, rightVariant := range rightVariants {
			result, ok := binaryLeafOp(leftVariant, op, rightVariant)
			if !ok {
				return nil, false
			}
			results = append(results, result)
		}
	}
	return normalizeOperatorResults(results...), true
}

// UnaryOp projects the result type for a Lua unary operator.
func UnaryOp(op string, operand typ.Type) (typ.Type, bool) {
	if operand == nil {
		return nil, false
	}
	if op == "not" {
		return typ.Boolean, true
	}
	operand = operatorSurface(operand)
	if operand == nil {
		return nil, false
	}
	variants, ok := operatorUnionVariants(operand)
	if !ok {
		return nil, false
	}
	results := make([]typ.Type, 0, len(variants))
	for _, variant := range variants {
		result, ok := unaryLeafOp(op, variant)
		if !ok {
			return nil, false
		}
		results = append(results, result)
	}
	return normalizeOperatorResults(results...), true
}

func binaryLeafOp(left typ.Type, op string, right typ.Type) (typ.Type, bool) {
	if op != ".." && (isNilOrOptional(left) || isNilOrOptional(right)) {
		return nil, false
	}
	if result, ok := dynamicBinaryResult(left, op, right); ok {
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

func unaryLeafOp(op string, operand typ.Type) (typ.Type, bool) {
	if isNilOrOptional(operand) {
		return nil, false
	}
	if result, ok := dynamicUnaryResult(op, operand); ok {
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

// dropNilForConcat removes the nil arm of a possibly-nil operand so concat is
// judged by its present-value type. It handles both the *typ.Optional shape
// (array element reads) and a union carrying a nil member. A bare nil is left
// unchanged so concat of a definitely-nil operand still fails as a genuine
// error.
func dropNilForConcat(t typ.Type) typ.Type {
	surface := operatorSurface(t)
	switch v := surface.(type) {
	case *typ.Optional:
		return v.Inner
	case *typ.Union:
		members := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			surfaceMember := operatorSurface(member)
			if surfaceMember == nil || surfaceMember.Kind() == kind.Nil {
				continue
			}
			members = append(members, member)
		}
		if len(members) == 0 || len(members) == len(v.Members) {
			return t
		}
		return normalize.UnionForEvidence(members...)
	default:
		return t
	}
}

func operatorSurface(t typ.Type) typ.Type {
	var inline [8]typ.Type
	inlineN := 0
	var overflow map[typ.Type]struct{}
	for t != nil {
		seen := false
		for i := 0; i < inlineN; i++ {
			if inline[i] == t {
				seen = true
				break
			}
		}
		if !seen && overflow != nil {
			_, seen = overflow[t]
		}
		if seen {
			return nil
		}
		if inlineN < len(inline) {
			inline[inlineN] = t
			inlineN++
		} else {
			if overflow == nil {
				overflow = make(map[typ.Type]struct{})
			}
			overflow[t] = struct{}{}
		}
		t = unwrap.Annotated(t)
		switch v := t.(type) {
		case *typ.Alias:
			next := v.UnaliasedTarget()
			if next == nil || next == t {
				return nil
			}
			t = next
		case *typ.Instantiated:
			expanded := subst.ExpandInstantiated(v)
			if expanded == nil || expanded == t {
				return nil
			}
			t = expanded
		default:
			return t
		}
	}
	return nil
}

// operatorUnionVariants computes the finite regular-graph basis of a union.
// A recursive backedge contributes no new member; success requires at least
// one productive leaf. This is the exact least solution of union expansion,
// rather than an arbitrary unfolding-depth approximation.
func operatorUnionVariants(t typ.Type) ([]typ.Type, bool) {
	t = operatorSurface(t)
	if t == nil {
		return nil, false
	}
	if _, union := t.(*typ.Union); !union {
		return []typ.Type{t}, true
	}
	out := make([]typ.Type, 0, 1)
	active := make(map[typ.Type]struct{})
	var visit func(typ.Type) bool
	visit = func(current typ.Type) bool {
		current = operatorSurface(current)
		if current == nil {
			return false
		}
		union, ok := current.(*typ.Union)
		if !ok {
			out = append(out, current)
			return true
		}
		if _, cyclic := active[union]; cyclic {
			return true
		}
		active[union] = struct{}{}
		defer delete(active, union)
		if len(union.Members) == 0 {
			return false
		}
		for _, member := range union.Members {
			if !visit(member) {
				return false
			}
		}
		return true
	}
	return out, visit(t) && len(out) != 0
}

func normalizeOperatorResults(results ...typ.Type) typ.Type {
	hasAny := false
	hasUnknown := false
	for _, result := range results {
		result = operatorSurface(result)
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

func dynamicBinaryResult(left typ.Type, op string, right typ.Type) (typ.Type, bool) {
	switch {
	case typ.IsNever(left) || typ.IsNever(right):
		return typ.Never, true
	case isOrderedComparisonOp(op) && (typ.IsAny(left) || typ.IsAny(right) || typ.IsUnknown(left) || typ.IsUnknown(right)):
		return typ.Boolean, true
	case typ.IsAny(left) || typ.IsAny(right):
		return typ.Unknown, true
	case typ.IsUnknown(left) || typ.IsUnknown(right):
		return typ.Unknown, true
	default:
		return nil, false
	}
}

func isOrderedComparisonOp(op string) bool {
	switch op {
	case "<", ">", "<=", ">=":
		return true
	default:
		return false
	}
}

func dynamicUnaryResult(op string, operand typ.Type) (typ.Type, bool) {
	switch {
	case typ.IsNever(operand):
		return typ.Never, true
	case op == "#" && (typ.IsAny(operand) || typ.IsUnknown(operand)):
		return typ.Integer, true
	case typ.IsAny(operand):
		return typ.Unknown, true
	case typ.IsUnknown(operand):
		return typ.Unknown, true
	default:
		return nil, false
	}
}
