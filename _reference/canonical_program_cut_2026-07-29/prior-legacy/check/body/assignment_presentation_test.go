package body

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func TestTypeAnnotationLabelPreservesFunctionReturnAlias(t *testing.T) {
	expr := &ast.FunctionTypeExpr{
		Returns: []ast.TypeExpr{&ast.TypeRefExpr{Path: []string{"Res"}}},
	}
	if got, want := TypeAnnotationLabel(expr), "fun() -> Res"; got != want {
		t.Fatalf("TypeAnnotationLabel = %q, want %q", got, want)
	}
}

func TestTypeAnnotationLabelFormatsFunctionParamsAndVariadic(t *testing.T) {
	expr := &ast.FunctionTypeExpr{
		Params: []ast.FunctionParamExpr{
			{Name: "id", Type: &ast.PrimitiveTypeExpr{Name: "string"}},
			{Type: &ast.GenericTypeExpr{
				Base: &ast.TypeRefExpr{Path: []string{"Result"}},
				Args: []ast.TypeExpr{&ast.TypeRefExpr{Path: []string{"Res"}}},
			}},
		},
		Variadic: &ast.PrimitiveTypeExpr{Name: "number"},
		Returns:  []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "boolean"}},
	}
	if got, want := TypeAnnotationLabel(expr), "fun(id: string, Result<Res>, ...: number) -> boolean"; got != want {
		t.Fatalf("TypeAnnotationLabel = %q, want %q", got, want)
	}
}
