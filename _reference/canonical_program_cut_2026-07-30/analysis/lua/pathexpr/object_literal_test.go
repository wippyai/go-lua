package pathexpr

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/compiler/ast"
)

func tableString(value string, syntax ast.AttrKeySyntax) path.Path {
	switch syntax {
	case ast.AttrKeyDot:
		return path.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: value}}}
	case ast.AttrKeyIndex:
		return path.Path{Segments: []segment.Segment{{Kind: segment.SegmentIndexString, Name: value}}}
	default:
		return path.Path{}
	}
}

func tableInt(index int) path.Path {
	return path.Path{Segments: []segment.Segment{{Kind: segment.SegmentIndexInt, Index: index}}}
}

func TestObjectEntriesStaticAndNestedFlatten(t *testing.T) {
	nestedLeaf := &ast.NumberExpr{Value: "1"}
	dynamicValue := &ast.NumberExpr{Value: "2"}
	deepLeaf := &ast.NumberExpr{Value: "3"}
	deeper := &ast.TableExpr{Fields: []*ast.Field{
		{Key: &ast.StringExpr{Value: "d"}, KeySyntax: ast.AttrKeyDot, Value: deepLeaf},
	}}
	nested := &ast.TableExpr{Fields: []*ast.Field{
		{Key: &ast.StringExpr{Value: "b"}, KeySyntax: ast.AttrKeyDot, Value: nestedLeaf},
		{Key: &ast.IdentExpr{Value: "dynamic"}, KeySyntax: ast.AttrKeyIndex, Value: dynamicValue},
		{Key: &ast.StringExpr{Value: "c"}, KeySyntax: ast.AttrKeyDot, Value: deeper},
	}}
	table := &ast.TableExpr{Fields: []*ast.Field{
		{Key: &ast.StringExpr{Value: "a"}, KeySyntax: ast.AttrKeyDot, Value: nested},
	}}

	entries := ObjectEntries(table)
	if len(entries) != 4 {
		t.Fatalf("entries = %#v, want root and nested static entries", entries)
	}
	if entries[0].Index != 0 || entries[0].Value != nested || !entries[0].Suffix.Equal(tableString("a", ast.AttrKeyDot)) || !entries[0].Final {
		t.Fatalf("root entry = %#v", entries[0])
	}
	if entries[1].Index != 0 || entries[1].Value != nestedLeaf || !entries[1].Suffix.Equal(path.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "a"}, {Kind: segment.SegmentField, Name: "b"}}}) || entries[1].Final {
		t.Fatalf("nested entry = %#v", entries[1])
	}
	if entries[2].Index != 2 || entries[2].Value != deeper || !entries[2].Suffix.Equal(path.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "a"}, {Kind: segment.SegmentField, Name: "c"}}}) || !entries[2].Final {
		t.Fatalf("nested object entry = %#v", entries[2])
	}
	if entries[3].Index != 0 || entries[3].Value != deepLeaf || !entries[3].Suffix.Equal(path.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "a"}, {Kind: segment.SegmentField, Name: "c"}, {Kind: segment.SegmentField, Name: "d"}}}) || !entries[3].Final {
		t.Fatalf("deep nested entry = %#v", entries[3])
	}
	for _, entry := range entries {
		if entry.Value == dynamicValue {
			t.Fatalf("dynamic nested field was included: %#v", entry)
		}
	}
}

func TestObjectEntriesSkipsFinalExpandingArrayField(t *testing.T) {
	nonFinalVararg := &ast.Comma3Expr{}
	keyedVararg := &ast.Comma3Expr{}
	finalVararg := &ast.Comma3Expr{}
	table := &ast.TableExpr{Fields: []*ast.Field{
		{Value: nonFinalVararg},
		{Key: &ast.StringExpr{Value: "key"}, KeySyntax: ast.AttrKeyDot, Value: keyedVararg},
		{Value: finalVararg},
	}}

	entries := ObjectEntries(table)
	if len(entries) != 2 {
		t.Fatalf("entries = %#v, want non-final array and keyed vararg only", entries)
	}
	if entries[0].Value != nonFinalVararg || !entries[0].Suffix.Equal(tableInt(1)) || entries[0].Final {
		t.Fatalf("non-final vararg entry = %#v", entries[0])
	}
	if entries[1].Value != keyedVararg || !entries[1].Suffix.Equal(tableString("key", ast.AttrKeyDot)) || entries[1].Final {
		t.Fatalf("keyed vararg entry = %#v", entries[1])
	}
	for _, entry := range entries {
		if entry.Value == finalVararg {
			t.Fatalf("final expanding array field was included: %#v", entry)
		}
	}
}

func TestObjectLiteralTableUnwrapsWrappers(t *testing.T) {
	table := &ast.TableExpr{}
	wrapped := []ast.Expr{
		&ast.CastExpr{Expr: table, Syntax: ast.CastSyntaxAs},
		&ast.NonNilAssertExpr{Expr: table},
	}
	for _, expr := range wrapped {
		got, ok := ObjectLiteralTable(expr)
		if !ok || got != table {
			t.Fatalf("ObjectLiteralTable(%T) = %v/%v, want table/true", expr, got, ok)
		}
	}
	if got, ok := ObjectLiteralTable(&ast.IdentExpr{Value: "x"}); ok || got != nil {
		t.Fatalf("ObjectLiteralTable rejected non-table = %v/%v, want nil/false", got, ok)
	}
}

func TestObjectLiteralTableDoesNotUnwrapAnyCast(t *testing.T) {
	table := &ast.TableExpr{}
	expr := &ast.CastExpr{
		Expr: table,
		Type: &ast.PrimitiveTypeExpr{Name: "any"},
	}

	if got, ok := ObjectLiteralTable(expr); ok || got != nil {
		t.Fatalf("ObjectLiteralTable(any cast) = %v/%v, want nil/false", got, ok)
	}
}
