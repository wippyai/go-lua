package lua

import (
	"testing"
	"time"
)

// TestZZDeadlockProbe runs the two deadlock regression fixtures through the exact
// canonical oracle path (canonicalFixtureDiagnostics) under a wall-clock guard so a
// non-termination shows as a timeout rather than a leaked goroutine. Diagnostic
// probe for the cutover-gate non-convergence investigation; not a correctness gate.
func TestZZDeadlockProbe(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	byName := make(map[string]namedSuite, len(suites))
	for _, s := range suites {
		byName[s.Name] = s
	}

	for _, name := range []string{
		"regression/deadlock-dataflow-node",
		"regression/deadlock-compiler-lua",
	} {
		name := name
		s, ok := byName[name]
		if !ok {
			t.Fatalf("fixture %q not found", name)
		}
		t.Run(name, func(t *testing.T) {
			type result struct {
				verdict oracleVerdict
				diags   int
				entry   string
			}
			done := make(chan result, 1)
			start := time.Now()
			go func() {
				diags, entry := canonicalFixtureDiagnostics(s)
				v := judgeAgainstCuratedExpectations(s, diags, entry)
				done <- result{v, len(diags), entry}
			}()
			select {
			case r := <-done:
				t.Logf("CONVERGED in %s: %d diagnostics, entry=%s, passed=%v", time.Since(start), r.diags, r.entry, r.verdict.passed)
				for _, m := range r.verdict.missing {
					t.Logf("    MISS: %s", m)
				}
				for _, u := range r.verdict.unexpected {
					t.Logf("    FALSE+: %s", u)
				}
			case <-time.After(60 * time.Second):
				t.Fatalf("NON-CONVERGENCE: %s did not terminate within 60s", name)
			}
		})
	}
}
