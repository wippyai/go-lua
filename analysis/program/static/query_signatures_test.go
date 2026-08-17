package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestSignaturesQueryRootReturnsFunctionAndAssertionRows(t *testing.T) {
	component := staticContentComponent(t, signatureFixture(t))
	view := component.View().Signatures()
	function := keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1)
	if got, ok := view.TypeFunctions().At(0); !ok || got != function {
		t.Fatalf("TypeFunctions.At(0) = %v/%v", got, ok)
	}
	if got, ok := view.Assertions().At(0); !ok || got != keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1) {
		t.Fatalf("Assertions.At(0) = %v/%v", got, ok)
	}
	if count, ok := view.TypeFunctions().ParameterCount(function); !ok || count != 1 {
		t.Fatalf("TypeFunctions.ParameterCount() = %d/%v", count, ok)
	}
	if _, ok := view.Assertions().At(1); ok {
		t.Fatal("Assertions.At accepted out-of-range index")
	}
}
