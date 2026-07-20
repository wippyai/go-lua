package lua

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"
)

const fixtureCancellationGrace = time.Second

var fixtureSlotAcquireMu sync.Mutex

// fixtureDeadline returns the explicit developer deadline, when one was
// requested. Normal fixture and oracle execution is mathematically uncapped:
// correctness must come from convergence, never from a wall-clock budget.
func fixtureDeadline() (time.Duration, bool) {
	if v := os.Getenv("FIXTURE_DEADLINE_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second, true
		}
	}
	return 0, false
}

func fixtureSequential() bool {
	switch os.Getenv("FIXTURE_SEQUENTIAL") {
	case "1", "true", "TRUE", "yes", "YES":
		return true
	default:
		return false
	}
}

func fixtureParallelism() int {
	if fixtureSequential() {
		return 1
	}
	if v := os.Getenv("FIXTURE_PARALLELISM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	if n := runtime.GOMAXPROCS(0) / 2; n > 1 {
		if n > 4 {
			return 4
		}
		return n
	}
	return 1
}

func fixtureTimeoutExitsProcess() bool {
	switch os.Getenv("FIXTURE_TIMEOUT_EXIT") {
	case "0", "false", "FALSE", "no", "NO":
		return false
	default:
		return true
	}
}

func failFixtureDeadline(t *testing.T, message string) {
	t.Helper()
	if fixtureTimeoutExitsProcess() {
		fmt.Fprintln(os.Stderr, message)
		fmt.Fprintln(os.Stderr, "fatal: fixture did not stop after cooperative cancellation; exiting as a last-resort backstop")
		os.Exit(2)
	}
	t.Fatal(message)
}

// runWithDeadline runs fn directly unless a developer explicitly requested a
// deadline. The default path has neither a timer nor a cancellation goroutine.
// Under an explicit deadline, check phases receive a context that reaches the
// fixed-point engine. Process exit remains only for a worker that ignores that
// requested cancellation through the grace period.
func runWithDeadline(t *testing.T, suite namedSuite, step string, fn func(context.Context, *testing.T)) {
	t.Helper()
	deadline, bounded := fixtureDeadline()
	if !bounded {
		fn(context.Background(), t)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	done := make(chan any, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- r
				return
			}
			done <- nil
		}()
		fn(ctx, t)
	}()
	select {
	case r := <-done:
		if r != nil {
			// Re-raise the worker's panic on the test goroutine so the
			// stack and goroutine state are attributed correctly.
			panic(r)
		}
	case <-ctx.Done():
		message := fmt.Sprintf("fixture deadline exceeded: %s/%s did not complete within %s; cancellation requested", suite.Name, step, deadline)
		select {
		case r := <-done:
			if r != nil {
				panic(r)
			}
			t.Error(message)
		case <-time.After(fixtureCancellationGrace):
			failFixtureDeadline(t, message)
		}
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
	fixtureSlots := make(chan struct{}, fixtureParallelism())
	report := newFixtureImpactRecorder()
	t.Cleanup(func() {
		report.finish(t)
	})
	runFixtureSuites(t, suites, fixtureSlots, func(t *testing.T, s namedSuite) {
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
			runWithDeadline(t, s, "check", func(ctx context.Context, t *testing.T) {
				runCheckPhaseContext(t, s, ctx)
			})
		})
		t.Run("run", func(t *testing.T) {
			stepStart := time.Now()
			defer func() {
				report.recordStep(s.Name, "run", fixtureTestStatus(t), time.Since(stepStart), fixtureRunSkipReason(s))
			}()
			runWithDeadline(t, s, "run", func(_ context.Context, t *testing.T) {
				runExecPhase(t, s)
			})
		})
	})
}

func runFixtureSuites(t *testing.T, suites []namedSuite, slots chan struct{}, run func(*testing.T, namedSuite)) {
	t.Helper()
	for _, s := range suites {
		s := s
		t.Run(s.Name, func(t *testing.T) {
			if !fixtureSequential() {
				t.Parallel()
			}
			t.Cleanup(acquireFixtureSlots(slots, s))
			run(t, s)
		})
	}
}

func fixtureSlotCount(slots chan struct{}, suite namedSuite) int {
	if suite.Suite.Serial {
		return cap(slots)
	}
	return 1
}

func acquireFixtureSlots(slots chan struct{}, suite namedSuite) func() {
	slotCount := fixtureSlotCount(slots, suite)
	fixtureSlotAcquireMu.Lock()
	for i := 0; i < slotCount; i++ {
		slots <- struct{}{}
	}
	fixtureSlotAcquireMu.Unlock()
	return func() {
		for i := 0; i < slotCount; i++ {
			<-slots
		}
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

func TestFixtureOrder_GoogleMetadataThenOptionalHttpBody(t *testing.T) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}

	var metadata namedSuite
	var optionalBody namedSuite
	for _, s := range suites {
		switch s.Name {
		case "modules/google-client-metadata-regression":
			metadata = s
		case "modules/imported-optional-method-zero-arg-read":
			optionalBody = s
		}
	}

	if metadata.Name == "" || optionalBody.Name == "" {
		t.Fatalf("missing target suites: metadata=%q optionalBody=%q", metadata.Name, optionalBody.Name)
	}

	runCheckPhase(t, metadata)
	runCheckPhase(t, optionalBody)
}

func TestFixtureDeadlineIsUncappedByDefault(t *testing.T) {
	t.Setenv("FIXTURE_DEADLINE_SECONDS", "")
	if got, bounded := fixtureDeadline(); bounded || got != 0 {
		t.Fatalf("fixtureDeadline = (%s, %t), want uncapped", got, bounded)
	}
}

func TestFixtureDeadlineRequiresExplicitEnv(t *testing.T) {
	t.Setenv("FIXTURE_DEADLINE_SECONDS", "7")
	if got, bounded := fixtureDeadline(); !bounded || got != 7*time.Second {
		t.Fatalf("fixtureDeadline = (%s, %t), want (7s, true)", got, bounded)
	}
}

func TestFixtureDeadlineRejectsInvalidEnvWithoutAddingCap(t *testing.T) {
	t.Setenv("FIXTURE_DEADLINE_SECONDS", "invalid")
	if got, bounded := fixtureDeadline(); bounded || got != 0 {
		t.Fatalf("fixtureDeadline = (%s, %t), want uncapped", got, bounded)
	}
}

func TestFixtureParallelismEnvOverride(t *testing.T) {
	t.Setenv("FIXTURE_SEQUENTIAL", "")
	t.Setenv("FIXTURE_PARALLELISM", "3")
	if got := fixtureParallelism(); got != 3 {
		t.Fatalf("fixtureParallelism = %d, want 3", got)
	}
}

func TestFixtureParallelismDefaultIsCapped(t *testing.T) {
	t.Setenv("FIXTURE_SEQUENTIAL", "")
	t.Setenv("FIXTURE_PARALLELISM", "")
	previous := runtime.GOMAXPROCS(32)
	defer runtime.GOMAXPROCS(previous)
	if got := fixtureParallelism(); got != 4 {
		t.Fatalf("fixtureParallelism = %d, want capped default 4", got)
	}
}

func TestSerialFixtureAcquiresAllSlots(t *testing.T) {
	slots := make(chan struct{}, 4)
	if got := fixtureSlotCount(slots, namedSuite{}); got != 1 {
		t.Fatalf("fixtureSlotCount normal = %d, want 1", got)
	}
	if got := fixtureSlotCount(slots, namedSuite{Suite: fixtureSuite{Serial: true}}); got != 4 {
		t.Fatalf("fixtureSlotCount serial = %d, want all slots", got)
	}
}

func TestFixtureSequentialForcesSingleSlot(t *testing.T) {
	t.Setenv("FIXTURE_SEQUENTIAL", "1")
	t.Setenv("FIXTURE_PARALLELISM", "8")
	if got := fixtureParallelism(); got != 1 {
		t.Fatalf("fixtureParallelism = %d, want 1", got)
	}
}

func TestFixtureTimeoutExitsProcessByDefault(t *testing.T) {
	t.Setenv("FIXTURE_TIMEOUT_EXIT", "")
	if !fixtureTimeoutExitsProcess() {
		t.Fatal("fixture timeout should exit the process by default")
	}
}

func TestFixtureTimeoutExitCanBeDisabled(t *testing.T) {
	t.Setenv("FIXTURE_TIMEOUT_EXIT", "0")
	if fixtureTimeoutExitsProcess() {
		t.Fatal("fixture timeout exit should be disabled by FIXTURE_TIMEOUT_EXIT=0")
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
