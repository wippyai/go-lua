package formal

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/placement"
)

var (
	formalSelectorRangeResult    formalSelectorRange
	formalRouteBenchmarkResult   route
	formalRouteBenchmarkOK       bool
	formalDenseSealBenchmarkPlan routePlan
	formalDenseSealBenchmarkOK   bool
	formalPlannerBenchmarkPlan   routePlan
	formalPlannerBenchmarkOK     bool
	formalObservationBufferLen   int
	formalObservationBufferOK    bool
)

func TestFormalObservationBufferSelection(t *testing.T) {
	for _, test := range []struct {
		name       string
		count      int
		inline     bool
		wantOK     bool
		wantLength int
	}{
		{name: "empty-inline", count: 0, inline: true, wantOK: true, wantLength: 0},
		{name: "common-inline", count: 8, inline: true, wantOK: true, wantLength: 8},
		{name: "wide-overflow", count: 9, inline: false, wantOK: true, wantLength: 9},
		{name: "negative-rejected", count: -1, wantOK: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var inline [formalObservationInlineWidth]actualObservation
			observations, ok := formalObservationBuffer(test.count, inline[:])
			if ok != test.wantOK || len(observations) != test.wantLength {
				t.Fatalf("buffer(%d) = len %d/%t, want len %d/%t", test.count, len(observations), ok, test.wantLength, test.wantOK)
			}
			if !ok {
				return
			}
			if test.inline && cap(observations) != formalObservationInlineWidth {
				t.Fatalf("inline buffer capacity = %d, want %d", cap(observations), formalObservationInlineWidth)
			}
			if !test.inline && cap(observations) < test.count {
				t.Fatalf("overflow buffer capacity = %d, want at least %d", cap(observations), test.count)
			}
			if test.count > 0 {
				observations[0].valid = true
				if test.inline && !inline[0].valid {
					t.Fatal("inline buffer did not alias caller-owned storage")
				}
			}
		})
	}
}

// BenchmarkFormalObservationBufferSelection isolates the bounded storage
// decision. It does not require a constructible engine context: the staged
// and frame consumers both use this same caller-owned buffer contract.
func BenchmarkFormalObservationBufferSelection(b *testing.B) {
	for _, width := range []int{1, 8, 9, 32} {
		width := width
		b.Run(strconv.Itoa(width), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				var inline [formalObservationInlineWidth]actualObservation
				observations, ok := formalObservationBuffer(width, inline[:])
				formalObservationBufferLen = len(observations)
				formalObservationBufferOK = ok
			}
		})
	}
}

// BenchmarkFormalSelectorRange measures the solve-time selector
// representation. planFor only needs the interval and flags, so no selected
// index slice is required on this path.
func BenchmarkFormalSelectorRange(b *testing.B) {
	for _, width := range []int{1, 16, 128} {
		width := width
		b.Run(strconv.Itoa(width), func(b *testing.B) {
			spec := vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectSendSuffix, FromParam: 0}
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				formalSelectorRangeResult = resolveFormalSelectorRange(spec, width, true)
			}
		})
	}
}

// BenchmarkFormalRouteLookup bounds the staged-route checker lookup. Sealed
// plans are Heap-ordered, so routeForTag must remain logarithmic as widening
// exposes more allocation roots.
func BenchmarkFormalRouteLookup(b *testing.B) {
	for _, width := range []int{1, 16, 128, 1024, 16384} {
		width := width
		b.Run(strconv.Itoa(width), func(b *testing.B) {
			var plan routePlan
			for index := 0; index < width; index++ {
				if !plan.appendRoute(route{tag: routeTag(uint64(index+1)<<routeTagShift | 1)}) {
					b.Fatalf("append route %d", index)
				}
			}
			selected, selectedOK := plan.routeAt(width / 2)
			if !selectedOK {
				b.Fatal("lookup fixture route")
			}
			tag := selected.tag
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				formalRouteBenchmarkResult, formalRouteBenchmarkOK = routeForTag(plan, tag)
			}
		})
	}
}

// BenchmarkFormalRoutePlanSeal exercises the planner's invocation-local
// dense demand set and authenticated Value/Pack all-root widening. The
// all-root case is included because an open Value or Pack runtime tail must
// not allocate one route per Heap root.
func BenchmarkFormalRoutePlanSeal(b *testing.B) {
	schema := routePlanFixtureSchema(b, 1024)
	keys := routePlanAllocationKeys(b, schema)
	for _, width := range []int{1, 16, 128, 1024} {
		width := width
		if width > len(keys) {
			b.Fatalf("dense route-plan fixture roots=%d, want at least %d", len(keys), width)
		}
		b.Run(strconv.Itoa(width), func(b *testing.B) {
			var demands denseDemandScratch
			for index := width - 1; index >= 0; index-- {
				escape := placement.Retain
				if index%2 != 0 {
					escape = placement.Send
				}
				if !planAddDenseDemand(schema, keys[index], escape, index%7 == 0, &demands) {
					b.Fatalf("dense demand %d", index)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				formalDenseSealBenchmarkPlan, formalDenseSealBenchmarkOK = (&routePlan{}).seal(schema, &demands)
			}
			if !formalDenseSealBenchmarkOK || formalDenseSealBenchmarkPlan.routeCount() != width {
				b.Fatalf("dense sealed route plan = %t/%d, want %d", formalDenseSealBenchmarkOK, formalDenseSealBenchmarkPlan.routeCount(), width)
			}
		})
	}
	{
		var demands denseDemandScratch
		if !addUnknownAllDense(schema, &demands) {
			b.Fatal("dense all-root demand")
		}
		b.Run("all-root", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				formalDenseSealBenchmarkPlan, formalDenseSealBenchmarkOK = (&routePlan{}).seal(schema, &demands)
			}
			if !formalDenseSealBenchmarkOK || formalDenseSealBenchmarkPlan.routeCount() != len(keys) || !formalDenseSealBenchmarkPlan.allUnknown {
				b.Fatalf("dense all-root route plan = %t/%d/%t, want %d/true", formalDenseSealBenchmarkOK, formalDenseSealBenchmarkPlan.routeCount(), formalDenseSealBenchmarkPlan.allUnknown, len(keys))
			}
		})
	}
}

// BenchmarkFormalPlannerOpaqueDispatch covers the authenticated opaque Call
// boundary. The opaque arm has no Target/formal authority, so this benchmark
// measures the allocation-free no-route reduction after owner-fenced
// Call/Pack/Value checks. Fixture sealing is outside the timed region.
func BenchmarkFormalPlannerOpaqueDispatch(b *testing.B) {
	fixture := newOpaqueDispatchLawFixture(b, "formal-planner-benchmark")
	callFact := mustOpenDispatchValue(b, fixture.calls, fixture.key)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		formalPlannerBenchmarkPlan, formalPlannerBenchmarkOK = planFor(
			fixture.packs, fixture.calls, fixture.placement, fixture.values,
			fixture.contract, fixture.mounted, callFact, fixture.observations)
	}
	if !formalPlannerBenchmarkOK || formalPlannerBenchmarkPlan.routeCount() != 0 {
		b.Fatalf("planner result = %t/%d", formalPlannerBenchmarkOK, formalPlannerBenchmarkPlan.routeCount())
	}
}
