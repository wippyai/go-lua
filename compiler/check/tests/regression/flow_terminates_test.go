package regression

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestFlow_DeadlockDataflowNodeModuleTerminates verifies that the checker
// returns on the deadlock-dataflow-node module.
//
// The module has several `count = count + 1` loops inside `for _ in pairs(...)`,
// the exact strictly-ascending-counter shape that diverges under pure join. The
// The solver terminates because the numeric domain's widening fires at the
// loop-header feedback-vertex set, cutting the ascending bound to unconstrained.
func TestFlow_DeadlockDataflowNodeModuleTerminates(t *testing.T) {
	src := readFixtureSource(t, "deadlock-dataflow-node")

	// Convergence is by design; the bound here makes a non-termination regression
	// fail fast rather than wait for the process -timeout. The summary fixpoint
	// over this module converges in well under a second.
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = testutil.Check(src, testutil.WithStdlib())
	}()

	select {
	case <-done:
		// Converged and returned.
	case <-time.After(15 * time.Second):
		t.Fatal("flow did not terminate on deadlock-dataflow-node within 15s")
	}
}

// readFixtureSource reads a fixture's main.lua from the repository testdata tree,
// relative to this regression test package.
func readFixtureSource(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "testdata", "fixtures", "regression", name, "main.lua")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}
