package lua

import (
	"os"
	"strconv"
	"testing"
	"time"
)

const defaultFixtureDeadline = 30 * time.Second

// fixtureDeadline bounds a normal fixture step. FIXTURE_DEADLINE_SECONDS is a
// local/CI override for stress runs; fixture manifests may request a larger
// budget for intentionally broad adversarial suites.
func fixtureDeadline() time.Duration {
	if v := os.Getenv("FIXTURE_DEADLINE_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return defaultFixtureDeadline
}

func fixtureDeadlineForSuite(s namedSuite) time.Duration {
	if v := os.Getenv("FIXTURE_DEADLINE_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	if s.Suite.DeadlineSeconds > 0 {
		return time.Duration(s.Suite.DeadlineSeconds) * time.Second
	}
	return defaultFixtureDeadline
}

func fixtureSequential() bool {
	switch os.Getenv("FIXTURE_SEQUENTIAL") {
	case "1", "true", "TRUE", "yes", "YES":
		return true
	default:
		return false
	}
}

// runWithDeadline runs fn under the per-fixture deadline. On timeout it fails
// the step without claiming the root cause: a timeout can be slow finite
// diagnostic production, a too-small fixture budget, or a real fixed-point
// convergence bug. The worker goroutine may continue running until the test
// binary exits, but its result is ignored. fn may call t.* methods; t.Fatal in
// fn (via FailNow/Goexit) terminates the worker cleanly through the deferred
// done signal.
func runWithDeadline(t *testing.T, suite namedSuite, step string, fn func(t *testing.T)) {
	t.Helper()
	deadline := fixtureDeadlineForSuite(suite)
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
		t.Fatalf("fixture deadline exceeded: %s/%s did not complete within %s (slow finite analysis or possible abstract-interpreter non-convergence; rerun with FIXTURE_DEADLINE_SECONDS=%d to distinguish)", suite.Name, step, deadline, int(deadline.Seconds())*4)
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
	report := newFixtureImpactRecorder()
	t.Cleanup(func() {
		report.finish(t)
	})
	for _, s := range suites {
		s := s
		t.Run(s.Name, func(t *testing.T) {
			if !fixtureSequential() {
				t.Parallel()
			}
			suiteStart := time.Now()
			defer func() {
				report.recordSuite(s.Name, fixtureTestStatus(t), time.Since(suiteStart), s.Suite.Skip)
			}()
			if s.Suite.Skip != "" {
				t.Skip(s.Suite.Skip)
			}
			t.Run("check", func(t *testing.T) {
				stepStart := time.Now()
				defer func() {
					report.recordStep(s.Name, "check", fixtureTestStatus(t), time.Since(stepStart), fixtureCheckSkipReason(s))
				}()
				runWithDeadline(t, s, "check", func(t *testing.T) {
					runCheckPhase(t, s)
				})
			})
			t.Run("run", func(t *testing.T) {
				stepStart := time.Now()
				defer func() {
					report.recordStep(s.Name, "run", fixtureTestStatus(t), time.Since(stepStart), fixtureRunSkipReason(s))
				}()
				runWithDeadline(t, s, "run", func(t *testing.T) {
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

func TestFixtureDeadlineForSuiteUsesManifestBudget(t *testing.T) {
	t.Setenv("FIXTURE_DEADLINE_SECONDS", "")
	suite := namedSuite{Suite: fixtureSuite{DeadlineSeconds: 42}}
	if got := fixtureDeadlineForSuite(suite); got != 42*time.Second {
		t.Fatalf("fixtureDeadlineForSuite = %s, want 42s", got)
	}
}

func TestFixtureDeadlineForSuiteEnvOverridesManifestBudget(t *testing.T) {
	t.Setenv("FIXTURE_DEADLINE_SECONDS", "7")
	suite := namedSuite{Suite: fixtureSuite{DeadlineSeconds: 42}}
	if got := fixtureDeadlineForSuite(suite); got != 7*time.Second {
		t.Fatalf("fixtureDeadlineForSuite = %s, want 7s", got)
	}
}

func TestFixtureSequentialEnv(t *testing.T) {
	t.Setenv("FIXTURE_SEQUENTIAL", "")
	if fixtureSequential() {
		t.Fatal("fixtureSequential should default to false")
	}
	t.Setenv("FIXTURE_SEQUENTIAL", "1")
	if !fixtureSequential() {
		t.Fatal("fixtureSequential should accept 1")
	}
	t.Setenv("FIXTURE_SEQUENTIAL", "yes")
	if !fixtureSequential() {
		t.Fatal("fixtureSequential should accept yes")
	}
	t.Setenv("FIXTURE_SEQUENTIAL", "0")
	if fixtureSequential() {
		t.Fatal("fixtureSequential should reject 0")
	}
}

func fixtureCheckSkipReason(s namedSuite) string {
	if s.Suite.Check == nil {
		return ""
	}
	return s.Suite.Check.Skip
}

func fixtureRunSkipReason(s namedSuite) string {
	if s.Suite.Run == nil {
		return ""
	}
	return s.Suite.Run.Skip
}
