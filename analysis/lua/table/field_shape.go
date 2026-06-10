package table

import "github.com/wippyai/go-lua/analysis/type/typ"

// SplitNilableField converts a nil-capable table field value into an optional
// field shape.
func SplitNilableField(t typ.Type) (inner typ.Type, optional bool) {
	if t == nil {
		return typ.Unknown, true
	}
	nonNil, nilable := typ.WithoutNil(t, typ.NilProjectionStructural)
	if !nilable {
		return t, false
	}
	if nonNil == nil {
		return typ.Never, true
	}
	return nonNil, true
}
