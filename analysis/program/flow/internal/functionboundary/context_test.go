package functionboundary

import "testing"

func TestBoundaryContextHashesRemainCanonicalAndDistinct(t *testing.T) {
	result := validBoundaryResultForLaw(t)
	body := result.bodies[1]
	if got := hashBodyContext(result, body); got != body.context || !got.Available() {
		t.Fatalf("Body context hash = %v, want stored available context %v", got, body.context)
	}
	function := result.functions[0]
	if got := hashContext(result, function); got != function.context || !got.Available() {
		t.Fatalf("Function context hash = %v, want stored available context %v", got, function.context)
	}
	if body.context == function.context {
		t.Fatal("Body and Function context hashes collided")
	}
}
