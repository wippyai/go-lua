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

// JoinReturnSlot merges return slot types while preserving uncertainty.
//
// Unknown in return inference means unresolved runtime behavior. When one branch
// is unknown and another is explicit nil, keep unknown so summaries do not
// collapse to nil-only.
func JoinReturnSlot(a, b Type) Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	a = PruneSoftUnionMembers(a)
	b = PruneSoftUnionMembers(b)
	if (a.Kind() == kind.Unknown && b.Kind() == kind.Nil) || (b.Kind() == kind.Unknown && a.Kind() == kind.Nil) {
		return Unknown
	}
	return JoinPreferNonSoft(a, b)
}

// IsRefinableAnnotation reports whether an explicit annotation should be
// treated as a soft placeholder that call-site/contextual hints may refine.
//
// Canonical rule: explicit top types (`any`, `unknown`) are authoritative and
// must not be rewritten by hints. Structural soft placeholders like `{any}` or
// `any[]` remain refinable.
func IsRefinableAnnotation(t Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind() {
	case kind.Any, kind.Unknown:
		return false
	default:
		return IsSoft(t, SoftAnnotationPolicy)
	}
}
