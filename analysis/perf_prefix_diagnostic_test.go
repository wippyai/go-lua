package analysis_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target/profile"
)

func analyzeEdgeMatrixPrefix(t *testing.T, cases int) {
	t.Helper()
	source, err := os.ReadFile("../testdata/fixtures/semantic/type-engine-edge-matrix/main.lua")
	if err != nil {
		t.Fatal(err)
	}
	end := -1
	if cases == 370 {
		end = len(source)
	} else if lineEnd, ok := map[int]int{150: 390, 160: 426, 170: 461, 180: 496, 190: 531, 220: 636, 240: 716, 242: 723, 244: 732, 248: 748, 250: 755, 260: 796, 300: 966, 340: 1086}[cases]; ok {
		lines := strings.SplitAfter(string(source), "\n")
		if lineEnd <= 0 || lineEnd > len(lines) {
			t.Fatal("line boundary unavailable")
		}
		end = len(strings.Join(lines[:lineEnd], ""))
	} else {
		marker := []byte("-- case " + fmt.Sprintf("%03d", cases+1) + ":")
		end = strings.Index(string(source), string(marker))
		if end < 0 {
			t.Fatal("case marker unavailable")
		}
	}
	program, err := lower.Lower(lower.Source{Name: "main.lua", Text: source[:end]})
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
	started := time.Now()
	plan, compileStatus := analysis.Compile(linked)
	compiled := time.Now()
	if compileStatus != analysis.CompileComplete || plan == nil {
		t.Fatalf("Compile prefix%d: status=%d plan=%v", cases, compileStatus, plan != nil)
	}
	result, status, diagnostics := plan.SolveWithDiagnostics(context.Background(), engine.SolveDiagnosticOptions{Flags: engine.SolveDiagnosticAll, MaxRows: 256})
	solved := time.Now()
	t.Logf("Analyze prefix%d: compile=%s solve=%s total=%s", cases, compiled.Sub(started), solved.Sub(compiled), solved.Sub(started))
	if status != analysis.AnalyzeComplete || result == nil || result.BodyCount() == 0 {
		failure := diagnostics.Engine.Failure
		t.Logf("Analyze prefix%d diagnostics: phase=%s reason=%s rule=%s engine={flags:%d work:%d/%d cutoff:%t epochs:%d passes:%d refresh:%d eval:%d fail:%d fold:%d rhs:%d restart:%d activation:%d failure:{available:%t reason:%d phase:%s point:%v group:%v member:%v rule:%v}}",
			cases, diagnostics.Phase, diagnostics.Reason, diagnostics.Rule,
			diagnostics.Engine.Flags, diagnostics.Engine.Work, diagnostics.Engine.MaxWork, diagnostics.Engine.WorkCutoff,
			diagnostics.Engine.Epochs, diagnostics.Engine.EpochPasses, diagnostics.Engine.Refreshes, diagnostics.Engine.Evaluates, diagnostics.Engine.EvaluateFailures, diagnostics.Engine.Folds, diagnostics.Engine.RegionRHS, diagnostics.Engine.Restarts, diagnostics.Engine.Activations,
			failure.Available(), failure.Reason(), failure.Phase(), failure.Point(), failure.Group(), failure.Member(), failure.Rule())
		t.Fatalf("Analyze prefix%d: status=%d result=%v", cases, status, result != nil)
	}
}

func TestDiagnosticEdgeMatrixPrefix40(t *testing.T)  { analyzeEdgeMatrixPrefix(t, 40) }
func TestDiagnosticEdgeMatrixPrefix100(t *testing.T) { analyzeEdgeMatrixPrefix(t, 100) }
func TestDiagnosticEdgeMatrixPrefix150(t *testing.T) { analyzeEdgeMatrixPrefix(t, 150) }
func TestDiagnosticEdgeMatrixPrefix160(t *testing.T) { analyzeEdgeMatrixPrefix(t, 160) }
func TestDiagnosticEdgeMatrixPrefix170(t *testing.T) { analyzeEdgeMatrixPrefix(t, 170) }
func TestDiagnosticEdgeMatrixPrefix180(t *testing.T) { analyzeEdgeMatrixPrefix(t, 180) }
func TestDiagnosticEdgeMatrixPrefix190(t *testing.T) { analyzeEdgeMatrixPrefix(t, 190) }
func TestDiagnosticEdgeMatrixPrefix220(t *testing.T) { analyzeEdgeMatrixPrefix(t, 220) }
func TestDiagnosticEdgeMatrixPrefix240(t *testing.T) { analyzeEdgeMatrixPrefix(t, 240) }
func TestDiagnosticEdgeMatrixPrefix242(t *testing.T) { analyzeEdgeMatrixPrefix(t, 242) }
func TestDiagnosticEdgeMatrixPrefix244(t *testing.T) { analyzeEdgeMatrixPrefix(t, 244) }
func TestDiagnosticEdgeMatrixPrefix248(t *testing.T) { analyzeEdgeMatrixPrefix(t, 248) }
func TestDiagnosticEdgeMatrixPrefix250(t *testing.T) { analyzeEdgeMatrixPrefix(t, 250) }
func TestDiagnosticEdgeMatrixPrefix260(t *testing.T) { analyzeEdgeMatrixPrefix(t, 260) }
func TestDiagnosticEdgeMatrixPrefix300(t *testing.T) { analyzeEdgeMatrixPrefix(t, 300) }
func TestDiagnosticEdgeMatrixPrefix340(t *testing.T) { analyzeEdgeMatrixPrefix(t, 340) }
func TestDiagnosticEdgeMatrixPrefix370(t *testing.T) { analyzeEdgeMatrixPrefix(t, 370) }
