package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
)

func TestCompileWithDiagnosticsAlwaysTrueGuardRejectsNoSnapshotTopologyEdge(t *testing.T) {
	diagnostics := corpusHarnessFixtureRun(t, "advice/always-true-guard", corpusHarnessCompileMode()).compileDiagnostics
	if diagnostics.ReceiptCommitPublish != 0 {
		t.Fatalf("receipt publish boundary reported a publish subfailure after repair: %v", diagnostics.ReceiptCommitPublish)
	}
	if diagnostics.ReceiptLowering == engine.ReceiptAssemblyFailureSnapshotTopologyEdge {
		t.Fatalf("CompileWithDiagnostics regressed to snapshot topology edge: %+v", diagnostics)
	}
	// The fixture may remain incomplete at a later commit/publish stage while
	// semantic diagnostics are still being cut over; this law isolates the
	// repaired receipt boundary only.
}
