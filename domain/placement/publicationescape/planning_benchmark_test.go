package publicationescape

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/materialization"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

var benchmarkPlanningSink int

// BenchmarkExactPublicationPlanning measures the bounded planning buffers used
// by the common exact case: one selected Call operation, one fixed subject,
// one Value fact, and one exact allocation route. Setup is deliberately kept
// outside the timed loop; it mirrors the prepared batch cache built by BindHot.
func BenchmarkExactPublicationPlanning(b *testing.B) {
	fixture := newPublicationEscapeFixture(b)
	if len(fixture.allocations) == 0 {
		b.Skip("fixture has no allocation root")
	}
	atom, atomOK := fixture.values.Allocation(fixture.allocations[0], materialization.Recent)
	if !atomOK {
		b.Fatal("Value did not issue exact allocation handle")
	}
	fact, factOK := fixture.values.Singleton(atom)
	if !factOK {
		b.Fatal("allocation handle singleton")
	}
	coordinate, coordinateOK := fixture.values.CoordinateAt(0)
	if !coordinateOK {
		b.Fatal("Value coordinate")
	}
	rowID := identity.ContentID{1}
	prepared := &preparedBatch{
		rows:    []publicationRow{{id: rowID, requirement: placementdomain.SharedHeap, operation: vocabulary.Operation(1)}},
		sources: []sourceSpec{{tag: sourceTag(1), rowID: rowID, operation: vocabulary.Operation(1), coordinate: coordinate}},
	}
	rule := fixture.rule()
	b.ReportAllocs()
	b.ResetTimer()
	total := 0
	for index := 0; index < b.N; index++ {
		var gate operationGate
		gate.add(vocabulary.Operation(1))
		sources := prepared.sourcesForGate(gate)
		var facts factBuffer
		for sourceIndex := 0; sourceIndex < sources.len(); sourceIndex++ {
			source, sourceOK := sources.at(sourceIndex)
			if sourceOK {
				facts.set(factEntry{rowID: source.rowID, value: fact, present: true})
			}
		}
		routes, routesOK := rule.routeSet(fixture.placement, prepared, gate, facts)
		if !routesOK {
			b.Fatal("exact route planning rejected fixture fact")
		}
		total += routes.len()
	}
	benchmarkPlanningSink = total
}
