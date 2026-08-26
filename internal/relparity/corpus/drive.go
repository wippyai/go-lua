package corpus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

// ClassParity labels a fixture the two engines agreed on. It is a report
// facet only and never appears as a Divergence class.
const ClassParity Class = "parity"

// MaximumWorkers is the concurrency ceiling of a corpus walk. Four observation
// processes is the standing bound for this repository; a plan asking for more
// is clamped rather than honoured.
const MaximumWorkers = 4

// DefaultFixtureTimeout bounds one fixture's observation. A fixture that
// exceeds it is recorded as a timeout divergence and its process is killed;
// the walk never waits on it.
const DefaultFixtureTimeout = 30 * time.Second

// DefaultRetainedDivergences bounds the catalogue to a size a lane reads.
// Every divergence is still counted and classified.
const DefaultRetainedDivergences = 2000

// ProcessEnvironment is the bound every observation process runs under. Both
// values are the standing per-process limits for this repository, applied here
// so a walk cannot be the thing that exhausts the host.
var ProcessEnvironment = []string{"GOMEMLIMIT=2GiB", "GOMAXPROCS=2"}

// Probe is the observation command: one binary that takes one fixture name,
// compiles it once, solves it on both engines and writes one envelope.
type Probe struct {
	// Binary is the built observation command.
	Binary string
	// WorkingDirectory is the checkout both engines read the fixture from.
	WorkingDirectory string
	// Timeout bounds one fixture's observation process.
	Timeout time.Duration
	// Env is appended to every observation process's environment.
	Env []string
}

// Plan is one corpus walk.
type Plan struct {
	Probe               Probe
	Fixtures            []string
	Shard               int
	Shards              int
	Workers             int
	RetainedDivergences int
	// SlowestRetained is how many fixture timings the report carries.
	SlowestRetained int
}

// ErrNoFixtures refuses a walk with nothing to compare, for the same reason
// Enumerate refuses an empty corpus.
var ErrNoFixtures = fmt.Errorf("%w: plan named no fixture", ErrEmptyCorpus)

// outcome is one fixture's whole observation, kept per fixture so the walk can
// run concurrently and still assemble one canonically ordered report.
type outcome struct {
	fixture     string
	divergences []Divergence
	rows        int
	elapsed     time.Duration
	// unreached marks a fixture neither engine was asked about. It is not
	// agreement: counting it as parity would let a corpus that stopped
	// compiling read as a corpus the two engines agree on.
	unreached bool
}

// Run walks the plan's fixtures, observing each in its own bounded process,
// and returns the divergence catalogue.
//
// Fixtures are observed concurrently up to the worker ceiling but catalogued
// in canonical fixture order, so the report is a function of the corpus and
// not of the order the processes happened to finish in.
func Run(ctx context.Context, plan Plan) (Report, error) {
	if len(plan.Fixtures) == 0 {
		return Report{}, ErrNoFixtures
	}
	if plan.Probe.Binary == "" {
		return Report{}, fmt.Errorf("corpus: plan named no observation binary")
	}
	fixtures := append([]string(nil), plan.Fixtures...)
	sort.Strings(fixtures)

	probe := plan.Probe
	if probe.Timeout <= 0 {
		probe.Timeout = DefaultFixtureTimeout
	}
	workers := plan.Workers
	if workers <= 0 || workers > MaximumWorkers {
		workers = MaximumWorkers
	}
	retained := plan.RetainedDivergences
	if retained <= 0 {
		retained = DefaultRetainedDivergences
	}
	slowest := plan.SlowestRetained
	if slowest <= 0 {
		slowest = 10
	}

	started := time.Now()
	outcomes := make([]outcome, len(fixtures))
	next := make(chan int)
	var group sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for index := range next {
				outcomes[index] = observe(ctx, probe, fixtures[index])
			}
		}()
	}
	for index := range fixtures {
		next <- index
	}
	close(next)
	group.Wait()

	report := Report{
		Protocol:          Protocol,
		Probe:             probe.Binary,
		WorkingDirectory:  probe.WorkingDirectory,
		Shard:             plan.Shard,
		Shards:            plan.Shards,
		Workers:           workers,
		FixtureTimeout:    probe.Timeout.String(),
		Fixtures:          fixtures,
		FixtureListDigest: Digest(fixtures),
		FixtureCount:      len(fixtures),
		Classes:           map[Class]int{},
		FixtureClasses:    map[Class]int{},
	}
	rank := 0
	for _, result := range outcomes {
		report.Observations++
		report.RowsCompared += result.rows
		if len(result.divergences) == 0 {
			if result.unreached {
				report.FixturesUnreached++
				report.FixtureClasses[ClassUnreached]++
				continue
			}
			report.FixturesAtParity++
			report.FixtureClasses[ClassParity]++
			continue
		}
		report.FixturesDiverged++
		report.FixtureClasses[result.divergences[0].Class]++
		for _, divergence := range result.divergences {
			divergence.Rank = rank
			rank++
			report.DivergenceCount++
			report.Classes[divergence.Class]++
			if len(report.Divergences) < retained {
				report.Divergences = append(report.Divergences, divergence)
			} else {
				report.Truncated = true
			}
		}
	}
	report.Elapsed = time.Since(started).String()
	report.SlowestFixtures = slowestTimings(outcomes, slowest)
	return report, nil
}

func slowestTimings(outcomes []outcome, keep int) []FixtureTiming {
	timings := make([]FixtureTiming, 0, len(outcomes))
	for _, result := range outcomes {
		timings = append(timings, FixtureTiming{Fixture: result.fixture, Seconds: result.elapsed.Seconds()})
	}
	sort.Slice(timings, func(left, right int) bool {
		if timings[left].Seconds != timings[right].Seconds {
			return timings[left].Seconds > timings[right].Seconds
		}
		return timings[left].Fixture < timings[right].Fixture
	})
	if len(timings) > keep {
		timings = timings[:keep]
	}
	return timings
}

// observe runs one fixture's observation process under the bound and turns
// whatever it produced into catalogued divergences.
//
// Every way the process can fail to answer is a named class. There is no path
// on which a fixture leaves no trace in the catalogue.
func observe(ctx context.Context, probe Probe, fixture string) outcome {
	started := time.Now()
	bounded, cancel := context.WithTimeout(ctx, probe.Timeout)
	defer cancel()

	command := exec.CommandContext(bounded, probe.Binary, fixture)
	command.Dir = probe.WorkingDirectory
	command.Env = append(append(command.Environ(), ProcessEnvironment...), probe.Env...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	elapsed := time.Since(started)

	whole := func(class Class, old, fresh string) outcome {
		return outcome{
			fixture: fixture,
			elapsed: elapsed,
			divergences: []Divergence{{
				Fixture: fixture, Family: Wildcard, Site: Wildcard,
				Class: class, Old: old, New: fresh,
			}},
		}
	}
	if errors.Is(bounded.Err(), context.DeadlineExceeded) {
		return whole(ClassTimeout, "observed within "+probe.Timeout.String(),
			"exhausted the "+probe.Timeout.String()+" bound")
	}
	if err != nil {
		return whole(ClassProbeFailure, "", strings.TrimSpace(trimTo(stderr.String(), 2000))+" ("+err.Error()+")")
	}
	envelope, parseErr := ParseText(stdout.String())
	if parseErr != nil {
		return whole(ClassProtocol, "", parseErr.Error())
	}
	if envelope.Fixture != fixture {
		return whole(ClassProtocol, fixture, envelope.Fixture)
	}
	rows := 0
	unreached := false
	if old, held := envelope.Side(SideOld); held {
		rows = len(old.Rows)
		unreached = old.Status == StatusUncompiled
	}
	return outcome{
		fixture:     fixture,
		elapsed:     elapsed,
		rows:        rows,
		unreached:   unreached,
		divergences: Compare(envelope),
	}
}

func trimTo(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}
