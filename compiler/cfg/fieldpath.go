package cfg

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/pathkey"
)

func fieldSegmentsFromNames(fields []string) ([]constraint.Segment, bool) {
	if len(fields) == 0 {
		return nil, false
	}

	segments := make([]constraint.Segment, 0, len(fields))

	for _, field := range fields {
		if field == "" || !pathkey.IsIdentName(field) {
			return nil, false
		}

		segments = append(segments, constraint.Segment{Kind: constraint.SegmentField, Name: field})
	}

	return segments, true
}

func staticSegmentFromKeyExpr(key ast.Expr) (constraint.Segment, bool) {
	switch keyExpr := key.(type) {
	case *ast.StringExpr:
		if keyExpr.Value == "" {
			return constraint.Segment{}, false
		}

		if pathkey.IsIdentName(keyExpr.Value) {
			return constraint.Segment{Kind: constraint.SegmentField, Name: keyExpr.Value}, true
		}

		return constraint.Segment{Kind: constraint.SegmentIndexString, Name: keyExpr.Value}, true

	case *ast.NumberExpr:
		index, ok := pathkey.ParseIntLiteral(keyExpr.Value)
		if !ok {
			return constraint.Segment{}, false
		}

		return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: index}, true
	}

	return constraint.Segment{}, false
}
