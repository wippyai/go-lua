package analysis_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target/profile"
)

func analyzeCanonicalRealFixture(t *testing.T, path string, diagnosticOptions ...engine.SolveDiagnosticOptions) {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	program, err := lower.Lower(lower.Source{Name: "main.lua", Text: source})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := profile.Contract()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	lowered := time.Now()
	plan, compileStatus := analysis.Compile(linked)
	compiled := time.Now()
	if compileStatus != analysis.CompileComplete || plan == nil {
		t.Fatalf("Compile status=%d plan=%t", compileStatus, plan != nil)
	}
	var result *analysis.Result
	var status analysis.AnalyzeStatus
	var diagnostics analysis.AnalyzeDiagnostics
	if len(diagnosticOptions) != 0 {
		// The repository's bounded runner is the only wall/RSS safety authority.
		// An inner fixture timeout would turn a termination oracle into a timing
		// sample and discard the final permanent diagnostic snapshot.
		result, status, diagnostics = plan.SolveWithDiagnostics(context.Background(), diagnosticOptions[0])
	} else {
		result, status = plan.Solve(context.Background())
	}
	solved := time.Now()
	failure := diagnostics.Engine.Failure
	t.Logf("canonical real fixture: lower+link=%s compile=%s solve=%s total=%s phase=%s reason=%s rule=%s receipt-stage=%d artifact-row=%d receipt-ordinal=%d lowering=%d commit=%d commit-precondition=%d topology=%d schedule=%d engine={flags:%d work:%d/%d cutoff:%t epochs:%d passes:%d refresh:%d eval:%d fail:%d fold:%d rhs:%d restart:%d activation:%d failure:{available:%t reason:%d phase:%s point:%v group:%v member:%v rule:%v}}",
		lowered.Sub(started), compiled.Sub(lowered), solved.Sub(compiled), solved.Sub(started), diagnostics.Phase, diagnostics.Reason, diagnostics.Rule, diagnostics.ReceiptStage, diagnostics.ReceiptArtifactRow, diagnostics.ReceiptOrdinal, diagnostics.ReceiptLowering, diagnostics.ReceiptCommit, diagnostics.ReceiptCommitPrecondition, diagnostics.ReceiptTopology, diagnostics.ReceiptSchedule,
		diagnostics.Engine.Flags, diagnostics.Engine.Work, diagnostics.Engine.MaxWork, diagnostics.Engine.WorkCutoff,
		diagnostics.Engine.Epochs, diagnostics.Engine.EpochPasses, diagnostics.Engine.Refreshes, diagnostics.Engine.Evaluates, diagnostics.Engine.EvaluateFailures, diagnostics.Engine.Folds, diagnostics.Engine.RegionRHS, diagnostics.Engine.Restarts, diagnostics.Engine.Activations,
		failure.Available(), failure.Reason(), failure.Phase(), failure.Point(), failure.Group(), failure.Member(), failure.Rule())
	if diagnostics.Engine.WorkCutoff {
		if status != analysis.AnalyzeIncomplete || diagnostics.Phase != analysis.AnalyzeDiagnosticPhaseSolve || diagnostics.Reason != analysis.AnalyzeDiagnosticReasonWorkCutoff {
			t.Fatalf("diagnostic work cutoff = status:%d phase:%d reason:%d", status, diagnostics.Phase, diagnostics.Reason)
		}
		return
	}
	if status != analysis.AnalyzeComplete || result == nil || result.BodyCount() == 0 {
		t.Fatalf("Analyze status=%d result=%t bodies=%d", status, result != nil, func() int {
			if result == nil {
				return 0
			}
			return result.BodyCount()
		}())
	}
}

func TestDiagnosticCanonicalDeadlockCompilerLua(t *testing.T) {
	analyzeCanonicalRealFixture(t, "../testdata/fixtures/regression/deadlock-compiler-lua/main.lua", engine.SolveDiagnosticOptions{Flags: engine.SolveDiagnosticAll, MaxRows: 256, MaxWork: 65536})
}

func TestDiagnosticCanonicalDeadlockDataflowNode(t *testing.T) {
	analyzeCanonicalRealFixture(t, "../testdata/fixtures/regression/deadlock-dataflow-node/main.lua", engine.SolveDiagnosticOptions{Flags: engine.SolveDiagnosticAll, MaxRows: 256, MaxWork: 65536})
}

func TestDiagnosticCanonicalAdviceShapePolymorphic(t *testing.T) {
	analyzeCanonicalRealFixture(t, "../testdata/fixtures/advice/shape-polymorphic/main.lua", engine.SolveDiagnosticOptions{Flags: engine.SolveDiagnosticAll, MaxRows: 256})
}

func TestDiagnosticCanonicalControlForLoop(t *testing.T) {
	analyzeCanonicalRealFixture(t, "../testdata/fixtures/core/control-for-loop/main.lua", engine.SolveDiagnosticOptions{Flags: engine.SolveDiagnosticAll, MaxRows: 256})
}

func TestDiagnosticCanonicalBreakOutsideLoop(t *testing.T) {
	analyzeCanonicalRealFixture(t, "../testdata/fixtures/functions/break-outside-loop/main.lua", engine.SolveDiagnosticOptions{Flags: engine.SolveDiagnosticAll, MaxRows: 256})
}

func TestDiagnosticCanonicalBackwardGoto(t *testing.T) {
	analyzeCanonicalRealFixture(t, "../testdata/fixtures/functions/goto-backward/main.lua", engine.SolveDiagnosticOptions{Flags: engine.SolveDiagnosticAll, MaxRows: 256})
}
