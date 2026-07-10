package pathexpr

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/compiler/ast"
)

// TableFieldSuffixKind classifies the static table field form that produced a
// one-segment path suffix.
type TableFieldSuffixKind uint8

const (
	TableFieldSuffixImplicitIndex TableFieldSuffixKind = iota + 1
	TableFieldSuffixField
	TableFieldSuffixStringIndex
	TableFieldSuffixIntIndex
)

// TableFieldSuffix is the static one-segment path suffix contributed by a table
// constructor field.
type TableFieldSuffix struct {
	Path    path.Path
	Segment segment.Segment
	Kind    TableFieldSuffixKind
}

// CanNameSummaryPath reports whether this suffix is usable when deriving a
// function summary identity from a table literal path.
func (s TableFieldSuffix) CanNameSummaryPath() bool {
	return s.Kind != TableFieldSuffixStringIndex || s.Segment.Name != ""
}

// ResolveTableFieldSuffix classifies a table constructor field with a static
// key and returns the one-segment suffix it contributes to a path. Implicit
// array fields advance arrayIndex using Lua's 1-based constructor positions.
// Dynamic keys are rejected. Callers remain responsible for value-list shape
// rules such as final vararg expansion.
func ResolveTableFieldSuffix(field *ast.Field, arrayIndex *int) (TableFieldSuffix, bool) {
	if field == nil {
		return TableFieldSuffix{}, false
	}
	if field.Key == nil {
		if arrayIndex == nil {
			return TableFieldSuffix{}, false
		}
		*arrayIndex = *arrayIndex + 1
		return tableFieldSuffix(
			TableFieldSuffixImplicitIndex,
			segment.Segment{Kind: segment.SegmentIndexInt, Index: *arrayIndex},
		), true
	}
	switch key := field.Key.(type) {
	case *ast.StringExpr:
		switch field.KeySyntax {
		case ast.AttrKeyDot:
			if key.Value == "" {
				return TableFieldSuffix{}, false
			}
			return tableFieldSuffix(
				TableFieldSuffixField,
				segment.Segment{Kind: segment.SegmentField, Name: key.Value},
			), true
		case ast.AttrKeyIndex:
			return tableFieldSuffix(
				TableFieldSuffixStringIndex,
				segment.Segment{Kind: segment.SegmentIndexString, Name: key.Value},
			), true
		default:
			return TableFieldSuffix{}, false
		}
	case *ast.NumberExpr:
		if field.KeySyntax != ast.AttrKeyIndex {
			return TableFieldSuffix{}, false
		}
		index, ok := parseNonNegativeDecimalInt(key.Value)
		if !ok {
			return TableFieldSuffix{}, false
		}
		return tableFieldSuffix(
			TableFieldSuffixIntIndex,
			segment.Segment{Kind: segment.SegmentIndexInt, Index: index},
		), true
	default:
		return TableFieldSuffix{}, false
	}
}

func tableFieldSuffix(kind TableFieldSuffixKind, seg segment.Segment) TableFieldSuffix {
	return TableFieldSuffix{
		Path:    path.Path{Segments: []segment.Segment{seg}},
		Segment: seg,
		Kind:    kind,
	}
}
