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
