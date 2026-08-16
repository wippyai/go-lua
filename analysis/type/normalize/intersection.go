package normalize

import (
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// IntersectionForMeet applies the pure intersection normalization policy used
// by semantic meet and access projection code.
func IntersectionForMeet(members ...typ.Type) typ.Type {
	flat := make([]typ.Type, 0, len(members))
	hasNever := false
	hasNil := false

	var addMember func(typ.Type)
	addMember = func(member typ.Type) {
		if member == nil {
			return
		}
		unwrapped := unwrap.Annotated(member)
		if unwrapped == nil {
			return
		}
		switch unwrapped.Kind() {
		case kind.Any:
			return
		case kind.Never:
			hasNever = true
			return
		case kind.Nil:
			hasNil = true
			return
		case kind.Intersection:
			for _, nested := range unwrapped.(*typ.Intersection).Members {
				addMember(nested)
			}
			return
		default:
			flat = append(flat, member)
		}
	}

	for _, member := range members {
		addMember(member)
	}

	if hasNever {
		return typ.Never
	}
	// Boolean literals are disjoint scalar singletons.  Retaining both here
	// manufactures an impossible frame (false & true) that later assignment
	// checks can mistake for a live declared target instead of Bottom.
	hasTrue, hasFalse := false, false
	for _, member := range flat {
		hasTrue = hasTrue || typ.TypeEquals(member, typ.LiteralBool(true))
		hasFalse = hasFalse || typ.TypeEquals(member, typ.LiteralBool(false))
	}
	if hasTrue && hasFalse {
		return typ.Never
	}

	if hasNil {
		if len(flat) == 0 {
			return typ.Nil
		}
		allAcceptNil := true
		for _, member := range flat {
			if !intersectionMemberAcceptsNil(member) {
				allAcceptNil = false
				break
			}
		}
		if allAcceptNil {
			return typ.Nil
		}
		flat = append(flat, typ.Nil)
	}

	return typ.MaterializeIntersection(flat)
}

func intersectionMemberAcceptsNil(t typ.Type) bool {
	if t == nil {
		return false
	}

	return inspect.Visit(t, inspect.Visitor[bool]{
		Optional: func(o *typ.Optional) bool {
			return true
		},
		Union: func(u *typ.Union) bool {
			for _, member := range u.Members {
				if member.Kind() == kind.Nil {
					return true
				}
			}
			return false
		},
		Intersection: func(in *typ.Intersection) bool {
			for _, member := range in.Members {
				if member.Kind() == kind.Nil {
					return true
				}
			}
			return false
		},
		Default: func(t typ.Type) bool {
			k := t.Kind()
			return k == kind.Nil || k.IsPlaceholder()
		},
	})
}
