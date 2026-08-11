package application

import (
	typekind "github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/program/flow/kind"
)

// BinaryResult projects the result type for one canonical Lua binary
// application. Operation identity is the Program vocabulary, never a source
// spelling reconstructed by a downstream domain.
func BinaryResult(left typ.Type, op kind.BinaryOp, right typ.Type) (typ.Type, bool) {
	if left == nil || right == nil {
		return nil, false
	}
	if op == kind.BinaryEqual || op == kind.BinaryNotEqual {
		return typ.Boolean, true
	}

	left = operatorSurface(left)
	right = operatorSurface(right)
	if left == nil || right == nil {
		return nil, false
	}
	// Concatenation result type does not depend on operand presence: `a .. b`
	// always yields string when both operands are string- or number-like. A
	// possibly-nil operand (a runtime-error risk reported separately) must not
	// make the result type unresolvable, which would silently drop the enclosing
	// assignment and leave the target reading as its pre-assignment value. Drop
	// operand optionality so concat resolves; the inner-type concat-operand check
	// still rejects non-concatenable operands such as optional tables.
	if op == kind.BinaryConcat {
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
			result, ok := binaryLeafResult(leftVariant, op, rightVariant)
			if !ok {
				return nil, false
			}
			results = append(results, result)
		}
	}
	return normalizeOperatorResults(results...), true
}

// UnaryResult projects the result type for one canonical Lua unary
// application. Operation identity is the Program vocabulary.
func UnaryResult(op kind.UnaryOp, operand typ.Type) (typ.Type, bool) {
	if operand == nil {
		return nil, false
	}
	if op == kind.UnaryNot {
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
		result, ok := unaryLeafResult(op, variant)
		if !ok {
			return nil, false
		}
		results = append(results, result)
	}
	return normalizeOperatorResults(results...), true
}

func binaryLeafResult(left typ.Type, op kind.BinaryOp, right typ.Type) (typ.Type, bool) {
	if op != kind.BinaryConcat && (isNilOrOptional(left) || isNilOrOptional(right)) {
		return nil, false
	}
	if result, ok := dynamicBinaryResult(left, op, right); ok {
		return result, true
	}

	switch op {
	case kind.BinaryAdd, kind.BinarySub, kind.BinaryMul, kind.BinaryDiv,
		kind.BinaryIDiv, kind.BinaryMod, kind.BinaryPow, kind.BinaryBitAnd,
		kind.BinaryBitOr, kind.BinaryBitXor, kind.BinaryShiftLeft, kind.BinaryShiftRight:
		return arithmeticBinaryResult(left, op, right)
	case kind.BinaryConcat:
		return concatBinaryResult(left, right)
	case kind.BinaryLess, kind.BinaryGreater, kind.BinaryLessEqual, kind.BinaryGreaterEqual:
		return comparisonBinaryResult(left, op, right)
	}

	return nil, false
}

func unaryLeafResult(op kind.UnaryOp, operand typ.Type) (typ.Type, bool) {
	if isNilOrOptional(operand) {
		return nil, false
	}
	if result, ok := dynamicUnaryResult(op, operand); ok {
		return result, true
	}

	switch op {
	case kind.UnaryNeg:
		if isIntegerish(operand) {
			return typ.Integer, true
		}
		if isArithmeticNumeric(operand) {
			return typ.Number, true
		}
	case kind.UnaryBitNot:
		if isIntegerConvertible(operand) {
			return typ.Integer, true
		}
	case kind.UnaryLen:
		if result, found, ok := unaryMetamethodReturn(operand, MetaLen); found {
			return result, ok
		}
		if isStringLike(operand) || isTableLike(operand) {
			return typ.Integer, true
		}
		return nil, false
	default:
		return nil, false
	}

	if law, ok := Unary(op); ok && law.Meta != MetaAbsent {
		result, found, ok := unaryMetamethodReturn(operand, law.Meta)
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
			if surfaceMember == nil || surfaceMember.Kind() == typekind.Nil {
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

func dynamicBinaryResult(left typ.Type, op kind.BinaryOp, right typ.Type) (typ.Type, bool) {
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

func isOrderedComparisonOp(op kind.BinaryOp) bool {
	switch op {
	case kind.BinaryLess, kind.BinaryGreater, kind.BinaryLessEqual, kind.BinaryGreaterEqual:
		return true
	default:
		return false
	}
}

func dynamicUnaryResult(op kind.UnaryOp, operand typ.Type) (typ.Type, bool) {
	switch {
	case typ.IsNever(operand):
		return typ.Never, true
	case op == kind.UnaryLen && (typ.IsAny(operand) || typ.IsUnknown(operand)):
		return typ.Integer, true
	case typ.IsAny(operand):
		return typ.Unknown, true
	case typ.IsUnknown(operand):
		return typ.Unknown, true
	default:
		return nil, false
	}
}
