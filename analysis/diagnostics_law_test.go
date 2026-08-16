package analysis

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
)

func TestPlanDiagnosticsRejectInvalidOptionsAtSetup(t *testing.T) {
	plan, status := Compile(directFieldHostileLink(t, `return 1`))
	if status != CompileComplete || plan == nil {
		t.Fatal("diagnostic Plan fixture")
	}
	result, analyzeStatus, diagnostics := plan.SolveWithDiagnostics(context.Background(), engine.SolveDiagnosticOptions{MaxWork: 1})
	if result != nil || analyzeStatus != AnalyzeInvalid || diagnostics.Phase != AnalyzeDiagnosticPhaseSetup || diagnostics.Reason != AnalyzeDiagnosticReasonInvalidOptions {
		t.Fatalf("invalid diagnostic options = result:%t status:%v phase:%v reason:%v", result != nil, analyzeStatus, diagnostics.Phase, diagnostics.Reason)
	}
	result, analyzeStatus, diagnostics = plan.SolveWithDiagnostics(nil, engine.SolveDiagnosticOptions{})
	if result != nil || analyzeStatus != AnalyzeInvalid || diagnostics.Phase != AnalyzeDiagnosticPhaseSetup || diagnostics.Reason != AnalyzeDiagnosticReasonInvalidPlan {
		t.Fatalf("invalid diagnostic context = result:%t status:%v phase:%v reason:%v", result != nil, analyzeStatus, diagnostics.Phase, diagnostics.Reason)
	}
	var zero Plan
	result, analyzeStatus, diagnostics = zero.SolveWithDiagnostics(context.Background(), engine.SolveDiagnosticOptions{})
	if result != nil || analyzeStatus != AnalyzeInvalid || diagnostics.Phase != AnalyzeDiagnosticPhaseSetup || diagnostics.Reason != AnalyzeDiagnosticReasonInvalidPlan {
		t.Fatalf("invalid diagnostic plan = result:%t status:%v phase:%v reason:%v", result != nil, analyzeStatus, diagnostics.Phase, diagnostics.Reason)
	}
}
