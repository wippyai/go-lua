package rootguardeffects

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

var benchmarkConcrete transfer.Result
var benchmarkBoundary Result
var benchmarkExit state.State
var benchmarkSummary summary.Summary

func BenchmarkRootGuardBoundary(b *testing.B) {
	reg := standard.Registry()
	f := newFixture(reg)
	plan, err := Compile(f.graph, f.input, resultRoot)
	if err != nil {
		b.Fatal(err)
	}
	entry := fixtureEntry(reg)
	config := Config{Registry: reg, Resolver: f.resolver, Entry: entry}
	b.Run("concrete-body-solve", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkConcrete = solveFixture(f, reg, entry, nil)
		}
	})
	b.Run("boundary-only-summary", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkBoundary, err = plan.Execute(config, nil)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkExit, benchmarkSummary = benchmarkBoundary.Exit, benchmarkBoundary.Summary
		}
	})
	observe := ObservationSet{f.points[3]: {}, f.graph.Exit(): {}}
	b.Run("sparse-observations", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkBoundary, err = plan.Execute(config, observe)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
