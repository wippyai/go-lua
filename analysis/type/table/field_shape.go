package table

import "github.com/wippyai/go-lua/analysis/type/typ"

// splitNilableFieldType converts a nil-capable table field value into an
// optional field shape.
func splitNilableFieldType(t typ.Type) (inner typ.Type, optional bool) {
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

// EntryValueShape returns the present payload and absent-capable bit for a
// table entry value.
func EntryValueShape(t typ.Type) (typ.Type, bool) {
	if t == nil {
		return nil, false
	}
	if inner, optional := splitNilableFieldType(t); optional {
		return inner, true
	}
	return t, false
}

// PresentReadonlyEntryValue returns the value type for a readonly table entry
// after the entry is known to be present.
func PresentReadonlyEntryValue(t typ.Type) typ.Type {
	payload, _ := EntryValueShape(t)
	return payload
}
