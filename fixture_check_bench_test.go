package lua

import (
	"context"
	"testing"
	"time"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
)

var fixtureCheckBenchmarkSuites = []string{
	"semantic/nested-channel-select-union-stress",
	"realworld/transactional-saga-orchestrator-soundness",
	"regression/deadlock-dataflow-node",
	"realworld/advanced-type-system-stress",
	"realworld/plugin-runtime-pipeline-soundness",
	"realworld/wippy-scheduler-create-integration",
}

func BenchmarkFixtureChecks(b *testing.B) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		b.Fatalf("discovering fixtures: %v", err)
	}
	byName := make(map[string]namedSuite, len(suites))
	for _, s := range suites {
		byName[s.Name] = s
	}

	for _, name := range fixtureCheckBenchmarkSuites {
		s, ok := byName[name]
		if !ok {
			b.Fatalf("missing check benchmark fixture %q", name)
		}
		b.Run(name, func(b *testing.B) {
			diags, entryFile := fixtureDiagnostics(s)
			if verdict := judgeAgainstCuratedExpectations(s, diags, entryFile); !verdict.passed {
				b.Fatalf("fixture %s no longer satisfies curated expectations: missing=%v unexpected=%v", name, verdict.missing, verdict.unexpected)
			}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				fixtureDiagnostics(s)
			}
		})
	}
}

// BenchmarkFixtureCancellationOverhead compares the normal fixture path with
// the deadline path's cooperative worklist checks on a stress fixture. Calls
// alternate their order every iteration to avoid rewarding the second
// sub-benchmark with warmed CPU/cache state.
func BenchmarkFixtureCancellationOverhead(b *testing.B) {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		b.Fatalf("discovering fixtures: %v", err)
	}
	var suite namedSuite
	for _, candidate := range suites {
		if candidate.Name == "realworld/advanced-type-system-stress" {
			suite = candidate
			break
		}
	}
	if suite.Name == "" {
		b.Fatal("missing stress fixture realworld/advanced-type-system-stress")
	}

	ctx := context.Background()
	var baseline, cancellable time.Duration
	runBaseline := func() {
		started := time.Now()
		fixtureDiagnostics(suite)
		baseline += time.Since(started)
	}
	runCancellable := func() {
		started := time.Now()
		fixtureDiagnosticsWithOptions(suite, testutil.WithContext(ctx))
		cancellable += time.Since(started)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			runBaseline()
			runCancellable()
		} else {
			runCancellable()
			runBaseline()
		}
	}
	b.StopTimer()
	baselinePerOp := float64(baseline.Nanoseconds()) / float64(b.N)
	cancellablePerOp := float64(cancellable.Nanoseconds()) / float64(b.N)
	b.ReportMetric(baselinePerOp, "baseline-ns/op")
	b.ReportMetric(cancellablePerOp, "cancellable-ns/op")
	b.ReportMetric(100*(cancellablePerOp-baselinePerOp)/baselinePerOp, "cancellation-delta-percent")
}
