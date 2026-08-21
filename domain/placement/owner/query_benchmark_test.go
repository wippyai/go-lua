package owner_test

import (
	"testing"

	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

var (
	placementSummaryFoldBenchmarkResult    placementdomain.PlacementSummaryObservation
	placementSummaryFoldBenchmarkOK        bool
	placementSummaryEncodeBenchmarkPresent bool
	placementSummaryEncodeBenchmarkOK      bool
	placementSummaryEncodeBenchmarkRows    uint64
	placementSummaryEncodeBenchmarkData    []byte
)

// BenchmarkPlacementSummaryFold measures Placement's owned summary reducer
// over the dense Heap vector that the real owner/query fixture binds. Schema,
// row values, and the read callback are all prepared before timing; repeated
// calls exercise the reducer's coordinate validation and lattice join without
// charging fixture construction to the fold.
func BenchmarkPlacementSummaryFold(b *testing.B) {
	schema, values, present := placementSummaryBenchmarkInputs(b)
	observation := placementdomain.BeginPlacementSummary(schema)
	at := func(index int) (placementdomain.Placement, bool, bool) {
		return values[index], present[index], true
	}

	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		placementSummaryFoldBenchmarkResult, placementSummaryFoldBenchmarkOK = placementdomain.AccumulatePlacementSummaryRows(schema, observation, len(values), at)
		if !placementSummaryFoldBenchmarkOK {
			b.Fatal("Placement summary fold")
		}
		observation = placementSummaryFoldBenchmarkResult
	}
}

// BenchmarkPlacementSummaryEncoding measures the Placement-owned publication
// boundary after one real summary fold. The immutable observation is prepared
// before timing; each iteration covers only canonical allocation projection,
// sorting, evidence validation, and wire payload encoding.
func BenchmarkPlacementSummaryEncoding(b *testing.B) {
	schema, values, present := placementSummaryBenchmarkInputs(b)
	observation := placementdomain.BeginPlacementSummary(schema)
	var foldOK bool
	observation, foldOK = placementdomain.AccumulatePlacementSummaryRows(schema, observation, len(values), func(index int) (placementdomain.Placement, bool, bool) {
		return values[index], present[index], true
	})
	if !foldOK || observation.Rows == 0 {
		b.Fatal("Placement summary fixture fold")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		placementSummaryEncodeBenchmarkPresent, placementSummaryEncodeBenchmarkRows, placementSummaryEncodeBenchmarkData, placementSummaryEncodeBenchmarkOK = placementdomain.EncodeSummaryResult(observation)
		if !placementSummaryEncodeBenchmarkPresent || !placementSummaryEncodeBenchmarkOK || placementSummaryEncodeBenchmarkRows != uint64(observation.Rows) || len(placementSummaryEncodeBenchmarkData) == 0 {
			b.Fatal("Placement summary publication encoding")
		}
	}
}

func placementSummaryBenchmarkInputs(b testing.TB) (placementdomain.Schema, []placementdomain.Placement, []bool) {
	b.Helper()
	schema, _ := placementOwnerFixture(b)
	count := schema.DenseKeyCount()
	values := make([]placementdomain.Placement, count)
	present := make([]bool, count)
	allocations := 0
	for index := 0; index < count; index++ {
		key, keyOK := schema.KeyAt(index)
		if !keyOK {
			b.Fatalf("Placement summary key %d", index)
		}
		switch key.Kind() {
		case heapdomain.RootBoot:
			values[index], present[index] = placementdomain.Bottom, true
		case heapdomain.RootAllocation:
			values[index], present[index] = placementdomain.OwnedHeap, true
			allocations++
		default:
			b.Fatalf("Placement summary key %d has unsupported kind %v", index, key.Kind())
		}
	}
	if allocations == 0 {
		b.Fatal("Placement summary fixture has no allocation roots")
	}
	return schema, values, present
}
