package interproctransformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

var benchmarkSummary summary.Summary
var benchmarkCaller transfer.Result

func BenchmarkRepeatedCallerBindings(b *testing.B) {
	f := newFixture()
	reg := standard.Registry()
	compiled, _ := Compile(CompileRequest{})
	left := typevalue.LiteralString(reg, "left")
	right := typevalue.LiteralString(reg, "right")
	b.Run("exact-context-callee-body-solve", func(b *testing.B) {
		engine := new(exactEngine)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, benchmarkSummary = engine.solve(f, left, right)
		}
		b.ReportMetric(float64(engine.bodySolves), "callee-solves")
	})
	b.Run("guarded-transformer-specialize", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchmarkSummary = compiled.Specialize(reg, left, right)
		}
		b.ReportMetric(0, "callee-solves")
	})
	b.Run("exact-context-resolve-and-apply", func(b *testing.B) {
		engine := new(exactEngine)
		entry := callerEntry(f, left, right)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, sum := engine.solve(f, left, right)
			outcome, err := Lower(sum)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkCaller = ApplyCaller(reg, f.caller, f.facts, f.resolver, f.call, f.callerBranch, entry, outcome)
		}
		b.ReportMetric(float64(engine.bodySolves), "callee-solves")
	})
	b.Run("transformer-specialize-and-apply", func(b *testing.B) {
		entry := callerEntry(f, left, right)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sum := compiled.Specialize(reg, left, right)
			outcome, err := Lower(sum)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkCaller = ApplyCaller(reg, f.caller, f.facts, f.resolver, f.call, f.callerBranch, entry, outcome)
		}
		b.ReportMetric(0, "callee-solves")
	})
}
