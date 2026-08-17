package analysis

import (
	"context"
	"testing"
	"time"
)

// The real-fixture lane names individual corpus fixtures whose analysis cost
// or termination is worth reporting on its own, outside a shard walk. It runs
// one compile and one diagnostic solve and reports the fixture's measured cost;
// a work cutoff is a failed analysis here, never a passed sample, so the lane
// carries no work budget and the bounded runner remains the only resource
// authority.
func analyzeCanonicalRealFixture(t *testing.T, name string) {
	t.Helper()
	sealed := time.Now()
	linked := fixtureLink(t, name)
	compiled := time.Now()
	plan, status, diagnostics := CompileWithDiagnostics(linked)
	if status != CompileComplete || plan == nil {
		t.Fatalf("compile fixture %q = %v plan=%t diagnostics=%+v", name, status, plan != nil, diagnostics)
	}
	defer plan.Close()
	solved := time.Now()
	result, analyzeStatus, solveDiagnostics := plan.SolveWithDiagnostics(context.Background(), fixtureSolveOptions())
	if analyzeStatus != AnalyzeComplete || result == nil {
		t.Fatalf("solve fixture %q = %v result=%t diagnostics=%+v", name, analyzeStatus, result != nil, solveDiagnostics)
	}
	seal, compile, solve := compiled.Sub(sealed), solved.Sub(compiled), time.Since(solved)
	t.Logf("canonical real fixture %s: seal=%s compile=%s solve=%s total=%s", name, seal, compile, solve, seal+compile+solve)
}

func TestDiagnosticCanonicalDeadlockCompilerLua(t *testing.T) {
	analyzeCanonicalRealFixture(t, "regression/deadlock-compiler-lua")
}

func TestDiagnosticCanonicalDeadlockDataflowNode(t *testing.T) {
	analyzeCanonicalRealFixture(t, "regression/deadlock-dataflow-node")
}

func TestDiagnosticCanonicalAdviceShapePolymorphic(t *testing.T) {
	analyzeCanonicalRealFixture(t, "advice/shape-polymorphic")
}

func TestDiagnosticCanonicalControlForLoop(t *testing.T) {
	analyzeCanonicalRealFixture(t, "core/control-for-loop")
}

func TestDiagnosticCanonicalBreakOutsideLoop(t *testing.T) {
	analyzeCanonicalRealFixture(t, "functions/break-outside-loop")
}

func TestDiagnosticCanonicalBackwardGoto(t *testing.T) {
	analyzeCanonicalRealFixture(t, "functions/goto-backward")
}
