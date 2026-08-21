package functionboundary_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestBoundaryQueriesExposeCanonicalFunctionBodyRootAndOutcomeRows(t *testing.T) {
	program := lowerFunctionBoundary(t, "local function f(a) return a end return f(1)")
	view := program.Flow()
	result := view.FunctionBoundaries()
	if result.Count() != view.Authored().Functions().Count() {
		t.Fatalf("Boundary count = %d, authored functions = %d", result.Count(), view.Authored().Functions().Count())
	}
	function, ok := view.Authored().Functions().At(0)
	if !ok {
		t.Fatal("fixture did not publish a Function")
	}
	boundary, ok := result.For(function)
	if !ok || !boundary.Available() {
		t.Fatal("For did not expose the sealed Function row")
	}
	_, body, _, ok := view.Authored().Functions().Get(function)
	if !ok {
		t.Fatal("authored Function row was unavailable")
	}
	if got, ok := boundary.Body(); !ok || got != body {
		t.Fatalf("Boundary.Body = %v/%v, want %v/true", got, ok, body)
	}
	if got, ok := result.ForBody(body); !ok || !got.Available() {
		t.Fatal("ForBody did not resolve the canonical function Body boundary")
	}
	root, ok := result.Root()
	if !ok || !root.Available() {
		t.Fatal("Root did not resolve the explicit assembly boundary")
	}
	if got, ok := root.Body(); !ok || keyspace.TermFamily(got) != keyspace.FamilyBody {
		t.Fatalf("Root.Body = %v/%v, want a Body", got, ok)
	}
	for index := 0; index < boundary.OutcomeCount(); index++ {
		exit, ok := boundary.OutcomeAt(index)
		if !ok || exit.Body != body {
			t.Fatalf("Function Outcome[%d] = %#v/%v, want owner %v", index, exit, ok, body)
		}
		owner, ownerOK := result.ForFunctionOutcome(exit.Outcome)
		if !ownerOK || !owner.Equal(boundary) {
			t.Fatalf("Function Outcome inverse[%d] failed", index)
		}
	}
}
