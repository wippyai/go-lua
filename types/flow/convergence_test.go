package flow

import "testing"

// A solve that empties its worklist has converged; one that runs into the
// iteration cap has not, and says so rather than passing a partial solution off
// as a finished one.
func TestSolutionConvergedReportsCappedSolve(t *testing.T) {
	c, _, _, _, _ := buildBranchJoinCFG()
	inputs := newInputs(newMockSSAGraph(c))

	solution := Solve(inputs, testResolver())
	if !solution.Converged() {
		t.Fatalf("Converged() = false for a solve that finished in %d iterations", solution.DebugIterations())
	}

	original := maxIterationsPerPoint
	maxIterationsPerPoint = 0
	defer func() { maxIterationsPerPoint = original }()

	capped := Solve(inputs, testResolver())
	if capped.Converged() {
		t.Errorf("Converged() = true for a solve stopped at its iteration cap")
	}
}
