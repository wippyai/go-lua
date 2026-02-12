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
	if IsAbsentOrUnknown(a) {
		return b
	}
	if IsAbsentOrUnknown(b) {
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
	if preferred, ok := preferArrayOverEmptyRecord(a, b); ok {
		return preferred
	}
	if (IsAny(a) && b.Kind() == kind.Nil) || (IsAny(b) && a.Kind() == kind.Nil) {
		return Any
	}
	if (IsUnknown(a) && b.Kind() == kind.Nil) || (IsUnknown(b) && a.Kind() == kind.Nil) {
		return Unknown
	}
	return JoinPreferNonSoft(a, b)
}

func preferArrayOverEmptyRecord(a, b Type) (Type, bool) {
	if isEmptyRecordNoMap(a) && isArrayLike(b) {
		return b, true
	}
	if isEmptyRecordNoMap(b) && isArrayLike(a) {
		return a, true
	}
	return nil, false
}

func isEmptyRecordNoMap(t Type) bool {
	switch v := t.(type) {
	case *Alias:
		return isEmptyRecordNoMap(v.Target)
	case *Record:
		return len(v.Fields) == 0 && !v.HasMapComponent()
	default:
		return false
	}
}

func isArrayLike(t Type) bool {
	switch v := t.(type) {
	case *Alias:
		return isArrayLike(v.Target)
	case *Array:
		return true
	default:
		return false
	}
}

// JoinBranchOutcome merges mutually-exclusive expression outcomes (for example,
// `a and b` / `a or b`) while preserving uncertainty.
//
// Unlike JoinPreferNonSoft, this must not treat unknown as absent information:
// expression typing needs to preserve runtime uncertainty when one branch may
// still produce unknown-like values.
func JoinBranchOutcome(a, b Type) Type {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}

	a = PruneSoftUnionMembers(a)
	b = PruneSoftUnionMembers(b)

	// Preserve runtime uncertainty for branch outcomes:
	// unknown and nil means "value may be unknown or absent".
	if (IsUnknown(a) && b.Kind() == kind.Nil) || (IsUnknown(b) && a.Kind() == kind.Nil) {
		return NewOptional(Unknown)
	}

	if IsSoft(a, SoftPlaceholderPolicy) && !IsSoft(b, SoftPlaceholderPolicy) && b.Kind() != kind.Nil {
		return b
	}
	if IsSoft(b, SoftPlaceholderPolicy) && !IsSoft(a, SoftPlaceholderPolicy) && a.Kind() != kind.Nil {
		return a
	}

	if TypeEquals(a, b) {
		return a
	}

	return PruneSoftUnionMembers(NewUnion(a, b))
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
	if t.Kind().IsPlaceholder() {
		return false
	}
	return IsSoft(t, SoftAnnotationPolicy)
}
