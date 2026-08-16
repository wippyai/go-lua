package application

import (
	typekind "github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program/flow/kind"
)

// SelectResult projects the type of Lua's short-circuit value selection.
// Selection is a distinct Program relation, not a binary operator spelling.
func SelectResult(left typ.Type, op kind.SelectOp, right typ.Type) (typ.Type, bool) {
	if op != kind.SelectAnd && op != kind.SelectOr {
		return nil, false
	}
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

func logicalVariantResult(left typ.Type, op kind.SelectOp, right typ.Type) (typ.Type, bool) {
	switch {
	case typ.IsNever(left):
		return typ.Never, true
	case typ.IsAny(left):
		if op == kind.SelectAnd {
			return normalizeOperatorResults(typ.Nil, typ.LiteralBool(false), right), true
		}
		return typ.Any, true
	case typ.IsUnknown(left):
		if op == kind.SelectAnd {
			return normalizeOperatorResults(typ.Nil, typ.LiteralBool(false), right), true
		}
		return typ.Unknown, true
	}

	switch truthinessOf(left) {
	case truthFalse:
		if op == kind.SelectAnd {
			return left, true
		}
		return right, true
	case truthTrue:
		if op == kind.SelectAnd {
			return right, true
		}
		return left, true
	case truthBoolean:
		if op == kind.SelectAnd {
			return normalizeOperatorResults(typ.LiteralBool(false), right), true
		}
		return normalizeOperatorResults(typ.LiteralBool(true), right), true
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
	if lit, ok := t.(*typ.Literal); ok && lit.Base() == typekind.Boolean {
		v, ok := lit.Value().(bool)
		if !ok {
			return truthUnknown
		}
		if v {
			return truthTrue
		}
		return truthFalse
	}
	switch t.Kind() {
	case typekind.Nil:
		return truthFalse
	case typekind.Boolean:
		return truthBoolean
	case typekind.Any, typekind.Unknown, typekind.Never, typekind.Optional, typekind.Union, typekind.TypeParam, typekind.Ref:
		return truthUnknown
	default:
		return truthTrue
	}
}
