package presence

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// AbsentOrUnknown reports whether t is missing (nil) or unknown.
//
// This intentionally does not treat the explicit nil type as unknown.
func AbsentOrUnknown(t typ.Type) bool {
	return t == nil || typ.IsUnknown(t)
}

// UnknownOrNil reports whether t is missing (nil), unknown, or explicit nil type.
func UnknownOrNil(t typ.Type) bool {
	return AbsentOrUnknown(t) || (t != nil && t.Kind() == kind.Nil)
}

// HasKnown reports whether the slice contains at least one concrete type.
// nil entries and unknown entries are treated as unresolved.
func HasKnown(types []typ.Type) bool {
	for _, t := range types {
		if !AbsentOrUnknown(t) {
			return true
		}
	}
	return false
}

// UnknownOnlyOrEmpty reports whether the slice has no concrete types.
func UnknownOnlyOrEmpty(types []typ.Type) bool {
	return !HasKnown(types)
}
