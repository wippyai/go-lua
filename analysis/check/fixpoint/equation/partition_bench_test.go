package equation_test

// This file micro-benchmarks the Stage-3 partition read path identified by
// the edge-matrix allocation profile as the dominant cost: copyFacts (68.7%
// of alloc_space), joinClosure, cloneGuards/guardsKey/canonicalGuards. Those
// helpers are unexported; every path below reaches them only through the
// exported surface (PartitionFromClosuresWithGuards, Partition.Values,
// Partition.AllValues), exactly as a Stage-3 kernel does.
//
// Baseline (median of 5 reps; allocs/op is the primary regression gate,
// sec/op is load-noisy — run instructions in engine/perf_bench_test.go):
//
//	PartitionFromClosuresWithGuards_1k    179.2µs   314.6Ki   2.013k allocs
//	PartitionFromClosuresWithGuards_10k   3.272ms   3.725Mi   20.02k allocs
//	JoinManyClosures_1k (64 closures)     201.2µs   256.2Ki   2.015k allocs
//	JoinManyClosures_10k (64 closures)    3.761ms   3.817Mi   20.02k allocs
//	PartitionValues_1k                    55.25µs   34.96Ki   11 allocs
//	PartitionValues_10k                   651.6µs   344.5Ki   11 allocs
//	PartitionAllValues_1k                 84.36µs   160.0Ki   4 allocs
//	PartitionAllValues_10k                1.220ms   1.500Mi   4 allocs
//
// Campaign tracking: persistent partition views (copyFacts elimination) move
// PartitionValues_*/PartitionAllValues_* from O(n)-copy reads to shared
// lookups; interned fact keys remove the strings.Split in
// resolvedBranchGuards (PartitionValues_* path); interned guard sets remove
// the re-sort/re-alloc in canonicalGuards/guardsKey on every join
// (PartitionFromClosuresWithGuards_*, JoinManyClosures_*).

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

// benchBody returns a distinguishable, deterministic body identity without
// paying for a real sha256 hash on every setup call.
func benchBody(seed byte) equation.BodyID {
	var body equation.BodyID
	body[0] = seed
	body[1] = seed + 1
	return body
}

// benchGuardSets builds count distinct guard combinations against one body,
// mixing single-guard and two-guard sets so canonicalGuards/guardsKey see the
// same shape variety as nested branch narrowing produces.
func benchGuardSets(body equation.BodyID, count int) [][]equation.Guard {
	sets := make([][]equation.Guard, count)
	for i := range sets {
		set := []equation.Guard{
			{Body: body, Encoding: []byte(fmt.Sprintf("front/branch/cond-%d/true", i))},
		}
		if i%3 == 0 {
			set = append(set, equation.Guard{Body: body, Encoding: []byte(fmt.Sprintf("front/branch/outer-%d/false", i/3))})
		}
		sets[i] = set
	}
	return sets
}

// benchFacts builds n published facts. A third carry no guard (an ordinary
// unconditional value publication); the rest cycle through guardSets, so a
// read under one active guard set sees a realistic partial-visibility mix
// instead of an all-or-nothing partition.
func benchFacts(n int, guardSets [][]equation.Guard) []equation.Fact {
	facts := make([]equation.Fact, n)
	for i := range facts {
		var guards []equation.Guard
		if i%3 != 0 {
			guards = guardSets[i%len(guardSets)]
		}
		facts[i] = equation.Fact{
			Key:    fmt.Sprintf("value/local-%05d/type", i),
			Value:  []byte(fmt.Sprintf("record{field-%d:number,tag-%d:string}", i, i%17)),
			Guards: guards,
		}
	}
	return facts
}

// benchClosures splits facts into closureCount published OutputClosures, the
// shape PartitionFromClosuresWithGuards receives from several predecessor
// leaves at a CFG join point.
func benchClosures(facts []equation.Fact, closureCount int) []equation.OutputClosure {
	if closureCount < 1 {
		closureCount = 1
	}
	closures := make([]equation.OutputClosure, closureCount)
	chunk := (len(facts) + closureCount - 1) / closureCount
	for i := range closures {
		start := i * chunk
		if start > len(facts) {
			start = len(facts)
		}
		end := start + chunk
		if end > len(facts) {
			end = len(facts)
		}
		closures[i] = equation.OutputClosure{Values: append([]equation.Fact(nil), facts[start:end]...)}
	}
	return closures
}

func runPartitionFromClosuresBench(b *testing.B, n, guardVariants, closureCount int) {
	b.Helper()
	body := benchBody(7)
	guardSets := benchGuardSets(body, guardVariants)
	facts := benchFacts(n, guardSets)
	closures := benchClosures(facts, closureCount)
	active := guardSets[0]

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		partition, err := equation.PartitionFromClosuresWithGuards(active, closures...)
		if err != nil {
			b.Fatal(err)
		}
		if partition.FactCount() != n {
			b.Fatalf("expected %d closed facts, got %d", n, partition.FactCount())
		}
	}
}

// BenchmarkPartitionFromClosuresWithGuards_1k/_10k measure the join+canon
// path (joinClosure, canonicalGuards, guardsKey, cloneGuards) that runs once
// per CFG join when a cyclic or acyclic snapshot is assembled from several
// predecessor leaves.
func BenchmarkPartitionFromClosuresWithGuards_1k(b *testing.B) {
	runPartitionFromClosuresBench(b, 1000, 12, 8)
}
func BenchmarkPartitionFromClosuresWithGuards_10k(b *testing.B) {
	runPartitionFromClosuresBench(b, 10000, 12, 8)
}

// BenchmarkJoinManyClosures_1k/_10k widen the predecessor fan-in (more,
// smaller closures per fact) to isolate joinClosure's per-closure append and
// sort cost from copyFacts' per-fact cut cost.
func BenchmarkJoinManyClosures_1k(b *testing.B) {
	runPartitionFromClosuresBench(b, 1000, 12, 64)
}
func BenchmarkJoinManyClosures_10k(b *testing.B) {
	runPartitionFromClosuresBench(b, 10000, 12, 64)
}

func runPartitionValuesBench(b *testing.B, n, guardVariants, closureCount int) {
	b.Helper()
	body := benchBody(7)
	guardSets := benchGuardSets(body, guardVariants)
	facts := benchFacts(n, guardSets)
	closures := benchClosures(facts, closureCount)
	active := guardSets[0]
	partition, err := equation.PartitionFromClosuresWithGuards(active, closures...)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		values := partition.Values()
		if len(values) == 0 {
			b.Fatal("expected at least one visible value under the active guard set")
		}
	}
}

// BenchmarkPartitionValues_1k/_10k pin the single hottest call in the
// profile: a kernel reading the same closed partition repeatedly, once per
// Stage-3 equation. Each call re-filters and re-copies the whole fact lane
// through copyFacts and resolvedBranchGuards -- this is the allocation the
// perf campaign's persistent-view work targets directly.
func BenchmarkPartitionValues_1k(b *testing.B)  { runPartitionValuesBench(b, 1000, 12, 8) }
func BenchmarkPartitionValues_10k(b *testing.B) { runPartitionValuesBench(b, 10000, 12, 8) }

func runPartitionAllValuesBench(b *testing.B, n, guardVariants, closureCount int) {
	b.Helper()
	body := benchBody(7)
	guardSets := benchGuardSets(body, guardVariants)
	facts := benchFacts(n, guardSets)
	closures := benchClosures(facts, closureCount)
	partition, err := equation.PartitionFromClosuresWithGuards(guardSets[0], closures...)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		values := partition.AllValues()
		if len(values) != n {
			b.Fatalf("expected %d values, got %d", n, len(values))
		}
	}
}

// BenchmarkPartitionAllValues_1k/_10k isolate copyFacts without guard
// resolution (AllValues passes a nil include filter), separating the raw
// per-fact cut/copy cost from the guard-visibility scan added by Values.
func BenchmarkPartitionAllValues_1k(b *testing.B)  { runPartitionAllValuesBench(b, 1000, 12, 8) }
func BenchmarkPartitionAllValues_10k(b *testing.B) { runPartitionAllValuesBench(b, 10000, 12, 8) }
