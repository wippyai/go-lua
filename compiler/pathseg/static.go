package pathseg

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/pathkey"
)

func segmentFromStringOrNumber(key ast.Expr) (constraint.Segment, bool) {
	switch k := key.(type) {
	case *ast.StringExpr:
		if pathkey.IsIdentName(k.Value) {
			return constraint.Segment{Kind: constraint.SegmentField, Name: k.Value}, true
		}
		return constraint.Segment{Kind: constraint.SegmentIndexString, Name: k.Value}, true
	case *ast.NumberExpr:
		if idx, ok := pathkey.ParseIntLiteral(k.Value); ok {
			return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: idx}, true
		}
		return constraint.Segment{}, false
	default:
		return constraint.Segment{}, false
	}
}

func dotSegment(key ast.Expr) (constraint.Segment, bool) {
	switch k := key.(type) {
	case *ast.StringExpr:
		if k.Value == "" {
			return constraint.Segment{}, false
		}
		return constraint.Segment{Kind: constraint.SegmentField, Name: k.Value}, true
	case *ast.IdentExpr:
		if k.Value == "" {
			return constraint.Segment{}, false
		}
		return constraint.Segment{Kind: constraint.SegmentField, Name: k.Value}, true
	default:
		return constraint.Segment{}, false
	}
}

func indexSegment(key ast.Expr) (constraint.Segment, bool) {
	switch k := key.(type) {
	case *ast.StringExpr:
		return constraint.Segment{Kind: constraint.SegmentIndexString, Name: k.Value}, true
	case *ast.NumberExpr:
		if idx, ok := pathkey.ParseIntLiteral(k.Value); ok {
			return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: idx}, true
		}
		return constraint.Segment{}, false
	default:
		return constraint.Segment{}, false
	}
}

func indexSegmentFromConstValue(val *flow.ConstValue) (constraint.Segment, bool) {
	if val == nil {
		return constraint.Segment{}, false
	}
	switch val.Kind {
	case flow.ConstString:
		return constraint.Segment{Kind: constraint.SegmentIndexString, Name: val.Str}, true
	case flow.ConstInt:
		return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: int(val.Int)}, true
	case flow.ConstFloat:
		if idx, ok := pathkey.FloatToSafeInt(val.Float); ok {
			return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: idx}, true
		}
	}
	return constraint.Segment{}, false
}

// StaticAttrKeySegment converts a static AttrGet key expression into a path
// segment using the legacy heuristic for hand-built AST nodes.
//
// Prefer StaticAttrSegment when the full AttrGetExpr is available; it preserves
// parser-produced dot-vs-bracket syntax. This helper intentionally keeps
// identifier-looking string literals as fields for old manual constructors.
//
// Legacy semantics:
//   - string "k": SegmentField("k")
//   - string "x-y": SegmentIndexString("x-y")
//   - number 1: SegmentIndexInt(1)
//   - identifier key: rejected as dynamic
func StaticAttrKeySegment(key ast.Expr) (constraint.Segment, bool) {
	return segmentFromStringOrNumber(key)
}

// StaticAttrKeySegmentWithSyntax converts an AttrGet key using source syntax.
// Unknown syntax preserves the legacy hand-built-AST heuristic; parser-produced
// dot and bracket syntax keep `.foo` and `["foo"]` structurally distinct.
func StaticAttrKeySegmentWithSyntax(key ast.Expr, syntax ast.AttrKeySyntax) (constraint.Segment, bool) {
	switch syntax {
	case ast.AttrKeyDot:
		return dotSegment(key)
	case ast.AttrKeyIndex:
		return indexSegment(key)
	default:
		return StaticAttrKeySegment(key)
	}
}

// StaticAttrSegment converts a full AttrGet expression into a static path
// segment, using its source syntax when present.
func StaticAttrSegment(attr *ast.AttrGetExpr) (constraint.Segment, bool) {
	if attr == nil {
		return constraint.Segment{}, false
	}
	return StaticAttrKeySegmentWithSyntax(attr.Key, attr.KeySyntax)
}

// StaticTableFieldKeySegment converts a static table field key expression into
// a path segment using the legacy hand-built-AST heuristic.
//
// Legacy table field semantics:
//   - identifier field: {name = v}
//   - unknown string "name": SegmentField("name")
//   - numeric field: {[1] = v}
func StaticTableFieldKeySegment(key ast.Expr) (constraint.Segment, bool) {
	if ident, ok := key.(*ast.IdentExpr); ok {
		if ident.Value == "" {
			return constraint.Segment{}, false
		}
		return constraint.Segment{Kind: constraint.SegmentField, Name: ident.Value}, true
	}
	return segmentFromStringOrNumber(key)
}

// StaticTableFieldKeySegmentWithSyntax converts a table field key using the
// parser's table-constructor key syntax. Unknown syntax keeps the legacy
// hand-built-AST heuristic for existing tests and manual constructors.
func StaticTableFieldKeySegmentWithSyntax(key ast.Expr, syntax ast.AttrKeySyntax) (constraint.Segment, bool) {
	switch syntax {
	case ast.AttrKeyDot:
		return dotSegment(key)
	case ast.AttrKeyIndex:
		return indexSegment(key)
	default:
		return StaticTableFieldKeySegment(key)
	}
}

// StaticTableFieldSegment converts a full table field into a path segment,
// preserving parsed `{foo = ...}` vs `{["foo"] = ...}` syntax when available.
func StaticTableFieldSegment(field *ast.Field) (constraint.Segment, bool) {
	if field == nil || field.Key == nil {
		return constraint.Segment{}, false
	}
	return StaticTableFieldKeySegmentWithSyntax(field.Key, field.KeySyntax)
}

// StaticTableFieldSegmentWithConst converts a full table field into a path
// segment and, for bracket-syntax dynamic identifiers, resolves compile-time
// constant keys as bracket index segments. Unknown syntax preserves legacy
// hand-built-AST behavior, so `{Key: IdentExpr("k")}` remains a named field.
func StaticTableFieldSegmentWithConst(field *ast.Field, constResolver func(string) *flow.ConstValue) (constraint.Segment, bool) {
	if field == nil || field.Key == nil {
		return constraint.Segment{}, false
	}
	if seg, ok := StaticTableFieldSegment(field); ok {
		return seg, true
	}
	if field.KeySyntax != ast.AttrKeyIndex || constResolver == nil {
		return constraint.Segment{}, false
	}
	ident, ok := field.Key.(*ast.IdentExpr)
	if !ok {
		return constraint.Segment{}, false
	}
	return indexSegmentFromConstValue(constResolver(ident.Value))
}

// TableFieldMatchesSegment reports whether a source table-constructor field
// lowers exactly to segment under parser-preserved table-key syntax.
func TableFieldMatchesSegment(field *ast.Field, segment constraint.Segment) bool {
	seg, ok := StaticTableFieldSegment(field)
	return ok && seg == segment
}

// TableFieldMatchesSegmentWithConst is TableFieldMatchesSegment plus
// compile-time constant resolution for bracket-syntax dynamic identifiers.
func TableFieldMatchesSegmentWithConst(field *ast.Field, segment constraint.Segment, constResolver func(string) *flow.ConstValue) bool {
	seg, ok := StaticTableFieldSegmentWithConst(field, constResolver)
	return ok && seg == segment
}
