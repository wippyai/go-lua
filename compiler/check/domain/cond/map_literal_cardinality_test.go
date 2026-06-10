package cond

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func TestMapLiteralKeyCardinality(t *testing.T) {
	staticField := func(key string, value ast.Expr) *ast.Field {
		return &ast.Field{Key: &ast.StringExpr{Value: key}, Value: value}
	}
	dynamicField := func(value ast.Expr) *ast.Field {
		return &ast.Field{
			Key:       &ast.IdentExpr{Value: "k"},
			KeySyntax: ast.AttrKeyIndex,
			Value:     value,
		}
	}
	num := func(value string) ast.Expr {
		return &ast.NumberExpr{Value: value}
	}

	tests := []struct {
		name string
		tbl  *ast.TableExpr
		want int64
	}{
		{
			name: "nil table",
			want: 0,
		},
		{
			name: "nil field",
			tbl:  &ast.TableExpr{Fields: []*ast.Field{nil}},
			want: 0,
		},
		{
			name: "nil value",
			tbl:  &ast.TableExpr{Fields: []*ast.Field{staticField("a", nil)}},
			want: 0,
		},
		{
			name: "duplicate static keys collapse",
			tbl: &ast.TableExpr{Fields: []*ast.Field{
				staticField("a", num("1")),
				staticField("a", num("2")),
				staticField("b", num("3")),
			}},
			want: 2,
		},
		{
			name: "dynamic keys do not prove new entries",
			tbl: &ast.TableExpr{Fields: []*ast.Field{
				staticField("a", num("1")),
				dynamicField(num("2")),
				staticField("b", num("3")),
			}},
			want: 2,
		},
		{
			name: "positional field is not a pure map",
			tbl: &ast.TableExpr{Fields: []*ast.Field{
				staticField("a", num("1")),
				{Value: num("2")},
			}},
			want: 0,
		},
		{
			name: "nil literal values write no entry",
			tbl: &ast.TableExpr{Fields: []*ast.Field{
				staticField("a", num("1")),
				staticField("b", &ast.NilExpr{}),
			}},
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapLiteralKeyCardinality(tt.tbl); got != tt.want {
				t.Fatalf("mapLiteralKeyCardinality() = %d, want %d", got, tt.want)
			}
		})
	}
}
