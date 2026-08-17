// Package literal centralizes policies for recognizing and widening literal
// types without depending on higher-level type analysis packages.
package literal

import (
	graph "github.com/wippyai/go-lua/domain/type/internal/typegraph"
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// ExtractAliasOnly returns a literal after following Alias targets only.
//
// This intentionally does not unwrap annotations or any other wrappers. Callers
// that need annotation-aware behavior should unwrap before calling this helper.
func ExtractAliasOnly(t typ.Type) (*typ.Literal, bool) {
	var seen graph.Path
	for {
		a, ok := t.(*typ.Alias)
		if !ok {
			break
		}
		if !seen.Enter(a, 0) {
			return nil, false
		}
		next := a.Target
		if next == nil || next == t {
			return nil, false
		}
		t = next
	}
	lit, ok := t.(*typ.Literal)
	return lit, ok
}

// PrimitiveBase maps a literal to its primitive base singleton.
//
// It returns nil for nil literals and for literals with a non-primitive base.
func PrimitiveBase(lit *typ.Literal) typ.Type {
	if lit == nil {
		return nil
	}
	switch lit.Base() {
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

// IntegerValue extracts the exact integer carried by an integer literal type.
func IntegerValue(t typ.Type) (int64, bool) {
	lit, ok := t.(*typ.Literal)
	if !ok || lit.Base() != kind.Integer {
		return 0, false
	}
	value, ok := lit.Value().(int64)
	return value, ok
}

// FamilyBase returns the primitive family that can represent t.
//
// It accepts literal types, primitive boolean/integer/number/string types, and
// unions whose members all belong to a mergeable literal family. Annotated
// wrappers and aliases are unwrapped for this family-level policy.
func FamilyBase(t typ.Type) (typ.Type, bool) {
	return familyBase(t, &graph.Path{})
}

func familyBase(t typ.Type, active *graph.Path) (typ.Type, bool) {
	for {
		ann, ok := t.(*typ.Annotated)
		if !ok {
			break
		}
		if ann.Inner == nil || ann.Inner == t {
			return nil, false
		}
		if !active.Enter(t, 0) {
			return nil, false
		}
		defer active.Leave(t, 0)
		t = ann.Inner
	}
	if t == nil {
		return nil, false
	}
	switch v := t.(type) {
	case *typ.Alias:
		next := v.Target
		if next == nil || next == t {
			return nil, false
		}
		if !active.Enter(t, 0) {
			return nil, false
		}
		defer active.Leave(t, 0)
		return familyBase(next, active)
	case *typ.Literal:
		base := PrimitiveBase(v)
		return base, base != nil
	case *typ.Union:
		var base typ.Type
		for _, member := range v.Members {
			memberBase, ok := familyBase(member, active)
			if !ok {
				return nil, false
			}
			if base == nil {
				base = memberBase
				continue
			}
			merged, ok := MergeFamilyBases(base, memberBase)
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

// MergeFamilyBases merges two primitive literal-family base types.
//
// Equal bases preserve the left input. Integer and number merge to number;
// unrelated bases do not merge.
func MergeFamilyBases(a, b typ.Type) (typ.Type, bool) {
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
