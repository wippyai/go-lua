package placement

import (
	"strconv"
	"testing"
)

var deepFrozenBenchmarkStates []EvidenceState

// BenchmarkFiniteDeepFrozenStates measures the placement-owned graph solver
// without engine-issued cells. The graph is prepared outside the timed region
// so the report covers only finiteDeepFrozenStates, including its result and
// SCC-condensation allocations.
func BenchmarkFiniteDeepFrozenStates(b *testing.B) {
	for _, count := range []int{64, 1024, 16_384} {
		b.Run("dense-chain/"+strconv.Itoa(count), func(b *testing.B) {
			local := benchmarkDeepFrozenProven(count)
			adjacency := benchmarkDeepFrozenDenseChain(count)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				deepFrozenBenchmarkStates = finiteDeepFrozenStates(local, adjacency)
			}
			b.StopTimer()
			assertDeepFrozenBenchmarkProven(b, count)
		})

		b.Run("frozen-cycle/"+strconv.Itoa(count), func(b *testing.B) {
			local := benchmarkDeepFrozenProven(count)
			adjacency := benchmarkDeepFrozenCycle(count)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				deepFrozenBenchmarkStates = finiteDeepFrozenStates(local, adjacency)
			}
			b.StopTimer()
			assertDeepFrozenBenchmarkProven(b, count)
		})

		b.Run("high-fanout/"+strconv.Itoa(count), func(b *testing.B) {
			local := benchmarkDeepFrozenProven(count)
			adjacency := benchmarkDeepFrozenHighFanout(count)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				deepFrozenBenchmarkStates = finiteDeepFrozenStates(local, adjacency)
			}
			b.StopTimer()
			assertDeepFrozenBenchmarkProven(b, count)
		})
	}
}

func benchmarkDeepFrozenProven(count int) []EvidenceState {
	local := make([]EvidenceState, count)
	for index := range local {
		local[index] = EvidenceProven
	}
	return local
}

func benchmarkDeepFrozenDenseChain(count int) [][]int {
	adjacency := make([][]int, count)
	for node := 0; node+1 < count; node++ {
		adjacency[node] = []int{node + 1}
	}
	return adjacency
}

func benchmarkDeepFrozenCycle(count int) [][]int {
	adjacency := make([][]int, count)
	if count == 0 {
		return adjacency
	}
	for node := range adjacency {
		adjacency[node] = []int{(node + 1) % count}
	}
	return adjacency
}

func benchmarkDeepFrozenHighFanout(count int) [][]int {
	adjacency := make([][]int, count)
	if count == 0 {
		return adjacency
	}
	adjacency[0] = make([]int, 0, count-1)
	for child := 1; child < count; child++ {
		adjacency[0] = append(adjacency[0], child)
	}
	return adjacency
}

func assertDeepFrozenBenchmarkProven(b *testing.B, count int) {
	b.Helper()
	if len(deepFrozenBenchmarkStates) != count {
		b.Fatalf("deep-frozen benchmark result length = %d, want %d", len(deepFrozenBenchmarkStates), count)
	}
	for index, state := range deepFrozenBenchmarkStates {
		if state != EvidenceProven {
			b.Fatalf("deep-frozen benchmark state[%d] = %v, want proven", index, state)
		}
	}
}
