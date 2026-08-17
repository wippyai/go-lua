package oracle

import "testing"

// TestPublicAnalyzeSemanticTypeEngineEdgeMatrix drives the public one-shot
// Analyze entry over the corpus' widest semantic fixture through the corpus
// spine. It is this package's named single-fixture probe of that entry: one
// fixture, one Analyze, one completed status, and the shared detached-Result
// contract.
func TestPublicAnalyzeSemanticTypeEngineEdgeMatrix(t *testing.T) {
	corpusHarnessFixtureRun(t, "semantic/type-engine-edge-matrix", corpusHarnessCensusMode())
}
