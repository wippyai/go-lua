package formal

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
)

var (
	formalSelectorRangeResult    formalSelectorRange
	formalRouteBenchmarkResult   route
	formalRouteBenchmarkOK       bool
	formalRouteSealBenchmarkPlan routePlan
	formalRouteSealBenchmarkOK   bool
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
		{name: "wide-fallback", count: 9, inline: false, wantOK: true, wantLength: 9},
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
				t.Fatalf("fallback buffer capacity = %d, want at least %d", cap(observations), test.count)
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
			plan := routePlan{routes: make([]route, width)}
			for index := range plan.routes {
				plan.routes[index].tag = routeTag(uint64(index+1)<<routeTagShift | 1)
			}
			tag := plan.routes[width/2].tag
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				formalRouteBenchmarkResult, formalRouteBenchmarkOK = routeForTag(plan, tag)
			}
		})
	}
}

// BenchmarkFormalRoutePlanSeal measures the hot formal demand reduction at
// the requested widths. The input map is deliberately populated in reverse
// dense order; seal must emit Heap order without sorting the R demanded roots.
// A single large owner-fenced schema keeps fixture construction outside the
// timed region while exposing the linear dense walk for every sub-benchmark.
func BenchmarkFormalRoutePlanSeal(b *testing.B) {
	schema := routePlanFixtureSchema(b, 1024)
	keys := routePlanAllocationKeys(b, schema)
	for _, width := range []int{1, 16, 128, 1024} {
		width := width
		if width > len(keys) {
			b.Fatalf("route-plan fixture roots=%d, want at least %d", len(keys), width)
		}
		b.Run(strconv.Itoa(width), func(b *testing.B) {
			demands := make(map[heap.Key]routeDemand, width)
			for index := width - 1; index >= 0; index-- {
				escape := placement.Retain
				if index%2 != 0 {
					escape = placement.Send
				}
				demands[keys[index]] = routeDemand{escape: escape, unknown: index%7 == 0}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				formalRouteSealBenchmarkPlan, formalRouteSealBenchmarkOK = (&routePlan{}).seal(schema, demands)
			}
			if !formalRouteSealBenchmarkOK || len(formalRouteSealBenchmarkPlan.routes) != width {
				b.Fatalf("sealed route plan = %t/%d, want %d", formalRouteSealBenchmarkOK, len(formalRouteSealBenchmarkPlan.routes), width)
			}
		})
	}
}
