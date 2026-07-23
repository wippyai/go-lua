package lua

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
)

const stage6OracleDenominator = 106.5

// Run the recordable stage-6 oracle measurement with:
//
//	PERF_ORACLE=1 go test . -run '^TestNewEngineOracleRemeasure$' -count=1 -v
//
// This intentionally follows TestFullOracle's discovered corpus, 16-fixture
// batches, fixtureParallelism policy, and expectation normalization. Until the
// stage-7 cut wires RuntimeCache into fixture calls, cache counters are
// reported as explicitly unwired rather than inventing a speedup.
func TestNewEngineOracleRemeasure(t *testing.T) {
	if os.Getenv("PERF_ORACLE") != "1" {
		t.Skip("oracle re-measure disabled; run with PERF_ORACLE=1")
	}
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	if len(suites) == 0 {
		t.Fatal("oracle re-measure discovered no fixture suites")
	}

	started := time.Now()
	reporter := newFixtureOracleReporter()
	var measurementsMu sync.Mutex
	measurements := make([]newEnginePerFileMeasurement, 0, len(suites))
	var functionalBodies, formalEquations, applyInstantiations int
	fixtureSlots := make(chan struct{}, fixtureParallelism())
	for batchNumber, first := 0, 0; first < len(suites); batchNumber, first = batchNumber+1, first+fixtureOracleBatchSize {
		last := first + fixtureOracleBatchSize
		if last > len(suites) {
			last = len(suites)
		}
		batch := suites[first:last]
		t.Run(fmt.Sprintf("batch-%04d", batchNumber), func(t *testing.T) {
			runFixtureSuites(t, batch, fixtureSlots, func(t *testing.T, suite namedSuite) {
				fixtureStarted := time.Now()
				stats := &program.Stats{}
				diagnostics, entryFile := fixtureDiagnosticsWithOptions(suite, testutil.WithStats(stats))
				verdict := fullOracleVerdictFromDiagnostics(suite, diagnostics, entryFile)
				reporter.record(verdict, isDeadlockFixtureSuite(suite))
				if !verdict.passed {
					t.Errorf("fixture fails checked-in expectations (%d missing, %d unexpected)", len(verdict.missing), len(verdict.unexpected))
				}
				measurementsMu.Lock()
				measurements = append(measurements, newEnginePerFileMeasurement{name: suite.Name, elapsed: time.Since(fixtureStarted), passed: verdict.passed})
				functionalBodies += stats.FunctionalSummary.LexicalBodies
				formalEquations += stats.FunctionalSummary.FormalEquations
				applyInstantiations += stats.FunctionalSummary.ApplyInstantiations
				measurementsMu.Unlock()
			})
		})
		reporter.logBatch(t, batchNumber, len(batch))
	}
	wall := time.Since(started)
	reporter.finish(t)
	reportNewEnginePerFileDistribution(t, measurements)
	t.Logf("ORACLE_REMEASURE wall=%s denominator=%.1fs speedup=%.3fx fixtures=%d", wall.Round(time.Millisecond), stage6OracleDenominator, stage6OracleDenominator/wall.Seconds(), len(measurements))
	t.Logf("ORACLE_REMEASURE functional_summary bodies=%d formal_equations=%d apply_instantiations=%d", functionalBodies, formalEquations, applyInstantiations)
	t.Log("ORACLE_REMEASURE cache_hits=unwired-pre-cut cache_misses=unwired-pre-cut cache_joins=unwired-pre-cut cache_overflows=unwired-pre-cut unique_projections=unwired-pre-cut")
}
