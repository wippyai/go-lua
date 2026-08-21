package suspension

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
)

func TestValueAggregateIndexSharesImmutableGeometryAcrossRows(t *testing.T) {
	program, _, want := suspensionCatalogLawProgram(t, true)
	view := suspensionCatalogLawView(t, program)
	row := suspensionCatalogLawSubject(t, suspensionCatalogLawID(t, "indexed-yield"), lifecycle.SubjectLivenessValues, want[0])
	index, indexOK := buildValueAggregateIndex(program)
	if !indexOK {
		t.Fatal("build Values aggregate index")
	}
	first, firstOK := subjectValueIDsIndexed(program, view, row, index)
	second, secondOK := subjectValueIDsIndexed(program, view, row, index)
	fixed := want[:len(want)-1]
	entry := index.entries[want[0]]
	if !firstOK || !secondOK || !entry.open || len(first) != len(fixed) || len(second) != len(fixed) {
		t.Fatalf("indexed Values geometry = %v/%t, %v/%t open=%v; want fixed %v/true", first, firstOK, second, secondOK, entry.open, fixed)
	}
	if &first[0] != &second[0] {
		t.Fatal("indexed Values geometry was copied per liveness row")
	}
	for index := range fixed {
		if first[index] != fixed[index] || second[index] != fixed[index] {
			t.Fatalf("indexed Values geometry[%d] = %v/%v, want %v", index, first[index], second[index], fixed[index])
		}
	}
}

func TestValueAggregateIndexCopiesSharePrivateSourceBacking(t *testing.T) {
	program, _, want := suspensionCatalogLawProgram(t, true)
	index, indexOK := buildValueAggregateIndex(program)
	if !indexOK || index.sourceSets == nil {
		t.Fatal("Values aggregate source cache was not initialized")
	}
	// catalogOperandIndexed receives the small index by value. Two copies
	// therefore model two liveness-row joins; the map backing must remain
	// shared while the source slice itself stays private to the catalog.
	firstIndex, secondIndex := index, index
	sources := []source{{id: want[0], tag: routeTag(1)}}
	firstIndex.sourceSets[want[0]] = aggregateSources{sources: sources, ok: true}
	first, firstOK := firstIndex.sourceSets[want[0]]
	second, secondOK := secondIndex.sourceSets[want[0]]
	if !firstOK || !secondOK || !first.ok || !second.ok || len(first.sources) != 1 || len(second.sources) != 1 {
		t.Fatal("indexed source cache did not survive index copies")
	}
	if &first.sources[0] != &second.sources[0] {
		t.Fatal("indexed source storage was copied per liveness row")
	}
	if first.sources[0].id != want[0] || second.sources[0].id != want[0] || first.sources[0].tag != routeTag(1) || second.sources[0].tag != routeTag(1) {
		t.Fatal("indexed source storage changed while being shared")
	}
}

func BenchmarkValueAggregateIndexLookup(b *testing.B) {
	program, _, want := suspensionCatalogLawProgram(b, true)
	view := suspensionCatalogLawView(b, program)
	row := suspensionCatalogLawSubject(b, suspensionCatalogLawID(b, "indexed-benchmark"), lifecycle.SubjectLivenessValues, want[0])
	index, indexOK := buildValueAggregateIndex(program)
	if !indexOK {
		b.Fatal("build Values aggregate index")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		ids, idsOK := subjectValueIDsIndexed(program, view, row, index)
		if !idsOK || len(ids) != len(want)-1 {
			b.Fatal("indexed Values lookup")
		}
	}
}
