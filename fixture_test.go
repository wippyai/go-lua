package lua

import (
	"os"
	"strconv"
	"testing"
	"time"
)

// fixtureDeadline bounds how long any single fixture phase (check/run) may
// take before the harness reports it as non-convergence. A fixture that
// exceeds the deadline is a divergence signal: an abstract-interpretation
// fixpoint failed to terminate, or per-point widening is missing on a
// domain that lacks the ascending-chain property. The deadline is enforced
// per phase so divergence surfaces as a clean per-fixture FAIL message
// instead of the whole test binary getting killed by the outer go-test
// timeout (which masks WHICH fixture diverged).
//
// See Forge journal seq 304 for the foundational design that requires this.
func fixtureDeadline() time.Duration {
	if v := os.Getenv("FIXTURE_DEADLINE_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 30 * time.Second
}

// runWithDeadline runs fn under the per-fixture deadline. On timeout it
// fails the test with a non-convergence diagnostic. The worker goroutine
// may continue running until the test binary exits, but its result is
// ignored. fn may call t.* methods; t.Fatal in fn (via FailNow/Goexit)
// terminates the worker cleanly through the deferred done signal.
func runWithDeadline(t *testing.T, phase string, fn func(t *testing.T)) {
	t.Helper()
	deadline := fixtureDeadline()
	done := make(chan any, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- r
				return
			}
			done <- nil
		}()
		fn(t)
	}()
	select {
	case r := <-done:
		if r != nil {
			// Re-raise the worker's panic on the test goroutine so the
			// stack and goroutine state are attributed correctly.
			panic(r)
		}
	case <-time.After(deadline):
		t.Fatalf("non-convergence: %s did not complete within %s — abstract-interpreter fixpoint failed to terminate (see Forge journal seq 304)", phase, deadline)
	}
}

func TestFixtures(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	if len(suites) == 0 {
		t.Fatal("no fixture suites found")
	}
	for _, s := range suites {
		s := s
		t.Run(s.Name, func(t *testing.T) {
			if s.Suite.Skip != "" {
				t.Skip(s.Suite.Skip)
			}
			t.Run("check", func(t *testing.T) {
				runWithDeadline(t, "check", func(t *testing.T) {
					runCheckPhase(t, s)
				})
			})
			t.Run("run", func(t *testing.T) {
				runWithDeadline(t, "run", func(t *testing.T) {
					runExecPhase(t, s)
				})
			})
		})
	}
}

func BenchmarkFixtures(b *testing.B) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		b.Fatalf("discovering fixtures: %v", err)
	}
	for _, s := range suites {
		if s.Suite.Bench == nil {
			continue
		}
		s := s
		b.Run(s.Name, func(b *testing.B) {
			runBenchPhase(b, s)
		})
	}
}

func TestFixtureOrder_GenericRegistryThenMultiReturn(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}

	var generic namedSuite
	var multi namedSuite
	for _, s := range suites {
		switch s.Name {
		case "realworld/generic-registry":
			generic = s
		case "realworld/multi-return-error-chain":
			multi = s
		}
	}

	if generic.Name == "" || multi.Name == "" {
		t.Fatalf("missing target suites: generic=%q multi=%q", generic.Name, multi.Name)
	}

	runCheckPhase(t, generic)
	runCheckPhase(t, multi)
}
