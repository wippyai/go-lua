package core

import "github.com/wippyai/go-lua/types/typ"

// ForEachMember calls f for each leaf member type in a composite type.
//
// For unions and intersections, recursively visits all member types.
// For other types, calls f with the type itself.
//
// The callback returns false to stop iteration early.
// ForEachMember returns true if all callbacks returned true.
//
// This is useful for checking properties that must hold for all members.
func ForEachMember(t typ.Type, f func(typ.Type) bool) bool {
	return forEachMemberDepth(t, f, 0)
}

// forEachMemberDepth recursively visits members with depth limiting.
func forEachMemberDepth(t typ.Type, f func(typ.Type) bool, depth int) bool {
	if stopDepth(t, depth) {
		return true
	}

	return typ.Visit(t, typ.Visitor[bool]{
		Union: func(u *typ.Union) bool {
			for _, m := range u.Members {
				if !forEachMemberDepth(m, f, depth+1) {
					return false
				}
			}

			return true
		},
		Intersection: func(in *typ.Intersection) bool {
			for _, m := range in.Members {
				if !forEachMemberDepth(m, f, depth+1) {
					return false
				}
			}

			return true
		},
		Default: func(t typ.Type) bool {
			return f(t)
		},
	})
}

// AllMembers returns all leaf types from a union or intersection.
//
// Flattens nested unions and intersections into a single slice of member types.
// For a non-composite type, returns a slice containing just that type.
//
// Example: (A | (B | C)) & D returns [A, B, C, D]
func AllMembers(t typ.Type) []typ.Type {
	var result []typ.Type

	ForEachMember(t, func(m typ.Type) bool {
		result = append(result, m)
		return true
	})

	return result
}
