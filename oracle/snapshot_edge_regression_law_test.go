package oracle

import "testing"

func TestCompileWithDiagnosticsAlwaysTrueGuardRejectsNoSnapshotTopologyEdge(t *testing.T) {
	run := corpusHarnessFixtureRun(t, "advice/always-true-guard", corpusHarnessCompileMode())
	if run.compileDiagnostics.ReceiptCommit.Available() {
		t.Fatalf("receipt commit boundary reported a failure after repair: %v", run.compileDiagnostics.ReceiptCommit)
	}
	if run.compileDiagnostics.ReceiptLowering.Available() {
		t.Fatalf("CompileWithDiagnostics regressed to a lowering boundary: %v", run.compileDiagnostics.ReceiptLowering)
	}
	// The fixture may remain incomplete at a later commit/publish stage while
	// semantic diagnostics are still being cut over; this law isolates the
	// repaired receipt boundary only.
}
