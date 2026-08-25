package store

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	"github.com/wippyai/go-lua/domain/placement"
)

var storeBenchmarkPlacement placement.Fact

var (
	storeRouteBenchmarkResult Route
	storeRouteBenchmarkOK     bool
)

// BenchmarkStorageDemand covers the compact neutral-lifetime interpretation
// used before a storage transfer enters the engine rule.
func BenchmarkStorageDemand(b *testing.B) {
	var demand placement.Placement
	var forced bool
	var ok bool
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		demand, forced, ok = Demand(Lifetime(i%int(LifetimeClosure) + 1))
	}
	demand, forced, ok = Demand(LifetimeUnknown)
	if !ok || !forced || demand != placement.Unknown {
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
		var ok bool
		storeBenchmarkPlacement, ok = Apply(placement.DefaultFact(), lifetime)
		if !ok {
			b.Fatal("storage placement input")
		}
	}
	if storeBenchmarkPlacement != (placement.Fact{Class: placement.SharedHeap, RetainEscape: placement.EvidenceProven}) {
		b.Fatal("storage placement")
	}
}

// BenchmarkRouteSetIndexedRead isolates reading one member of a wide route
// set. Members are held in ascending coordinate order, so an ordinal is a
// direct address and reading one must neither allocate nor rescan the set.
func BenchmarkRouteSetIndexedRead(b *testing.B) {
	const width = 128
	var plan derived1Rows
	for index := width; index > 0; index-- {
		var placed bool
		plan, placed = insertDerived1Row(plan, uint32(index), uint64(index), Route{Tag: uint64(index)})
		if !placed {
			b.Fatal("route set setup")
		}
	}
	ordinal := width / 2
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		storeRouteBenchmarkResult, storeRouteBenchmarkOK = derived1At(plan, ordinal)
	}
	if !storeRouteBenchmarkOK || storeRouteBenchmarkResult.Tag != uint64(ordinal+1) {
		b.Fatal("route lookup")
	}
}
