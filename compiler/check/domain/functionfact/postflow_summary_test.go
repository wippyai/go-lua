package functionfact

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPostflowReturnSummary_DropsImplicitAllNilSummary(t *testing.T) {
	if got := PostflowReturnSummary(&ast.FunctionExpr{}, []typ.Type{typ.Nil}); got != nil {
		t.Fatalf("PostflowReturnSummary() = %v, want nil for implicit all-nil summary", got)
	}
}

func TestPostflowReturnSummary_KeepsDeclaredAllNilSummary(t *testing.T) {
	fn := &ast.FunctionExpr{ReturnTypes: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "nil"}}}
	got := PostflowReturnSummary(fn, []typ.Type{typ.Nil})
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.Nil) {
		t.Fatalf("PostflowReturnSummary() = %v, want declared nil summary", got)
	}
}
