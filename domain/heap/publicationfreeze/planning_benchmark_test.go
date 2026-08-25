package publicationfreeze

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

var publicationFreezePlanningSink int

// BenchmarkExactPublicationFreezePlanning measures the common exact planning
// path up to Heap's schema-owned route set: one selected operation, one
// mounted subject fact, and one exact route. Bind-time preparation and schema
// construction stay outside the timed loop.
func BenchmarkExactPublicationFreezePlanning(b *testing.B) {
	rowID := identity.ContentID{1}
	var prepared preparedCall
	prepared.rows = []freezeRow{{id: rowID, operation: vocabulary.Operation(1), subjectTag: sourceTag(1)}}
	if !prepared.sources.add(sourceSpec{tag: sourceTag(1), rowID: rowID, operation: vocabulary.Operation(1)}) {
		b.Fatal("exact source setup")
	}
	var gate operationGate
	if !gate.add(vocabulary.Operation(1)) {
		b.Fatal("exact operation setup")
	}
	var facts factBuffer
	if !facts.set(factEntry{rowID: rowID, present: true}) {
		b.Fatal("exact fact setup")
	}
	var route route
	if !route.Key.Valid() {
		// The route buffer is still the exact final planning representation; a
		// zero key is intentionally not handed to schema-owned planFor here.
		route.Tag = 1
	}
	b.ReportAllocs()
	b.ResetTimer()
	total := 0
	for index := 0; index < b.N; index++ {
		sources := prepared.sourcesForGate(gate)
		if sources.len() != 1 {
			b.Fatal("exact source projection")
		}
		if _, _, found := facts.get(rowID); !found {
			b.Fatal("exact fact lookup")
		}
		var planned routePlan
		if !planned.Add(route) {
			b.Fatal("exact route plan")
		}
		total += planned.Count()
	}
	publicationFreezePlanningSink = total
}

func TestPublicationFreezeHotBuffersOrdinaryPathAllocationFree(t *testing.T) {
	rowID := identity.ContentID{1}
	if allocations := testing.AllocsPerRun(1000, func() {
		var gate operationGate
		if !gate.add(vocabulary.Operation(1)) || !gate.add(vocabulary.Operation(2)) || !gate.admits(vocabulary.Operation(2)) {
			t.Fatal("operation gate")
		}
		var prepared preparedCall
		if !prepared.sources.add(sourceSpec{tag: 1, rowID: rowID, operation: vocabulary.Operation(1)}) {
			t.Fatal("source buffer")
		}
		sources := prepared.sourcesForGate(gate)
		if sources.len() != 1 {
			t.Fatal("source projection")
		}
		var facts factBuffer
		if !facts.set(factEntry{rowID: rowID, present: true}) {
			t.Fatal("fact buffer")
		}
		if _, present, found := facts.get(rowID); !found || !present {
			t.Fatal("fact lookup")
		}
		var planned routePlan
		if !planned.Add(route{Tag: heapdomain.RawRouteTag(1)}) || planned.Count() != 1 {
			t.Fatal("route plan")
		}
	}); allocations != 0 {
		t.Fatalf("ordinary publication-freeze planning allocations = %v", allocations)
	}
}

func TestPublicationFreezeHotBuffersPreserveOverflow(t *testing.T) {
	var gate operationGate
	for operation := vocabulary.Operation(1); operation <= vocabulary.Operation(inlineOperationCapacity+2); operation++ {
		if !gate.add(operation) {
			t.Fatalf("operation %d", operation)
		}
	}
	if gate.count != inlineOperationCapacity+2 || !gate.admits(vocabulary.Operation(inlineOperationCapacity+2)) {
		t.Fatalf("operation overflow count=%d", gate.count)
	}
	var sources sourceBuffer
	for index := 1; index <= inlineSourceCapacity+2; index++ {
		id := identity.ContentID{byte(index)}
		if !sources.add(sourceSpec{tag: sourceTag(index), rowID: id, operation: vocabulary.Operation(1)}) {
			t.Fatalf("source %d", index)
		}
	}
	if sources.len() != inlineSourceCapacity+2 {
		t.Fatalf("source overflow count=%d", sources.len())
	}
	var facts factBuffer
	for index := 1; index <= inlineFactCapacity+2; index++ {
		if !facts.set(factEntry{rowID: identity.ContentID{byte(index)}, present: true}) {
			t.Fatalf("fact %d", index)
		}
	}
	if facts.count != inlineFactCapacity+2 {
		t.Fatalf("fact overflow count=%d", facts.count)
	}
}
