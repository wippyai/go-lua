package table

import "github.com/wippyai/go-lua/analysis/type/typ"

// SplitNilableFieldType converts a nil-capable table field value into an
// optional field shape.
func SplitNilableFieldType(t typ.Type) (inner typ.Type, optional bool) {
	if t == nil {
		return typ.Unknown, true
	}
	nonNil, nilable := withoutNil(t, nilProjectionStructural)
	if !nilable {
		return t, false
	}
	if nonNil == nil {
		return typ.Never, true
	}
	return nonNil, true
}

// PresentReadonlyEntryValue returns the value type for a readonly table entry
// after the entry is known to be present.
func PresentReadonlyEntryValue(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	if inner, optional := SplitNilableFieldType(t); optional {
		return inner
	}
	return t
}
