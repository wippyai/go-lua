package corpus

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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

// DefaultFixtureTimeout bounds one fixture's solve and publication phase. A
// fixture that exceeds it is recorded as a timeout divergence and its process
// is killed; compilation happens before the SolveReady phase boundary and is
// not charged to this budget.
const DefaultFixtureTimeout = 5 * time.Second

// DefaultProcessTimeout is the independent watchdog for one observation
// process, including compilation. It is deliberately larger than the solve
// budget so a cold compiler may finish, but finite so a compiler or probe
// deadlock cannot hold a corpus walk forever.
const DefaultProcessTimeout = 40 * time.Second

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
	// Timeout bounds solving and publication after the observation process emits
	// SolveReady. It does not include compilation.
	Timeout time.Duration
	// ProcessTimeout bounds the complete observation process, including
	// compilation. It is a watchdog, not a convergence budget.
	ProcessTimeout time.Duration
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
	if probe.ProcessTimeout <= 0 {
		probe.ProcessTimeout = DefaultProcessTimeout
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
		ProcessTimeout:    probe.ProcessTimeout.String(),
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
	processCtx, cancelProcess := context.WithTimeout(ctx, probe.ProcessTimeout)
	defer cancelProcess()

	command := exec.CommandContext(processCtx, probe.Binary, fixture)
	command.Dir = probe.WorkingDirectory
	command.Env = append(append(command.Environ(), ProcessEnvironment...), probe.Env...)
	stdoutPipe, err := command.StdoutPipe()
	if err != nil {
		return outcome{fixture: fixture, elapsed: time.Since(started), divergences: []Divergence{{
			Fixture: fixture, Family: Wildcard, Site: Wildcard,
			Class: ClassProbeFailure, Old: "", New: err.Error(),
		}}}
	}
	var stdout, stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return outcome{fixture: fixture, elapsed: time.Since(started), divergences: []Divergence{{
			Fixture: fixture, Family: Wildcard, Site: Wildcard,
			Class: ClassProbeFailure, Old: "", New: err.Error(),
		}}}
	}

	// Reading stdout in its own goroutine lets the driver observe the phase
	// marker while the process is running, without risking a pipe deadlock on a
	// large envelope. The marker is consumed here; ParseText still receives
	// exactly one ordinary envelope and no second answer path is introduced.
	ready := make(chan struct{}, 1)
	readDone := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		scanner.Buffer(make([]byte, 4096), 4<<20)
		seenReady := false
		for scanner.Scan() {
			line := scanner.Text()
			if !seenReady && line == SolveReady {
				seenReady = true
				ready <- struct{}{}
				continue
			}
			_, _ = io.WriteString(&stdout, line+"\n")
		}
		readDone <- scanner.Err()
	}()
	waitDone := make(chan error, 1)
	go func() { waitDone <- command.Wait() }()

	var (
		readySeen     bool
		solveStart    time.Time
		solveTimer    *time.Timer
		solveDeadline <-chan time.Time
		readErr       error
		waitErr       error
		readHeld      bool
		waitHeld      bool
	)
	for !readHeld || !waitHeld {
		select {
		case <-ready:
			if !readySeen {
				readySeen = true
				solveStart = time.Now()
				solveTimer = time.NewTimer(probe.Timeout)
				solveDeadline = solveTimer.C
			}
		case <-solveDeadline:
			cancelProcess()
			for !readHeld || !waitHeld {
				select {
				case readErr = <-readDone:
					readHeld = true
				case waitErr = <-waitDone:
					waitHeld = true
				}
			}
			return wholeOutcome(fixture, time.Since(solveStart), ClassTimeout,
				"solve exceeded "+probe.Timeout.String())
		case readErr = <-readDone:
			readHeld = true
		case waitErr = <-waitDone:
			waitHeld = true
		}
	}
	// A short-lived probe can finish both output and Wait before the scheduler
	// selects the buffered phase event. Consume that event before deciding the
	// marker was absent; otherwise a valid fast envelope becomes a false
	// protocol failure.
	if !readySeen {
		select {
		case <-ready:
			readySeen = true
			solveStart = time.Now()
		default:
		}
	}
	if solveTimer != nil {
		if !solveTimer.Stop() {
			select {
			case <-solveTimer.C:
			default:
			}
		}
	}
	elapsed := time.Duration(0)
	if readySeen {
		elapsed = time.Since(solveStart)
	}
	if errors.Is(processCtx.Err(), context.DeadlineExceeded) {
		return wholeOutcome(fixture, elapsed, ClassProcessTimeout,
			"process exceeded watchdog "+probe.ProcessTimeout.String())
	}
	if readErr != nil {
		return wholeOutcome(fixture, elapsed, ClassProbeFailure, readErr.Error())
	}
	if !readySeen {
		// A solved envelope without the phase marker has no trustworthy start
		// point for the analysis budget. Refuse it instead of silently treating
		// an old/unphased probe as parity.
		return wholeOutcome(fixture, elapsed, ClassProtocol,
			"missing "+SolveReady+" phase marker")
	}

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
	if waitErr != nil {
		return whole(ClassProbeFailure, "", strings.TrimSpace(trimTo(stderr.String(), 2000))+" ("+waitErr.Error()+")")
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

func wholeOutcome(fixture string, elapsed time.Duration, class Class, detail string) outcome {
	return outcome{
		fixture: fixture, elapsed: elapsed,
		divergences: []Divergence{{
			Fixture: fixture, Family: Wildcard, Site: Wildcard,
			Class: class, Old: "", New: detail,
		}},
	}
}

func trimTo(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}
