package analysis

import "testing"

// The real-fixture lane names individual corpus fixtures whose analysis cost
// or termination is worth reporting on its own, outside a shard walk. It runs
// the shared harness spine and reports the fixture's measured cost; a work
// cutoff is a failed analysis here, never a passed sample, so the lane carries
// no work budget and the bounded runner remains the only resource authority.
func analyzeCanonicalRealFixture(t *testing.T, name string) {
	t.Helper()
	run := corpusHarnessFixtureRun(t, name, corpusHarnessReceiptMode())
	t.Logf("canonical real fixture %s: seal=%s compile=%s solve=%s total=%s", name, run.cost.seal, run.cost.compile, run.cost.solve, run.cost.total())
}

func TestDiagnosticCanonicalDeadlockCompilerLua(t *testing.T) {
	analyzeCanonicalRealFixture(t, "regression/deadlock-compiler-lua")
}

func TestDiagnosticCanonicalDeadlockDataflowNode(t *testing.T) {
	analyzeCanonicalRealFixture(t, "regression/deadlock-dataflow-node")
}

func TestDiagnosticCanonicalAdviceShapePolymorphic(t *testing.T) {
	analyzeCanonicalRealFixture(t, "advice/shape-polymorphic")
}

func TestDiagnosticCanonicalControlForLoop(t *testing.T) {
	analyzeCanonicalRealFixture(t, "core/control-for-loop")
}

func TestDiagnosticCanonicalBreakOutsideLoop(t *testing.T) {
	analyzeCanonicalRealFixture(t, "functions/break-outside-loop")
}

func TestDiagnosticCanonicalBackwardGoto(t *testing.T) {
	analyzeCanonicalRealFixture(t, "functions/goto-backward")
}
