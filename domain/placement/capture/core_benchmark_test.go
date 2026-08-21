package capture

import (
	"strconv"
	"testing"
)

var (
	captureRouteBenchmarkResult route
	captureRouteBenchmarkOK     bool
)

// BenchmarkCaptureRouteLookup bounds the staged-route checker lookup for
// exact and widened capture plans. Routes are emitted in Heap order, so the
// lookup should remain logarithmic rather than scanning every selected root.
func BenchmarkCaptureRouteLookup(b *testing.B) {
	for _, width := range []int{1, 16, 128, 1024} {
		width := width
		b.Run(strconv.Itoa(width), func(b *testing.B) {
			plan := routePlan{routes: make([]route, width)}
			for index := range plan.routes {
				plan.routes[index].tag = routeTag(index + 1)
			}
			tag := plan.routes[width/2].tag
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				captureRouteBenchmarkResult, captureRouteBenchmarkOK = routeAtTag(plan, tag)
			}
		})
	}
}
