package static

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func TestFunctionTypeShapeKeepsVariadicTailDistinct(t *testing.T) {
	primitive := &ast.PrimitiveTypeExpr{}
	shape := &ast.FunctionTypeExpr{
		Params:           []ast.FunctionParamExpr{{Type: primitive}},
		Variadic:         primitive,
		VariadicPosition: ast.Position{Line: 1, Column: 5, EndLine: 1, EndColumn: 7},
	}
	fixed, variadic, err := FunctionTypeShape(shape)
	if err != nil || fixed != 1 || variadic != primitive {
		t.Fatalf("FunctionTypeShape = %d/%T/%v, want 1/primitive/nil error", fixed, variadic, err)
	}
	if _, _, err := FunctionTypeShape(&ast.FunctionTypeExpr{Params: []ast.FunctionParamExpr{{}}}); err == nil {
		t.Fatal("FunctionTypeShape accepted an untyped fixed parameter")
	}
}
