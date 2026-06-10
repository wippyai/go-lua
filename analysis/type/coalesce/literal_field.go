package coalesce

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func extractLiteral(t typ.Type) (*typ.Literal, bool) {
	for {
		a, ok := t.(*typ.Alias)
		if !ok {
			break
		}
		t = a.Target
	}
	lit, ok := t.(*typ.Literal)
	return lit, ok
}

func literalBase(lit *typ.Literal) typ.Type {
	if lit == nil {
		return nil
	}
	switch lit.Base {
	case kind.Boolean:
		return typ.Boolean
	case kind.Integer:
		return typ.Integer
	case kind.Number:
		return typ.Number
	case kind.String:
		return typ.String
	default:
		return nil
	}
}

func literalFamilyBase(t typ.Type) (typ.Type, bool) {
	t = typ.UnwrapAnnotated(t)
	if t == nil {
		return nil, false
	}
	switch v := t.(type) {
	case *typ.Alias:
		return literalFamilyBase(v.Target)
	case *typ.Literal:
		base := literalBase(v)
		return base, base != nil
	case *typ.Union:
		var base typ.Type
		for _, member := range v.Members {
			memberBase, ok := literalFamilyBase(member)
			if !ok {
				return nil, false
			}
			if base == nil {
				base = memberBase
				continue
			}
			merged, ok := mergeLiteralFamilyBases(base, memberBase)
			if !ok {
				return nil, false
			}
			base = merged
		}
		return base, base != nil
	default:
		switch t.Kind() {
		case kind.Boolean, kind.Integer, kind.Number, kind.String:
			return t, true
		default:
			return nil, false
		}
	}
}

func mergeLiteralFamilyBases(a, b typ.Type) (typ.Type, bool) {
	if a == nil || b == nil {
		return nil, false
	}
	if typ.SameNodeOrAcyclicEqual(a, b) {
		return a, true
	}
	if (a.Kind() == kind.Integer && b.Kind() == kind.Number) ||
		(a.Kind() == kind.Number && b.Kind() == kind.Integer) {
		return typ.Number, true
	}
	return nil, false
}

func joinNonDiscriminantField(a, b typ.Type) (typ.Type, bool) {
	if typ.SameNodeOrAcyclicEqual(a, b) {
		return a, true
	}
	al, aOK := extractLiteral(a)
	bl, bOK := extractLiteral(b)
	if aOK && bOK && al.Base == bl.Base {
		if typ.LiteralEquals(al, bl) {
			return a, true
		}
		return literalBase(al), true
	}
	left, ok := literalFamilyBase(a)
	if !ok {
		return nil, false
	}
	right, ok := literalFamilyBase(b)
	if !ok {
		return nil, false
	}
	return mergeLiteralFamilyBases(left, right)
}
