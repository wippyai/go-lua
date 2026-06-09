package functionfact

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNarrowReturnSummary_DropsImplicitAllNilSummary(t *testing.T) {
	if got := NarrowReturnSummary(&ast.FunctionExpr{}, []typ.Type{typ.Nil}); got != nil {
		t.Fatalf("NarrowReturnSummary() = %v, want nil for implicit all-nil summary", got)
	}
}

func TestNarrowReturnSummary_KeepsDeclaredAllNilSummary(t *testing.T) {
	fn := &ast.FunctionExpr{ReturnTypes: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "nil"}}}
	got := NarrowReturnSummary(fn, []typ.Type{typ.Nil})
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Nil) {
		t.Fatalf("NarrowReturnSummary() = %v, want declared nil summary", got)
	}
}
