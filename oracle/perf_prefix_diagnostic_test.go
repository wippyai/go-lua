package oracle

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// The prefix lane measures how one fixture's analysis cost grows with its
// source. Its input is a truncation of a corpus fixture rather than a fixture
// directory, so it seals a single-module Link itself and then runs one compile,
// one diagnostic solve, and one closed plan.
func analyzeEdgeMatrixPrefix(t *testing.T, cases int) {
	t.Helper()
	project := corpusHarnessFixture(t, "semantic/type-engine-edge-matrix")
	source := corpusHarnessSourceText(t, project, "main.lua")
	end := edgeMatrixPrefixEnd(t, source, cases)
	linked, err := testfixture.SealSource(corpusHarnessContract(t), "main.lua", source[:end])
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	plan, status, diagnostics := analysis.CompileWithDiagnostics(linked)
	compile := time.Since(started)
	if status != analysis.CompileComplete || plan == nil {
		t.Fatalf("Analyze prefix%d compile = %v plan=%t diagnostics=%+v", cases, status, plan != nil, diagnostics)
	}
	defer plan.Close()
	started = time.Now()
	result, analyzeStatus, solveDiagnostics := plan.SolveWithDiagnostics(context.Background(), corpusHarnessSolveOptions())
	solve := time.Since(started)
	t.Logf("Analyze prefix%d: compile=%s solve=%s total=%s", cases, compile, solve, compile+solve)
	if analyzeStatus != analysis.AnalyzeComplete || result == nil {
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

// diagnosticEdgeMatrixPrefixCases are the sampled prefix boundaries. Each one
// is an independent compile-and-solve of its own truncated source, so each
// runs as its own subtest: a pattern naming one case number touches no other
// case's compile.
var diagnosticEdgeMatrixPrefixCases = []int{
	40, 100, 150, 160, 170, 180, 190, 220, 240, 242, 244, 248, 250, 260, 300, 340, 370,
}

func TestDiagnosticEdgeMatrixPrefix(t *testing.T) {
	for _, cases := range diagnosticEdgeMatrixPrefixCases {
		cases := cases
		t.Run(strconv.Itoa(cases), func(t *testing.T) {
			analyzeEdgeMatrixPrefix(t, cases)
		})
	}
}
