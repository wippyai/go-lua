// zzalloc_measure_test.go is the throwaway allocation receipt lane for the
// Arm B allocation-shape landing. It is not a law and must not be kept.
package analysis

import (
	"context"
	"testing"
)

func BenchmarkZZEdgeMatrix(b *testing.B) {
	linked := fixtureLink(b, "semantic/type-engine-edge-matrix")
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		result, status := Analyze(context.Background(), linked)
		if status != AnalyzeComplete || result == nil {
			b.Fatalf("Analyze = %v result=%t", status, result != nil)
		}
	}
}
