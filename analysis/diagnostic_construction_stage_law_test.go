// diagnostic_construction_stage_law_test.go proves the construction stage
// vocabulary: the analyzer names every boundary the program constructor
// publishes, names each one exactly once, and localizes nothing else.

package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
)

// TestAnalyzeDiagnosticConstructionStageIsTotalOverEngineBoundaries is the
// coverage law. The engine declares one boundary per refusal site of the
// program constructor; the analyzer must have a distinct stage for each, or a
// construction refusal reaches the acceptance suite with no localization. The
// engine's set is walked through its own minter, so a stage added there fails
// this law until the analyzer names it.
func TestAnalyzeDiagnosticConstructionStageIsTotalOverEngineBoundaries(t *testing.T) {
	named := make(map[AnalyzeDiagnosticReceiptStage]engine.ProgramConstructionStage)
	for ordinal := 1; ordinal < 256; ordinal++ {
		construction := engine.ProgramConstructionStage(ordinal)
		failure := engine.ProgramConstructionFailure(construction)
		if !failure.Available() {
			continue
		}
		stage, projected := analyzeDiagnosticConstructionStage(failure)
		if !projected {
			t.Fatalf("engine construction stage %d has no analyzer stage", ordinal)
		}
		if previous, duplicate := named[stage]; duplicate {
			t.Fatalf("engine construction stages %d and %d share analyzer stage %s", previous, construction, stage)
		}
		named[stage] = construction
		if rendered := stage.String(); rendered == "none" || rendered == "invalid" {
			t.Fatalf("analyzer stage for engine construction stage %d renders %q", ordinal, rendered)
		}
	}
	if len(named) == 0 {
		t.Fatal("the engine published no program construction boundary")
	}
}

// TestAnalyzeDiagnosticReceiptStageNamesAreTotalAndDistinct keeps the stage a
// readable localization. The acceptance harness interpolates this value as the
// only compile and solve failure locator it has, so an unnamed or shared member
// silently degrades that report.
func TestAnalyzeDiagnosticReceiptStageNamesAreTotalAndDistinct(t *testing.T) {
	owner := make(map[string]AnalyzeDiagnosticReceiptStage)
	for stage := AnalyzeDiagnosticReceiptStageNone; stage <= AnalyzeDiagnosticReceiptStageSolverMint; stage++ {
		rendered := stage.String()
		if rendered == "" || rendered == "invalid" {
			t.Fatalf("declared stage %d renders %q", stage, rendered)
		}
		if previous, duplicate := owner[rendered]; duplicate {
			t.Fatalf("stages %d and %d both render %q", previous, stage, rendered)
		}
		owner[rendered] = stage
	}
	if rendered := (AnalyzeDiagnosticReceiptStageSolverMint + 1).String(); rendered != "invalid" {
		t.Fatalf("the ordinal past the declared set renders %q", rendered)
	}
}

// TestEnterConstructionLocalizesOnlyConstructionBoundaries keeps the routing
// closed. Runtime binding ends at either the observation attach path or the
// program constructor, and both report through one failure value: a boundary
// the constructor did not raise must leave the stage the caller already
// recorded.
func TestEnterConstructionLocalizesOnlyConstructionBoundaries(t *testing.T) {
	diagnostics := AnalyzeDiagnostics{ReceiptStage: AnalyzeDiagnosticReceiptStageRuntime}
	for _, foreign := range []engine.SolveFailure{{}, engine.ReceiptCompilationAttachFailure(1)} {
		diagnostics.enterConstruction(foreign)
		if diagnostics.ReceiptStage != AnalyzeDiagnosticReceiptStageRuntime {
			t.Fatalf("boundary %v moved the stage to %s", foreign, diagnostics.ReceiptStage)
		}
	}
	diagnostics.enterConstruction(engine.ProgramConstructionFailure(engine.ProgramConstructionStageSolverMint))
	if diagnostics.ReceiptStage != AnalyzeDiagnosticReceiptStageSolverMint {
		t.Fatalf("the solver mint boundary localized as %s", diagnostics.ReceiptStage)
	}
}
