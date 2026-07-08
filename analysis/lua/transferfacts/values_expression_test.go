package transferfacts

import (
	"testing"

	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestWIRModeExpressionFactsDoNotUseASTFallback(t *testing.T) {
	l := lowerer{
		wir:                     wir.NewBody("wir-expression-no-ast-fallback"),
		expressionOperations:    make(map[factflow.ExprRef]factflow.ExpressionOperation),
		dynamicIndexExpressions: make(map[factflow.ExprRef]factflow.DynamicIndexExpression),
	}

	concat := &ast.StringConcatOpExpr{
		Lhs: &ast.IdentExpr{Value: "left"},
		Rhs: &ast.StringExpr{Value: "right"},
	}
	l.addExpressionOperation(1, concat)
	if _, ok := l.expressionOperations[1]; ok {
		t.Fatal("WIR mode expression operation fell back to AST operand sources")
	}

	index := &ast.AttrGetExpr{
		Object:    &ast.IdentExpr{Value: "items"},
		Key:       &ast.IdentExpr{Value: "key"},
		KeySyntax: ast.AttrKeyIndex,
	}
	l.addDynamicIndexExpression(2, index)
	if _, ok := l.dynamicIndexExpressions[2]; ok {
		t.Fatal("WIR mode dynamic-index expression fell back to AST operand sources")
	}
}
