package oracle

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program/target"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func TestPlanDiagnosticsRejectInvalidOptionsAtSetup(t *testing.T) {
	contract, err := target.Seal(&target.Spec{Semantics: domaincontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealSource(contract, "diagnostics_test.lua", []byte(`return 1`))
	if err != nil {
		t.Fatal(err)
	}
	plan, status := analysis.Compile(linked)
	if status != analysis.CompileComplete || plan == nil {
		t.Fatal("diagnostic Plan fixture")
	}
	result, analyzeStatus, diagnostics := plan.SolveWithDiagnostics(context.Background(), engine.SolveDiagnosticOptions{MaxRows: 1})
	if result != nil || analyzeStatus != analysis.AnalyzeInvalid || diagnostics.Phase != analysis.AnalyzeDiagnosticPhaseSetup || diagnostics.Reason != analysis.AnalyzeDiagnosticReasonInvalidOptions {
		t.Fatalf("invalid diagnostic options = result:%t status:%v phase:%v reason:%v", result != nil, analyzeStatus, diagnostics.Phase, diagnostics.Reason)
	}
	result, analyzeStatus, diagnostics = plan.SolveWithDiagnostics(nil, engine.SolveDiagnosticOptions{})
	if result != nil || analyzeStatus != analysis.AnalyzeInvalid || diagnostics.Phase != analysis.AnalyzeDiagnosticPhaseSetup || diagnostics.Reason != analysis.AnalyzeDiagnosticReasonInvalidPlan {
		t.Fatalf("invalid diagnostic context = result:%t status:%v phase:%v reason:%v", result != nil, analyzeStatus, diagnostics.Phase, diagnostics.Reason)
	}
	var zero analysis.Plan
	result, analyzeStatus, diagnostics = zero.SolveWithDiagnostics(context.Background(), engine.SolveDiagnosticOptions{})
	if result != nil || analyzeStatus != analysis.AnalyzeInvalid || diagnostics.Phase != analysis.AnalyzeDiagnosticPhaseSetup || diagnostics.Reason != analysis.AnalyzeDiagnosticReasonInvalidPlan {
		t.Fatalf("invalid diagnostic plan = result:%t status:%v phase:%v reason:%v", result != nil, analyzeStatus, diagnostics.Phase, diagnostics.Reason)
	}
}
