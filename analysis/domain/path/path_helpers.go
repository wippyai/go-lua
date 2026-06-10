package path

import (
	"strconv"
	"strings"
)

// PlaceholderIndexFromString extracts a placeholder index from the canonical $N form.
// Returns -1 for invalid syntax, negative values, and overflow.
func PlaceholderIndexFromString(s string) int {
	if len(s) >= 2 && s[0] == '$' {
		return parseNonNegativeDecimal(s[1:])
	}
	return -1
}

func parseNonNegativeDecimal(s string) int {
	if s == "" {
		return -1
	}

	maxInt := int(^uint(0) >> 1)
	value := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch < '0' || ch > '9' {
			return -1
		}
		digit := int(ch - '0')
		if value > (maxInt-digit)/10 {
			return -1
		}
		value = value*10 + digit
	}
	return value
}

// PlaceholderArgIndex resolves a placeholder path to an argument index.
//
// Returns false when the path is not a placeholder or index is out of bounds.
func PlaceholderArgIndex(path Path, argCount int) (int, bool) {
	if !path.IsPlaceholder() {
		return 0, false
	}
	idx := path.PlaceholderIndex()
	if idx < 0 || idx >= argCount {
		return 0, false
	}
	return idx, true
}

// NewReturnSlot returns the path for a return value slot.
func NewReturnSlot(index int) Path {
	if index < 0 {
		return Path{}
	}
	return Path{Root: "ret[" + strconv.Itoa(index) + "]"}
}

// ReturnIndexFromString extracts a return slot index from "ret[N]".
//
// Returns -1 for invalid syntax, negative values, and overflow.
func ReturnIndexFromString(s string) int {
	if strings.HasPrefix(s, "ret[") && strings.HasSuffix(s, "]") {
		return parseNonNegativeDecimal(s[4 : len(s)-1])
	}
	return -1
}

// IsReturnPath checks if the path represents a return value (ret[N] format).
func IsReturnPath(p Path) bool {
	return p.Symbol == 0 && len(p.Segments) == 0 && ReturnIndexFromString(p.Root) >= 0
}

// SegmentFieldName returns the record-field name addressed by a field-like
// segment. Dot fields and string indexes both project through record fields for
// structural typing. Empty names are rejected; callers that need runtime Lua
// string-key semantics, where an empty bracket string is valid, should keep that
// policy separate.
func SegmentFieldName(seg Segment) (string, bool) {
	switch seg.Kind {
	case SegmentField, SegmentIndexString:
		return seg.Name, seg.Name != ""
	default:
		return "", false
	}
}

// SplitFieldPath splits a path into its parent path and field name.
// Returns false if the path has no field segment. The returned parent path owns
// its own segment slice.
func SplitFieldPath(path Path) (parent Path, field string, ok bool) {
	if path.IsEmpty() || len(path.Segments) == 0 {
		return Path{}, "", false
	}
	last := path.Segments[len(path.Segments)-1]
	if last.Kind != SegmentField {
		return Path{}, "", false
	}
	parent = Path{Root: path.Root, Symbol: path.Symbol, Version: path.Version}
	if len(path.Segments) > 1 {
		parent.Segments = append(parent.Segments, path.Segments[:len(path.Segments)-1]...)
	}
	return parent, last.Name, true
}
