package mutator

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestMutationValueFieldSegmentKeepsStaticKeyShape(t *testing.T) {
	idx := 1
	cases := []struct {
		name  string
		field *ast.Field
		want  constraint.Segment
	}{
		{
			name:  "identifier field",
			field: &ast.Field{Key: &ast.IdentExpr{Value: "handler"}},
			want:  constraint.Segment{Kind: constraint.SegmentField, Name: "handler"},
		},
		{
			name:  "identifier-shaped string stays field until AST carries key syntax",
			field: &ast.Field{Key: &ast.StringExpr{Value: "handler"}},
			want:  constraint.Segment{Kind: constraint.SegmentField, Name: "handler"},
		},
		{
			name:  "non-identifier string index",
			field: &ast.Field{Key: &ast.StringExpr{Value: "x-y"}},
			want:  constraint.Segment{Kind: constraint.SegmentIndexString, Name: "x-y"},
		},
		{
			name:  "empty string index",
			field: &ast.Field{Key: &ast.StringExpr{Value: ""}},
			want:  constraint.Segment{Kind: constraint.SegmentIndexString, Name: ""},
		},
		{
			name:  "numeric literal index",
			field: &ast.Field{Key: &ast.NumberExpr{Value: "2"}},
			want:  constraint.Segment{Kind: constraint.SegmentIndexInt, Index: 2},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := mutationValueFieldSegment(tc.field, &idx)
			if !ok {
				t.Fatal("mutationValueFieldSegment rejected static key")
			}
			if got != tc.want {
				t.Fatalf("mutationValueFieldSegment = %#v, want %#v", got, tc.want)
			}
		})
	}
}
