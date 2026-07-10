package pathexpr

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestResolveTableFieldSuffixStaticKeys(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field *ast.Field
		want  TableFieldSuffix
	}{
		{
			name:  "dot field",
			field: &ast.Field{Key: &ast.StringExpr{Value: "name"}, KeySyntax: ast.AttrKeyDot},
			want:  tableFieldSuffix(TableFieldSuffixField, segment.Segment{Kind: segment.SegmentField, Name: "name"}),
		},
		{
			name:  "string index",
			field: &ast.Field{Key: &ast.StringExpr{Value: "key"}, KeySyntax: ast.AttrKeyIndex},
			want:  tableFieldSuffix(TableFieldSuffixStringIndex, segment.Segment{Kind: segment.SegmentIndexString, Name: "key"}),
		},
		{
			name:  "empty string index",
			field: &ast.Field{Key: &ast.StringExpr{}, KeySyntax: ast.AttrKeyIndex},
			want:  tableFieldSuffix(TableFieldSuffixStringIndex, segment.Segment{Kind: segment.SegmentIndexString}),
		},
		{
			name:  "integer index",
			field: &ast.Field{Key: &ast.NumberExpr{Value: "12"}, KeySyntax: ast.AttrKeyIndex},
			want:  tableFieldSuffix(TableFieldSuffixIntIndex, segment.Segment{Kind: segment.SegmentIndexInt, Index: 12}),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			arrayIndex := 0
			got, ok := ResolveTableFieldSuffix(tc.field, &arrayIndex)
			if !ok {
				t.Fatalf("ResolveTableFieldSuffix rejected %s", tc.name)
			}
			if got.Kind != tc.want.Kind || got.Segment != tc.want.Segment || !reflect.DeepEqual(got.Path.Segments, tc.want.Path.Segments) {
				t.Fatalf("ResolveTableFieldSuffix = %#v, want %#v", got, tc.want)
			}
			if arrayIndex != 0 {
				t.Fatalf("arrayIndex = %d, want unchanged", arrayIndex)
			}
		})
	}
}

func TestResolveTableFieldSuffixImplicitArrayIndex(t *testing.T) {
	arrayIndex := 0

	first, ok := ResolveTableFieldSuffix(&ast.Field{}, &arrayIndex)
	if !ok {
		t.Fatalf("first implicit array field rejected")
	}
	assertTableFieldSuffix(t, first, TableFieldSuffixImplicitIndex, path.Path{Segments: []segment.Segment{{Kind: segment.SegmentIndexInt, Index: 1}}})

	second, ok := ResolveTableFieldSuffix(&ast.Field{}, &arrayIndex)
	if !ok {
		t.Fatalf("second implicit array field rejected")
	}
	assertTableFieldSuffix(t, second, TableFieldSuffixImplicitIndex, path.Path{Segments: []segment.Segment{{Kind: segment.SegmentIndexInt, Index: 2}}})

	if arrayIndex != 2 {
		t.Fatalf("arrayIndex = %d, want 2", arrayIndex)
	}
}

func TestResolveTableFieldSuffixRejectsDynamicOrInvalidKeys(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	overflow := "1"
	for i := maxInt; i > 0; i /= 10 {
		overflow += "0"
	}

	for _, tc := range []struct {
		name  string
		field *ast.Field
	}{
		{name: "nil field", field: nil},
		{name: "implicit without index", field: &ast.Field{}},
		{name: "empty dot", field: &ast.Field{Key: &ast.StringExpr{}, KeySyntax: ast.AttrKeyDot}},
		{name: "default string syntax", field: &ast.Field{Key: &ast.StringExpr{Value: "name"}}},
		{name: "number dot", field: &ast.Field{Key: &ast.NumberExpr{Value: "1"}, KeySyntax: ast.AttrKeyDot}},
		{name: "decimal fraction", field: &ast.Field{Key: &ast.NumberExpr{Value: "1.5"}, KeySyntax: ast.AttrKeyIndex}},
		{name: "overflow integer", field: &ast.Field{Key: &ast.NumberExpr{Value: overflow}, KeySyntax: ast.AttrKeyIndex}},
		{name: "dynamic index", field: &ast.Field{Key: ident("key"), KeySyntax: ast.AttrKeyIndex}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			arrayIndex := 0
			got, ok := ResolveTableFieldSuffix(tc.field, nil)
			if tc.field != nil && tc.field.Key != nil {
				got, ok = ResolveTableFieldSuffix(tc.field, &arrayIndex)
			}
			if ok || len(got.Path.Segments) != 0 || got.Kind != 0 {
				t.Fatalf("ResolveTableFieldSuffix = %#v/%v, want empty/false", got, ok)
			}
			if arrayIndex != 0 {
				t.Fatalf("arrayIndex = %d, want unchanged", arrayIndex)
			}
		})
	}
}

func assertTableFieldSuffix(t *testing.T, got TableFieldSuffix, wantKind TableFieldSuffixKind, wantPath path.Path) {
	t.Helper()
	if got.Kind != wantKind || !reflect.DeepEqual(got.Path.Segments, wantPath.Segments) {
		t.Fatalf("suffix = %#v, want kind %d path %#v", got, wantKind, wantPath)
	}
	if len(wantPath.Segments) != 1 || got.Segment != wantPath.Segments[0] {
		t.Fatalf("suffix segment = %#v, want %#v", got.Segment, wantPath.Segments)
	}
}
