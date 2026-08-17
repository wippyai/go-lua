package analysis

import (
	"context"
	"testing"
)

// TestPublicAnalyzeSemanticTypeEngineEdgeMatrix drives the public one-shot
// Analyze entry over the corpus' widest semantic fixture. It is this package's
// named single-fixture probe of that entry: one fixture, one Analyze, one
// completed status.
func TestPublicAnalyzeSemanticTypeEngineEdgeMatrix(t *testing.T) {
	linked := fixtureLink(t, "semantic/type-engine-edge-matrix")
	result, status := Analyze(context.Background(), linked)
	if status != AnalyzeComplete || result == nil {
		t.Fatalf("Analyze = %v result=%t", status, result != nil)
	}
	if !result.ContentID().Available() || result.SourceID() != linked.ContentID() {
		t.Fatalf("Result carries no detached identity of its source")
	}
}
