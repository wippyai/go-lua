package typ

import "github.com/wippyai/go-lua/analysis/type/kind"

// IsUnknown reports whether t is explicitly the unknown type.
func IsUnknown(t Type) bool {
	return t != nil && t.Kind() == kind.Unknown
}

// IsAny reports whether t is explicitly the any type.
func IsAny(t Type) bool {
	return t != nil && t.Kind() == kind.Any
}

// IsNever reports whether t is explicitly the never type.
func IsNever(t Type) bool {
	return t != nil && t.Kind() == kind.Never
}

// AbsentOrUnknown reports whether t is missing (nil) or unknown.
//
// This intentionally does not treat the explicit nil type as unknown.
func AbsentOrUnknown(t Type) bool {
	return t == nil || IsUnknown(t)
}

// UnknownOrNil reports whether t is missing (nil), unknown, or explicit nil type.
func UnknownOrNil(t Type) bool {
	return AbsentOrUnknown(t) || (t != nil && t.Kind() == kind.Nil)
}

// HasKnown reports whether the slice contains at least one concrete type.
// nil entries and unknown entries are treated as unresolved.
func HasKnown(types []Type) bool {
	for _, t := range types {
		if !AbsentOrUnknown(t) {
			return true
		}
	}
	return false
}

// UnknownOnlyOrEmpty reports whether the slice has no concrete types.
func UnknownOnlyOrEmpty(types []Type) bool {
	return !HasKnown(types)
}
