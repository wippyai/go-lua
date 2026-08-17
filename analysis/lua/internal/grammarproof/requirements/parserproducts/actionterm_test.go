package parserproducts

import (
	"go/parser"
	"testing"
)

func TestActionTermInterningRetainsScopeAndDeduplicatesEquivalentTerms(t *testing.T) {
	builder := newActionTermBuilder(nil)
	scope := builder.scope(ActionScopeProduction, "expr#1", 1, 1, nil)
	expression, err := parser.ParseExpr("Arg1")
	if err != nil {
		t.Fatal(err)
	}
	first, err := builder.expression(&scope, expression)
	if err != nil {
		t.Fatal(err)
	}
	second, err := builder.expression(&scope, expression)
	if err != nil {
		t.Fatal(err)
	}
	if first == 0 || first != second {
		t.Fatalf("equivalent action terms = %d/%d, want one interned identity", first, second)
	}
	term, ok := builder.terms.Term(first)
	if !ok || term.Scope != scope.id || term.Kind != ActionTermInput || term.Slot != 0 {
		t.Fatalf("interned term = %#v/%v, want scoped input", term, ok)
	}
}
