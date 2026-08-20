package oracle

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// TestNumericLoopWithGuardedBodyConverges is the analysis-level statement of
// the close reuse law.  A numeric for whose body carries one guarded branch is
// the minimal program whose recurrence head re-closes a stable exact RHS
// against the root published by the previous pass.  When that close cannot
// retain the predecessor root, every pass republishes an unchanged plane, so
// the region head re-dirties itself forever and raw-only publications grow
// without bound.  The law is stated by convergence alone: the solve completes
// and its raw-only publication count stays bounded.  A solve that fails to
// converge does not return, and the absence of a returned complete status is
// the failure this law reports.
func TestNumericLoopWithGuardedBodyConverges(t *testing.T) {
	const source = "for i=1,100 do if i>10 then end end\n"

	program, err := lower.Lower(lower.Source{Name: "main.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	plan, compileStatus := analysis.Compile(linked)
	if plan != nil {
		defer plan.Close()
	}
	if compileStatus != analysis.CompileComplete || plan == nil {
		t.Fatalf("compile: status=%d plan=%t", compileStatus, plan != nil)
	}
	result, status, diagnostics := plan.SolveWithDiagnostics(context.Background(), engine.SolveDiagnosticOptions{
		Presentation: engine.SolveDiagnosticPresentation{Flags: engine.SolveDiagnosticAll},
		Resources:    engine.SolveDiagnosticResources{MaxRows: 256},
	})
	t.Logf("guarded numeric loop: epochs=%d passes=%d publications=%d rawOnly=%d bumps=%d restarts=%d",
		diagnostics.Engine.Epochs, diagnostics.Engine.EpochPasses, diagnostics.Engine.Publications,
		diagnostics.Engine.RawOnlyPublications, diagnostics.Engine.VersionBumps, diagnostics.Engine.Restarts)
	if status != analysis.AnalyzeComplete || result == nil {
		t.Fatalf("solve: status=%d result=%t phase=%s reason=%s", status, result != nil, diagnostics.Phase, diagnostics.Reason)
	}
	// A converged head publishes its closed plane once per genuine change.
	// Linear growth in raw-only publications is the observable signature of a
	// root-identity limit cycle at an unchanged lattice value.
	if diagnostics.Engine.RawOnlyPublications > 32 {
		t.Fatalf("raw-only publications = %d, want a bounded handful", diagnostics.Engine.RawOnlyPublications)
	}
}
