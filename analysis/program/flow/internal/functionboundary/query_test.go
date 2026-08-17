package functionboundary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestBoundaryQueriesExposeCanonicalFunctionBodyAndRootRows(t *testing.T) {
	result := validBoundaryResultForLaw(t)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	if result.Count() != 1 {
		t.Fatalf("Boundary count = %d, want 1", result.Count())
	}
	boundary, ok := result.At(0)
	if !ok || !boundary.Available() {
		t.Fatal("Boundary.At(0) did not expose the sealed Function row")
	}
	if got, ok := boundary.Function(); !ok || got != function {
		t.Fatalf("Boundary.Function = %v/%v, want %v/true", got, ok, function)
	}
	if got, ok := boundary.Body(); !ok || got != body {
		t.Fatalf("Boundary.Body = %v/%v, want %v/true", got, ok, body)
	}
	if got, ok := result.ForBody(body); !ok || !got.Available() {
		t.Fatal("ForBody did not resolve the canonical function Body boundary")
	}
	if root, ok := result.Root(); !ok || !root.Available() {
		t.Fatal("Root did not resolve the explicit assembly boundary")
	}
}
