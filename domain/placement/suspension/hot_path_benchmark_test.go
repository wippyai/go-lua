package suspension

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
)

// BenchmarkSuspensionPlacementForState measures the closed mapping from the
// neutral Program liveness axis to its conservative Placement demand.
func BenchmarkSuspensionPlacementForState(b *testing.B) {
	var available bool
	var result placementBenchmarkValue
	states := [...]lifecycle.SubjectLivenessState{
		lifecycle.SubjectLivenessDiesBefore,
		lifecycle.SubjectLivenessLive,
		lifecycle.SubjectLivenessUnknown,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		value, ok := PlacementForState(states[i%len(states)])
		result = placementBenchmarkValue(value)
		available = ok
	}
	if !available || result == 0 {
		b.Fatal("suspension placement")
	}
}

type placementBenchmarkValue uint8
