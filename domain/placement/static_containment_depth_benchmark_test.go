package placement

import (
	"strconv"
	"testing"
)

var (
	containmentDepthBenchmarkDepths []uint32
	containmentDepthBenchmarkKnown  []bool
	sharedGraphBenchmarkDeep        []EvidenceState
)

// BenchmarkStaticHeapGraphProjection measures the combined private projection
// used by the heterogeneous query after its one authenticated Heap walk. It
// keeps both solver inputs immutable and makes the cost of deriving the two
// evidence columns visible as one operation.
func BenchmarkStaticHeapGraphProjection(b *testing.B) {
	for _, count := range []int{64, 1024, 16_384} {
		b.Run("chain/"+strconv.Itoa(count), func(b *testing.B) {
			allocationDense := make([]int, count)
			allocationOrdinal := make([]int, count)
			for index := 0; index < count; index++ {
				allocationDense[index] = index
				allocationOrdinal[index] = index
			}
			adjacency := benchmarkContainmentChain(count)
			graph := staticHeapGraph{
				allocationDense:     allocationDense,
				allocationOrdinal:   allocationOrdinal,
				adjacency:           adjacency,
				allocationAdjacency: adjacency,
				deepLocal:           benchmarkDeepFrozenProven(count),
				cellsComplete:       true,
				depthComplete:       true,
			}
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				containmentDepthBenchmarkDepths, containmentDepthBenchmarkKnown = graph.depthStates()
				sharedGraphBenchmarkDeep = graph.deepStates()
			}
		})
	}
}

// BenchmarkStaticHeapGraphProjectionScratchReuse is a paired allocation
// benchmark for the query-local Tarjan workspace. Each iteration creates a
// fresh scratch value, matching one heterogeneous query; only the sequential
// depth and DeepFrozen solves share its backing arrays.
func BenchmarkStaticHeapGraphProjectionScratchReuse(b *testing.B) {
	const count = 16_384
	allocationDense := make([]int, count)
	allocationOrdinal := make([]int, count)
	for index := 0; index < count; index++ {
		allocationDense[index] = index
		allocationOrdinal[index] = index
	}
	adjacency := benchmarkContainmentChain(count)
	graph := staticHeapGraph{
		allocationDense:     allocationDense,
		allocationOrdinal:   allocationOrdinal,
		adjacency:           adjacency,
		allocationAdjacency: adjacency,
		deepLocal:           benchmarkDeepFrozenProven(count),
		cellsComplete:       true,
		depthComplete:       true,
	}
	b.Run("separate", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			containmentDepthBenchmarkDepths, containmentDepthBenchmarkKnown = graph.depthStates()
			sharedGraphBenchmarkDeep = graph.deepStates()
		}
	})
	b.Run("shared", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			var scratch containmentSCCScratch
			containmentDepthBenchmarkDepths, containmentDepthBenchmarkKnown = graph.depthStatesWithScratch(&scratch)
			sharedGraphBenchmarkDeep = graph.deepStatesWithScratch(&scratch)
		}
	})
}

// BenchmarkFiniteContainmentDepths measures the graph solver independently
// of engine-issued OrderedCells. OrderedCells is intentionally unconstructable
// outside the engine, so a full query benchmark belongs at the engine/query
// integration boundary; this benchmark keeps the placement-owned SCC and
// longest-path work visible without introducing a raw-value test seam.
func BenchmarkFiniteContainmentDepths(b *testing.B) {
	for _, count := range []int{64, 1024, 16_384} {
		b.Run("chain/"+strconv.Itoa(count), func(b *testing.B) {
			adjacency := benchmarkContainmentChain(count)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				containmentDepthBenchmarkDepths, containmentDepthBenchmarkKnown = finiteContainmentDepths(adjacency)
			}
		})
	}
	for _, count := range []int{64, 1024, 16_384} {
		b.Run("fanout/"+strconv.Itoa(count), func(b *testing.B) {
			adjacency := benchmarkContainmentFanout(count)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				containmentDepthBenchmarkDepths, containmentDepthBenchmarkKnown = finiteContainmentDepths(adjacency)
			}
		})
	}
}

// BenchmarkStaticContainmentCacheAuthenticatedHit measures the one-entry hit
// authority separately from graph/SCC work. The cache still compares every
// saved Heap value with heap.Equal; the hash is only a fast reject.
func BenchmarkStaticContainmentCacheAuthenticatedHit(b *testing.B) {
	entry := &staticContainmentCacheEntry{schema: Schema{}, hash: 19}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if !entry.matches(Schema{}, 19, nil, nil) {
			b.Fatal("authenticated cache hit")
		}
	}
}

func benchmarkContainmentChain(count int) [][]int {
	adjacency := make([][]int, count)
	for node := 0; node+1 < count; node++ {
		adjacency[node] = []int{node + 1}
	}
	return adjacency
}

func benchmarkContainmentFanout(count int) [][]int {
	const width = 8
	adjacency := make([][]int, count)
	for node := 0; node < count; node++ {
		end := node + width + 1
		if end > count {
			end = count
		}
		for child := node + 1; child < end; child++ {
			adjacency[node] = append(adjacency[node], child)
		}
	}
	return adjacency
}
