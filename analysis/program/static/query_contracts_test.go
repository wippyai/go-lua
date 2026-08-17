package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestContractsQueryRootEnumeratesCanonicalFunctionAndCallTerms(t *testing.T) {
	component := staticContentComponent(t, contractsFixture(t))
	view := component.View().Contracts()
	if got, ok := view.Functions().At(0); !ok || got != keyspace.MakeTerm(keyspace.FamilyFunction, 1) {
		t.Fatalf("Functions.At(0) = %v/%v", got, ok)
	}
	if got, ok := view.Calls().At(0); !ok || got != keyspace.MakeTerm(keyspace.FamilyCall, 1) {
		t.Fatalf("Calls.At(0) = %v/%v", got, ok)
	}
	if _, ok := view.Functions().At(-1); ok {
		t.Fatal("Functions.At accepted negative index")
	}
	if _, ok := view.Calls().At(1); ok {
		t.Fatal("Calls.At accepted out-of-range index")
	}
}
