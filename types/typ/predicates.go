package typ

import "github.com/wippyai/go-lua/types/kind"

// IsUnknown reports whether t is explicitly the unknown type.
func IsUnknown(t Type) bool {
	return t != nil && t.Kind() == kind.Unknown
}

// IsAbsentOrUnknown reports whether t is missing (nil) or unknown.
//
// This intentionally does not treat the explicit nil type as unknown.
func IsAbsentOrUnknown(t Type) bool {
	return t == nil || IsUnknown(t)
}

// HasKnownType reports whether the slice contains at least one concrete type.
// nil entries and unknown entries are treated as unresolved.
func HasKnownType(types []Type) bool {
	for _, t := range types {
		if !IsAbsentOrUnknown(t) {
			return true
		}
	}
	return false
}

// IsUnknownOnlyOrEmpty reports whether the slice has no concrete types.
func IsUnknownOnlyOrEmpty(types []Type) bool {
	return !HasKnownType(types)
}
