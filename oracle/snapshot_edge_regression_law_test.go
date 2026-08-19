package oracle

import "testing"

func TestCompileWithDiagnosticsAlwaysTrueGuardRejectsNoSnapshotTopologyEdge(t *testing.T) {
	run := corpusHarnessFixtureRun(t, "advice/always-true-guard", corpusHarnessCompileMode())
	if run.compileDiagnostics.AssembleCommit.Available() {
		t.Fatalf("assemble commit boundary reported a failure after repair: %v", run.compileDiagnostics.AssembleCommit)
	}
	if run.compileDiagnostics.AssembleLowering.Available() {
		t.Fatalf("CompileWithDiagnostics regressed to a lowering boundary: %v", run.compileDiagnostics.AssembleLowering)
	}
	// The fixture may remain incomplete at a later commit/publish stage while
	// semantic diagnostics are still being cut over; this law isolates the
	// repaired assemble boundary only.
}
