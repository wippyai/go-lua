package capture

import (
	"strconv"
	"testing"
)

var (
	captureRouteBenchmarkResult route
	captureRouteBenchmarkOK     bool
	capturePlanBenchmarkResult  routePlan
	capturePlanBenchmarkOK      bool
)

// BenchmarkCaptureRouteLookup bounds the staged-route checker lookup for
// exact and widened capture plans. Routes are emitted in Heap order, so the
// lookup should remain logarithmic rather than scanning every selected root.
func BenchmarkCaptureRouteLookup(b *testing.B) {
	for _, width := range []int{1, 16, 128, 1024} {
		width := width
		b.Run(strconv.Itoa(width), func(b *testing.B) {
			var plan routePlan
			for index := 0; index < width; index++ {
				if !plan.addRoute(route{tag: routeTag(index + 1)}) {
					b.Fatal("route setup")
				}
			}
			tag := routeTag(width/2 + 1)
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				captureRouteBenchmarkResult, captureRouteBenchmarkOK = routeAtTag(plan, tag)
			}
		})
	}
}

// BenchmarkCaptureExactRoutePlanInline measures the route-plan portion of a
// common capture transfer. Exact source atoms are reduced directly into the
// plan's bounded inline storage; widths up to the inline capacity should not
// allocate. Wider sets exercise the explicit suffix spill.
func BenchmarkCaptureExactRoutePlanInline(b *testing.B) {
	for _, width := range []int{1, 4, captureRouteInlineCapacity, captureRouteInlineCapacity + 1, 32} {
		width := width
		b.Run(strconv.Itoa(width), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				var plan routePlan
				ok := true
				for tag := 1; tag <= width; tag++ {
					ok = ok && plan.addRoute(route{tag: routeTag(tag)})
				}
				capturePlanBenchmarkResult = plan
				capturePlanBenchmarkOK = ok && plan.routeCount() == width
			}
			if !capturePlanBenchmarkOK {
				b.Fatalf("exact route plan width %d", width)
			}
		})
	}
}
