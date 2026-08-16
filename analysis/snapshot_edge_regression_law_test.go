package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/target/profile"
	"github.com/wippyai/go-lua/program/testfixture"
)

func TestCompileWithDiagnosticsAlwaysTrueGuardRejectsNoSnapshotTopologyEdge(t *testing.T) {
	project, err := testfixture.FrozenCorpusProject("advice/always-true-guard")
	if err != nil {
		t.Fatal(err)
	}
	contract, err := profile.Contract()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealCorpusProject(contract, project)
	if err != nil {
		t.Fatal(err)
	}
	plan, status, diagnostics := CompileWithDiagnostics(linked)
	if status != CompileComplete || plan == nil {
		t.Fatalf("receipt publish boundary remained incomplete: status=%v diagnostics=%+v", status, diagnostics)
	}
	defer plan.Close()
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
