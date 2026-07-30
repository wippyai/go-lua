package typeoperator

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func logicalBinaryOp(left typ.Type, op string, right typ.Type) (typ.Type, bool) {
	variants, ok := logicalLeftVariants(left)
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

func logicalLeftVariants(t typ.Type) ([]typ.Type, bool) {
	t = operatorSurface(t)
	if t == nil {
		return nil, false
	}
	switch t.(type) {
	case *typ.Optional, *typ.Union:
	default:
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
		switch v := current.(type) {
		case *typ.Optional:
			if _, cyclic := active[v]; cyclic {
				return true
			}
			active[v] = struct{}{}
			defer delete(active, v)
			out = append(out, typ.Nil)
			return visit(v.Inner)
		case *typ.Union:
			if _, cyclic := active[v]; cyclic {
				return true
			}
			active[v] = struct{}{}
			defer delete(active, v)
			if len(v.Members) == 0 {
				return false
			}
			for _, member := range v.Members {
				if !visit(member) {
					return false
				}
			}
			return true
		default:
			out = append(out, current)
			return true
		}
	}
	return out, visit(t) && len(out) != 0
}

func logicalVariantResult(left typ.Type, op string, right typ.Type) (typ.Type, bool) {
	switch {
	case typ.IsNever(left):
		return typ.Never, true
	case typ.IsAny(left):
		if op == "and" {
			return normalizeOperatorResults(typ.Nil, typ.False, right), true
		}
		return typ.Any, true
	case typ.IsUnknown(left):
		if op == "and" {
			return normalizeOperatorResults(typ.Nil, typ.False, right), true
		}
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
	t = operatorSurface(t)
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
