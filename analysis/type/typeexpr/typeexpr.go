// Package typeexpr constructs semantic type-expression shapes.
package typeexpr

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Optional constructs the semantic optional expression for inner.
func Optional(inner typ.Type) typ.Type {
	if inner == nil || inner.Kind() == kind.Nil {
		return typ.Nil
	}

	if inner.Kind() == kind.Optional {
		return inner
	}

	if typ.IsAny(inner) {
		return typ.Any
	}

	if inner.Kind() == kind.Union {
		u, ok := unwrapAnnotated(inner).(*typ.Union)
		if !ok {
			return typ.MaterializeOptional(inner)
		}
		members := make([]typ.Type, 0, len(u.Members)+1)
		members = append(members, typ.Nil)
		members = append(members, u.Members...)
		return typ.MaterializeUnion(members)
	}

	return typ.MaterializeOptional(inner)
}

// Union constructs the semantic union expression for members.
func Union(members ...typ.Type) typ.Type {
	if len(members) == 0 {
		return typ.Never
	}

	flat := make([]typ.Type, 0, len(members))
	hasNil := false

	var addMember func(typ.Type)
	addMember = func(member typ.Type) {
		unwrapped := unwrapAnnotatedOrNil(member)
		if unwrapped == nil {
			return
		}

		switch unwrapped.Kind() {
		case kind.Nil:
			hasNil = true
		case kind.Union:
			u, ok := unwrapped.(*typ.Union)
			if !ok {
				flat = append(flat, member)
				return
			}
			for _, nested := range u.Members {
				addMember(nested)
			}
		case kind.Optional:
			o, ok := unwrapped.(*typ.Optional)
			if !ok {
				flat = append(flat, member)
				return
			}
			hasNil = true
			addMember(o.Inner)
		default:
			flat = append(flat, member)
		}
	}

	for _, member := range members {
		addMember(member)
	}

	nonNil := typ.MaterializeUnion(flat)
	if !hasNil {
		return nonNil
	}

	unique := materializedMembers(flat, nonNil)
	switch len(unique) {
	case 0:
		return typ.Nil
	case 1:
		return typ.MaterializeOptional(unique[0])
	default:
		withNil := make([]typ.Type, 0, len(unique)+1)
		withNil = append(withNil, typ.Nil)
		withNil = append(withNil, unique...)
		return typ.MaterializeUnion(withNil)
	}
}

// Intersection constructs the semantic intersection expression for members.
func Intersection(members ...typ.Type) typ.Type {
	if len(members) == 0 {
		return typ.Any
	}

	flat := make([]typ.Type, 0, len(members))

	var addMember func(typ.Type)
	addMember = func(member typ.Type) {
		unwrapped := unwrapAnnotatedOrNil(member)
		if unwrapped == nil {
			return
		}

		if unwrapped.Kind() == kind.Intersection {
			i, ok := unwrapped.(*typ.Intersection)
			if !ok {
				flat = append(flat, member)
				return
			}
			for _, nested := range i.Members {
				addMember(nested)
			}
			return
		}

		flat = append(flat, member)
	}

	for _, member := range members {
		addMember(member)
	}

	return typ.MaterializeIntersection(flat)
}

func materializedMembers(raw []typ.Type, materialized typ.Type) []typ.Type {
	if len(raw) == 0 {
		return nil
	}
	if u, ok := materialized.(*typ.Union); ok {
		return u.Members
	}
	return []typ.Type{materialized}
}

func unwrapAnnotatedOrNil(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	return unwrapAnnotated(t)
}

func unwrapAnnotated(t typ.Type) typ.Type {
	if annotated, ok := t.(*typ.Annotated); ok {
		if annotated.Inner == nil {
			return typ.Unknown
		}
		return annotated.Inner
	}
	return t
}
