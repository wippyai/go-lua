package analysis

import "testing"

// TestPublicAnalyzeSemanticTypeEngineEdgeMatrix drives the public one-shot
// Analyze entry over the corpus' widest semantic fixture. It is the harness'
// named single-fixture census probe: same enumeration, same detached Result
// contract, one fixture.
func TestPublicAnalyzeSemanticTypeEngineEdgeMatrix(t *testing.T) {
	corpusHarnessFixtureRun(t, "semantic/type-engine-edge-matrix", corpusHarnessCensusMode())
}
