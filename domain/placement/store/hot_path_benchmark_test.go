package store

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	"github.com/wippyai/go-lua/domain/placement"
)

var storeBenchmarkPlacement placement.Placement

var (
	storeRouteBenchmarkResult Route
	storeRouteBenchmarkOK     bool
)

// BenchmarkStorageDemand covers the compact neutral-lifetime interpretation
// used before a storage transfer enters the engine rule.
func BenchmarkStorageDemand(b *testing.B) {
	var demand placement.Placement
	var forced bool
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		demand, forced = Demand(Lifetime(i%int(LifetimeUnknown) + 1))
	}
	demand, forced = Demand(LifetimeUnknown)
	if !forced || demand != placement.Unknown {
		b.Fatal("storage demand")
	}
}

// BenchmarkStorageApply measures the monotone placement join without any
// schema/catalog setup, isolating the operation used for each cell write.
func BenchmarkStorageApply(b *testing.B) {
	lifetime := FromProgram(lifecycle.StorageLifetimeGlobal)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storeBenchmarkPlacement = Apply(placement.Stack, lifetime)
	}
	if storeBenchmarkPlacement != placement.SharedHeap {
		b.Fatal("storage placement")
	}
}

// BenchmarkRoutePlanRouteAtTag isolates staged route validation. Route plans
// are emitted in dense Heap order, so lookup must not allocate or rescan the
// route set for each settled row.
func BenchmarkRoutePlanRouteAtTag(b *testing.B) {
	const width = 128
	plan := RoutePlan{routes: make([]Route, width)}
	for index := range plan.routes {
		plan.routes[index].Tag = uint64(index + 1)
	}
	tag := plan.routes[width/2].Tag
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		storeRouteBenchmarkResult, storeRouteBenchmarkOK = plan.routeAtTag(tag)
	}
	if !storeRouteBenchmarkOK || storeRouteBenchmarkResult.Tag != tag {
		b.Fatal("route lookup")
	}
}
