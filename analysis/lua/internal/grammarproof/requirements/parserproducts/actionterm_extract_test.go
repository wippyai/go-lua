package parserproducts

import (
	"go/parser"
	"testing"
)

func TestActionTermExtractionRejectsUnboundIdentifiersAndAcceptsControlTerms(t *testing.T) {
	builder := newActionTermBuilder(nil)
	scope := builder.scope(ActionScopeProduction, "expr#1", 0, 1, nil)
	unknown, err := parser.ParseExpr("missing")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builder.expression(&scope, unknown); err == nil {
		t.Fatal("unbound action identifier was accepted")
	}
	if id := builder.control(&scope, "Lexer"); id == 0 {
		t.Fatal("control term has no action identity")
	}
}
