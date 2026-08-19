package diagnostic

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
)

func TestPublicAssembleStageDoesNotNameConstructionInteriors(t *testing.T) {
	for stage := AnalyzeDiagnosticAssembleStageNone; stage <= AnalyzeDiagnosticAssembleStageBootstrapRules; stage++ {
		rendered := stage.String()
		if rendered == "" || rendered == "invalid" {
			t.Fatalf("declared stage %d renders %q", stage, rendered)
		}
	}
	if rendered := (AnalyzeDiagnosticAssembleStageBootstrapRules + 1).String(); rendered != "invalid" {
		t.Fatalf("the ordinal past the analysis-owned set renders %q", rendered)
	}
	for _, leaked := range []string{"admission", "topology-seal", "query-address", "observation-address", "factor-bind", "member-bind", "program-seal", "solver-mint"} {
		for stage := AnalyzeDiagnosticAssembleStageNone; stage <= AnalyzeDiagnosticAssembleStageBootstrapRules; stage++ {
			if stage.String() == leaked {
				t.Fatalf("public assemble stage names construction interior %q", leaked)
			}
		}
	}
}

func TestEnterConstructionRecordsOpaqueFailureWithoutRenamingTheStage(t *testing.T) {
	diagnostics := AnalyzeDiagnostics{AssembleStage: AnalyzeDiagnosticAssembleStageRuntime}
	// A construction refusal is the only boundary that may occupy Construction.
	// The foreign set is the unavailable failure, a boundary of another family,
	// and a compile-family boundary of the seal authority whose ordinal names no
	// declared stage.
	for _, foreign := range []engine.SolveFailure{{}, engine.ObservationSealArguments(), engine.ProgramSealFailure(uint64(1) << 20)} {
		diagnostics.EnterConstruction(foreign)
		if diagnostics.AssembleStage != AnalyzeDiagnosticAssembleStageRuntime {
			t.Fatalf("boundary %v moved the stage to %s", foreign, diagnostics.AssembleStage)
		}
		if diagnostics.Construction.Available() {
			t.Fatalf("non-construction boundary occupied Construction: %v", diagnostics.Construction)
		}
	}
	mint := engine.ProgramStageFailure(engine.ProgramSealStageSolverMint)
	if !mint.Available() {
		t.Fatal("solver mint construction failure")
	}
	diagnostics.EnterConstruction(mint)
	if diagnostics.AssembleStage != AnalyzeDiagnosticAssembleStageRuntime {
		t.Fatalf("construction refusal renamed the public stage to %s", diagnostics.AssembleStage)
	}
	if !diagnostics.Construction.Available() || diagnostics.Construction.Site != mint.Site {
		t.Fatalf("construction refusal lost its opaque site: %v", diagnostics.Construction)
	}
	if strings.Contains(diagnostics.Construction.String(), "solver-mint") || strings.Contains(diagnostics.Construction.String(), "SolverMint") {
		t.Fatalf("construction site rendered an implementation name: %s", diagnostics.Construction)
	}
}

func TestConstructionRefusalsRemainDistinctBySite(t *testing.T) {
	var first, second engine.SolveFailure
	for ordinal := 1; ordinal < 256; ordinal++ {
		failure := engine.ProgramStageFailure(engine.ProgramSealStage(ordinal))
		if !failure.Available() {
			continue
		}
		if !first.Available() {
			first = failure
			continue
		}
		second = failure
		break
	}
	if !first.Available() || !second.Available() {
		t.Fatal("engine published fewer than two construction boundaries")
	}
	if first.Site == second.Site {
		t.Fatal("distinct construction boundaries share a public site")
	}
	if first.Family != second.Family {
		t.Fatal("construction refusals left the compile family")
	}
}
