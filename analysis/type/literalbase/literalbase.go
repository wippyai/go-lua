package literalbase

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Extract returns a literal after following transparent aliases.
func Extract(t typ.Type) (*typ.Literal, bool) {
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

// Base maps a literal to its scalar base type.
func Base(lit *typ.Literal) typ.Type {
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

// FamilyBase returns the scalar base shared by a literal, scalar, or literal
// union family. Integer and number families merge upward to number.
func FamilyBase(t typ.Type) (typ.Type, bool) {
	t = typ.UnwrapAnnotated(t)
	if t == nil {
		return nil, false
	}
	switch v := t.(type) {
	case *typ.Alias:
		return FamilyBase(v.Target)
	case *typ.Literal:
		base := Base(v)
		return base, base != nil
	case *typ.Union:
		var base typ.Type
		for _, member := range v.Members {
			memberBase, ok := FamilyBase(member)
			if !ok {
				return nil, false
			}
			if base == nil {
				base = memberBase
				continue
			}
			merged, ok := MergeBases(base, memberBase)
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

// MergeBases returns the common scalar base for two literal family bases.
func MergeBases(a, b typ.Type) (typ.Type, bool) {
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

// JoinNonDiscriminantField preserves equal literals while widening
// non-discriminant literal families to their shared scalar base.
func JoinNonDiscriminantField(a, b typ.Type) (typ.Type, bool) {
	if typ.SameNodeOrAcyclicEqual(a, b) {
		return a, true
	}
	al, aOK := Extract(a)
	bl, bOK := Extract(b)
	if aOK && bOK && al.Base == bl.Base {
		if typ.LiteralEquals(al, bl) {
			return a, true
		}
		return Base(al), true
	}
	left, ok := FamilyBase(a)
	if !ok {
		return nil, false
	}
	right, ok := FamilyBase(b)
	if !ok {
		return nil, false
	}
	return MergeBases(left, right)
}
