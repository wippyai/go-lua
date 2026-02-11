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
