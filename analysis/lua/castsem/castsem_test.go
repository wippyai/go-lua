package castsem

import "testing"

func TestPrimitiveCastTargetClassification(t *testing.T) {
	for _, name := range []string{"boolean", "number", "integer", "string"} {
		if IsTopLikeTarget(name) {
			t.Fatalf("%s should not be top-like cast target", name)
		}
	}
	if !IsAnyTarget("any") {
		t.Fatalf("any should be classified as explicit any")
	}
	if !IsUnknownTarget("unknown") {
		t.Fatalf("unknown should be classified as explicit unknown")
	}
	if !IsTopLikeTarget("any") || !IsTopLikeTarget("unknown") {
		t.Fatalf("any and unknown should be classified as top-like")
	}
}
