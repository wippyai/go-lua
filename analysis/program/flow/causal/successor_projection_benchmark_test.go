package causal

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// BenchmarkSuccessorPlaneSemanticWalk is the shape of the heaviest published
// consumer of this authority: one pass over the whole combined successor plane
// that keeps only each route's semantic identity. It exists so the cost of a
// sealed projection stays a measured number rather than an argument.
func BenchmarkSuccessorPlaneSemanticWalk(b *testing.B) {
	result := openCausalFixture(b, wideCallSpec(512)).result
	plane := result.Successors()
	total := plane.TotalCount()
	if total == 0 {
		b.Fatal("sealed successor plane is empty")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		multiplicity := make(map[identity.ContentID]int, total)
		for index := 0; index < total; index++ {
			route, ok := plane.TotalAt(index)
			if !ok {
				b.Fatal("sealed route disappeared from the successor plane")
			}
			id, idOK := route.SemanticID()
			if !idOK {
				continue
			}
			multiplicity[id]++
		}
	}
}

// BenchmarkBoundaryArmProjection is the O(1) single-arm reissue. Its cost must
// not scale with the width of the CallBoundary row it points into.
func BenchmarkBoundaryArmProjection(b *testing.B) {
	result := openCausalFixture(b, wideCallSpec(512)).result
	view := result.Boundaries()
	call := result.boundaries.rows[0].Call
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, ok := view.Arm(call, BoundaryThrow); !ok {
			b.Fatal("sealed CallBoundary arm disappeared")
		}
	}
}
