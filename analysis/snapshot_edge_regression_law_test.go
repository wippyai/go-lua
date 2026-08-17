package analysis

import "testing"

func TestCompileWithDiagnosticsAlwaysTrueGuardRejectsNoSnapshotTopologyEdge(t *testing.T) {
	diagnostics := corpusHarnessFixtureRun(t, "advice/always-true-guard", corpusHarnessCompileMode()).compileDiagnostics
	if diagnostics.ReceiptCommit.Available() {
		t.Fatalf("receipt commit boundary reported a failure after repair: %v", diagnostics.ReceiptCommit)
	}
	if diagnostics.ReceiptLowering.Available() {
		t.Fatalf("CompileWithDiagnostics regressed to a lowering boundary: %v", diagnostics.ReceiptLowering)
	}
	// The fixture may remain incomplete at a later commit/publish stage while
	// semantic diagnostics are still being cut over; this law isolates the
	// repaired receipt boundary only.
}
