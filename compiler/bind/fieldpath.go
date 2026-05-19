package bind

import (
	"strings"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/pathkey"
)

// FieldPathKeyFromSegments converts path segments into the canonical binder key format.
//
// Canonical format is constraint.FormatSegments output, e.g.:
//   - .field
//   - .a.b
//   - ["a.b"]
//   - [1]
func FieldPathKeyFromSegments(segs []constraint.Segment) (string, bool) {
	if len(segs) == 0 {
		return "", false
	}

	for _, seg := range segs {
		switch seg.Kind {
		case constraint.SegmentField:
			if seg.Name == "" || !pathkey.IsIdentName(seg.Name) {
				return "", false
			}
		case constraint.SegmentIndexString:
			if seg.Name == "" {
				return "", false
			}
		case constraint.SegmentIndexInt:
			// Any int index is valid.
		default:
			return "", false
		}
	}

	return constraint.FormatSegments(segs), true
}

// NormalizeFieldPathKey canonicalizes legacy dotted paths into binder key format.
//
// Legacy dotted form:
//   - f
//   - a.b
//
// Canonical form:
//   - .f
//   - .a.b
//   - ["a.b"]
//   - [1]
func NormalizeFieldPathKey(path string) (string, bool) {
	if path == "" {
		return "", false
	}

	// Segment-suffix form (.field, [1], ["key"], legacy [key]).
	// Parse and re-encode to canonical representation.
	if strings.HasPrefix(path, ".") || strings.HasPrefix(path, "[") {
		segs := pathkey.ParseSuffix(path)
		if len(segs) == 0 {
			return "", false
		}
		return constraint.FormatSegments(segs), true
	}

	parts := strings.Split(path, ".")
	segs := make([]constraint.Segment, 0, len(parts))

	for _, part := range parts {
		if part == "" {
			return "", false
		}

		segs = append(segs, constraint.Segment{Kind: constraint.SegmentField, Name: part})
	}

	return FieldPathKeyFromSegments(segs)
}

func displayFieldPathKey(path string) string {
	if path == "" {
		return ""
	}

	if strings.HasPrefix(path, ".") {
		return path[1:]
	}

	return path
}

// DirectFieldNameFromKey returns the field name for a one-segment string-like
// field key. Numeric indexes and nested paths do not describe direct prototype
// fields and are rejected.
func DirectFieldNameFromKey(path string) (string, bool) {
	segs := pathkey.ParseSuffix(path)
	if len(segs) != 1 {
		return "", false
	}
	switch segs[0].Kind {
	case constraint.SegmentField, constraint.SegmentIndexString:
		return segs[0].Name, segs[0].Name != ""
	default:
		return "", false
	}
}
