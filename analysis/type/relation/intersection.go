package relation

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	. "github.com/wippyai/go-lua/analysis/type/typ"
)

// NormalizeIntersectionForMeet applies semantic meet policy explicitly requested
// by relation and projection code.
func NormalizeIntersectionForMeet(members ...Type) Type {
	flat := make([]Type, 0, len(members))
	hasNever := false
	hasNil := false

	var addMember func(Type)
	addMember = func(member Type) {
		if member == nil {
			return
		}
		unwrapped := UnwrapAnnotated(member)
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
			for _, nested := range unwrapped.(*Intersection).Members {
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
		return Never
	}

	if hasNil {
		if len(flat) == 0 {
			return Nil
		}
		allAcceptNil := true
		for _, member := range flat {
			if !intersectionMemberAcceptsNil(member) {
				allAcceptNil = false
				break
			}
		}
		if allAcceptNil {
			return Nil
		}
		flat = append(flat, Nil)
	}

	return NewIntersection(flat...)
}

func intersectionMemberAcceptsNil(t Type) bool {
	if t == nil {
		return false
	}

	return Visit(t, Visitor[bool]{
		Optional: func(o *Optional) bool {
			return true
		},
		Union: func(u *Union) bool {
			for _, member := range u.Members {
				if member.Kind() == kind.Nil {
					return true
				}
			}
			return false
		},
		Intersection: func(in *Intersection) bool {
			for _, member := range in.Members {
				if member.Kind() == kind.Nil {
					return true
				}
			}
			return false
		},
		Default: func(t Type) bool {
			k := t.Kind()
			return k == kind.Nil || k.IsPlaceholder()
		},
	})
}
