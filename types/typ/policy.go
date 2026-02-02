package typ

import "github.com/wippyai/go-lua/types/kind"

// JoinPreferNonSoft joins two types while preferring non-soft placeholders.
// This centralizes the "soft placeholder" policy used across inference and flow.
func JoinPreferNonSoft(a, b Type) Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	a = PruneSoftUnionMembers(a)
	b = PruneSoftUnionMembers(b)
	if IsSoft(a, SoftPlaceholderPolicy) && !IsSoft(b, SoftPlaceholderPolicy) {
		return b
	}
	if IsSoft(b, SoftPlaceholderPolicy) && !IsSoft(a, SoftPlaceholderPolicy) {
		return a
	}
	// Inline join.Two to avoid dependency cycles inside typ.
	if a == nil || a.Kind() == kind.Unknown {
		return b
	}
	if b == nil || b.Kind() == kind.Unknown {
		return a
	}
	if TypeEquals(a, b) {
		return a
	}
	return PruneSoftUnionMembers(NewUnion(a, b))
}
