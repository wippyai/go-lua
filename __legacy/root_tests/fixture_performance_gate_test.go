package lua

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"
)

// Run the per-file performance gate with:
//
//	PERF_GATE=1 go test . -run '^TestNewEnginePerFilePerformanceGate$' -count=1 -v
//
// The fixture package is already built before a test starts, so this measures
// checking rather than Go compilation. The new engine is still pre-cut: this
// exercises the production fixture path while it reaches the admitted
// compiled/interprocedural components available to it.
const newEnginePerFileLimit = 2 * time.Second

// These are the only temporary exceptions to the per-file law. Keep this list
// explicit and short: adding an exception is an acceptance decision, not a
// way to hide a new regression.
var newEnginePerFileExclusions = map[string]string{
	"realworld/notification-delivery-runtime": "standing freeze/coordinate-closure outlier",
	"soundness/memory-model-guards":           "standing memory-model outlier",
	"realworld/plugin-runtime-pipeline":       "standing plugin-runtime outlier",
}

type newEnginePerFileMeasurement struct {
	name     string
	elapsed  time.Duration
	passed   bool
	excluded string
}

// TestNewEnginePerFilePerformanceGate enforces the stage-6 two-second law.
// Every fixture check gets a fresh 2s cooperative deadline and a fresh module
// graph, which prevents a fast fixture from borrowing another fixture's cache
// or wall-clock budget. It deliberately reports p50, p95, and max even when
// the gate is red so the record remains useful while the three exclusions are
// being burned down.
func TestNewEnginePerFilePerformanceGate(t *testing.T) {
	if os.Getenv("PERF_GATE") != "1" {
		t.Skip("performance gate disabled; run with PERF_GATE=1")
	}
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	if len(suites) == 0 {
		t.Fatal("per-file gate discovered no fixtures")
	}

	measurements := make([]newEnginePerFileMeasurement, 0, len(suites))
	for _, suite := range suites {
		ctx, cancel := context.WithTimeout(context.Background(), newEnginePerFileLimit)
		started := time.Now()
		diagnostics, entryFile := fixtureDiagnosticsWithContext(suite, ctx)
		elapsed := time.Since(started)
		deadlineErr := ctx.Err()
		cancel()

		verdict := fullOracleVerdictFromDiagnostics(suite, diagnostics, entryFile)
		measurement := newEnginePerFileMeasurement{
			name:     suite.Name,
			elapsed:  elapsed,
			passed:   verdict.passed && deadlineErr == nil,
			excluded: newEnginePerFileExclusions[suite.Name],
		}
		measurements = append(measurements, measurement)
		if !measurement.passed && measurement.excluded == "" {
			t.Errorf("per-file gate %s: elapsed=%s deadline=%v missing=%v unexpected=%v", suite.Name, elapsed.Round(time.Millisecond), deadlineErr, verdict.missing, verdict.unexpected)
		}
	}

	reportNewEnginePerFileDistribution(t, measurements)
	for _, measurement := range measurements {
		if measurement.excluded != "" {
			t.Logf("PERF_GATE exclusion fixture=%s elapsed=%s reason=%s", measurement.name, measurement.elapsed.Round(time.Millisecond), measurement.excluded)
			continue
		}
		if measurement.elapsed > newEnginePerFileLimit {
			t.Errorf("per-file law violated: %s took %s, limit=%s", measurement.name, measurement.elapsed.Round(time.Millisecond), newEnginePerFileLimit)
		}
	}
}

func reportNewEnginePerFileDistribution(t testing.TB, measurements []newEnginePerFileMeasurement) {
	t.Helper()
	if len(measurements) == 0 {
		return
	}
	sorted := append([]newEnginePerFileMeasurement(nil), measurements...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].elapsed < sorted[j].elapsed })
	p50 := sorted[percentileIndex(len(sorted), 50)].elapsed
	p95 := sorted[percentileIndex(len(sorted), 95)].elapsed
	max := sorted[len(sorted)-1]
	t.Logf("PERF_GATE distribution fixtures=%d p50=%s p95=%s max=%s fixture=%s limit=%s", len(sorted), p50.Round(time.Millisecond), p95.Round(time.Millisecond), max.elapsed.Round(time.Millisecond), max.name, newEnginePerFileLimit)
	for index := len(sorted) - 1; index >= 0 && index >= len(sorted)-10; index-- {
		measurement := sorted[index]
		t.Logf("PERF_GATE fixture=%s elapsed=%s excluded=%t", measurement.name, measurement.elapsed.Round(time.Millisecond), measurement.excluded != "")
	}
}

func percentileIndex(count, percentile int) int {
	if count <= 1 || percentile <= 0 {
		return 0
	}
	index := (count*percentile + 99) / 100
	if index == 0 {
		return 0
	}
	if index > count {
		return count - 1
	}
	return index - 1
}

func (m newEnginePerFileMeasurement) String() string {
	return fmt.Sprintf("%s=%s", m.name, m.elapsed)
}
