package regression

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	checkpkg "github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestCanonicalFlow_DeadlockDataflowNodeModuleTerminates is the key gate of the
// canonical-flow cutover (DAG component 11a): running the canonical engine over
// the deadlock-dataflow-node module CONVERGES and returns, where the legacy flow
// deadlocks (its widening-free intraprocedural SCC solver never reaches a fixed
// point on the module's counting loops, hitting the 30s suite timeout).
//
// The module has several `count = count + 1` loops inside `for _ in pairs(...)`,
// the exact strictly-ascending-counter shape that diverges under pure join. The
// canonical solver terminates because the numeric domain's widening fires at the
// loop-header feedback-vertex set, cutting the ascending bound to unconstrained.
//
// This proves the cutover MECHANISM end to end: a Checker constructed
// WithCanonicalFlow drives the canonical driver over a whole real module and the
// interprocedural summary fixed point converges. Diagnostic parity is a later
// component (11b); here the deliverable is that the flow RUNS and TERMINATES.
func TestCanonicalFlow_DeadlockDataflowNodeModuleTerminates(t *testing.T) {
	src := readFixtureSource(t, "deadlock-dataflow-node")

	// Convergence is by design; the bound here makes a non-termination regression
	// fail fast rather than wait for the process -timeout. The summary fixpoint
	// over this module converges in well under a second.
	done := make(chan struct{})
	go func() {
		defer close(done)
		// The required modules (json, uuid, expr, ...) are unresolved here; the
		// canonical transfer treats unresolved calls as the sound value-domain
		// default, which does not affect termination.
		_ = testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(checkpkg.WithCanonicalFlow()))
	}()

	select {
	case <-done:
		// Converged and returned: the canonical flow terminates on the module.
	case <-time.After(15 * time.Second):
		t.Fatal("canonical flow did not terminate on deadlock-dataflow-node within 15s")
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
