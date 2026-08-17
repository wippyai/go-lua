package parserproducts

import (
	goast "go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestControlledActionAssertionRecognizesPositiveAndNegativeBranches(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "action.go", `package p; func f() { if value, ok := input.(T); !ok { return } }`, 0)
	if err != nil {
		t.Fatal(err)
	}
	branch := file.Decls[0].(*goast.FuncDecl).Body.List[0].(*goast.IfStmt)
	assertion, err := actionIfAssertion(branch)
	if err != nil {
		t.Fatal(err)
	}
	if assertion.valueName != "value" || assertion.successWhenTrue {
		t.Fatalf("assertion = %#v, want value with negative polarity", assertion)
	}
	if positive, err := assertionCondition(branch.Cond.(*goast.UnaryExpr), "ok"); err != nil || positive {
		t.Fatalf("assertion condition = %v/%v, want false", positive, err)
	}
}
