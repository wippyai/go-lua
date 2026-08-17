package analysis

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// The prefix lane measures how one fixture's analysis cost grows with its
// source. Its input is a truncation of a corpus fixture rather than a fixture
// directory, so it seals a single-module Link itself and then runs one compile,
// one diagnostic solve, and one closed plan.
func analyzeEdgeMatrixPrefix(t *testing.T, cases int) {
	t.Helper()
	source, err := fixtureProject(t, "semantic/type-engine-edge-matrix").SourceText("main.lua")
	if err != nil {
		t.Fatal(err)
	}
	end := edgeMatrixPrefixEnd(t, source, cases)
	linked := fixtureSourceLink(t, fixtureContract(t), "main.lua", source[:end])
	started := time.Now()
	plan, status, diagnostics := CompileWithDiagnostics(linked)
	compile := time.Since(started)
	if status != CompileComplete || plan == nil {
		t.Fatalf("Analyze prefix%d compile = %v plan=%t diagnostics=%+v", cases, status, plan != nil, diagnostics)
	}
	defer plan.Close()
	started = time.Now()
	result, analyzeStatus, solveDiagnostics := plan.SolveWithDiagnostics(context.Background(), fixtureSolveOptions())
	solve := time.Since(started)
	t.Logf("Analyze prefix%d: compile=%s solve=%s total=%s", cases, compile, solve, compile+solve)
	if analyzeStatus != AnalyzeComplete || result == nil {
		t.Fatalf("Analyze prefix%d solve = %v result=%t diagnostics=%+v", cases, analyzeStatus, result != nil, solveDiagnostics)
	}
}

// edgeMatrixPrefixEnd resolves one measured prefix boundary: a case marker for
// the sampled counts, a line boundary for the counts whose marker spans
// several cases, and the whole source for the complete fixture.
func edgeMatrixPrefixEnd(t *testing.T, source []byte, cases int) int {
	t.Helper()
	if cases == 370 {
		return len(source)
	}
	if lineEnd, ok := map[int]int{150: 390, 160: 426, 170: 461, 180: 496, 190: 531, 220: 636, 240: 716, 242: 723, 244: 732, 248: 748, 250: 755, 260: 796, 300: 966, 340: 1086}[cases]; ok {
		lines := strings.SplitAfter(string(source), "\n")
		if lineEnd <= 0 || lineEnd > len(lines) {
			t.Fatal("line boundary unavailable")
		}
		return len(strings.Join(lines[:lineEnd], ""))
	}
	end := strings.Index(string(source), "-- case "+fmt.Sprintf("%03d", cases+1)+":")
	if end < 0 {
		t.Fatal("case marker unavailable")
	}
	return end
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
