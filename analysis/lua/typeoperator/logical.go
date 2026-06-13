package typeoperator

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

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
