package publicationescape

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

var benchmarkPlanningSink int

var benchmarkAllRootPlanningSink int

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

// BenchmarkAllRootPublicationPlanning measures the common identity-unknown
// case.  The open subject keeps the publication requirement known while the
// owner schema supplies every possible allocation root lazily; no allocation
// catalogue or spill slice is built inside the timed route planner.
func BenchmarkAllRootPublicationPlanning(b *testing.B) {
	fixture := newPublicationEscapeFixture(b)
	prepared := &preparedBatch{
		rows:     []publicationRow{{id: identity.ContentID{2}, requirement: placementdomain.SharedHeap, operation: vocabulary.Operation(1), subjectOpen: true}},
		byTag:    map[sourceTag]sourceSpec{},
		prepared: true,
	}
	rule := fixture.rule()
	gate := operationGateForTest(vocabulary.Operation(1))
	b.ReportAllocs()
	b.ResetTimer()
	total := 0
	for index := 0; index < b.N; index++ {
		routes, routesOK := rule.routeSet(fixture.placement, prepared, gate, factBuffer{})
		if !routesOK || !routes.allRoot {
			b.Fatal("all-root route planning rejected fixture subject")
		}
		total += routes.len()
		for routeIndex := 0; routeIndex < routes.len(); routeIndex++ {
			route, routeOK := routes.at(routeIndex)
			if !routeOK || route.required != placementdomain.SharedHeap {
				b.Fatal("all-root route projection rejected fixture root")
			}
			total += int(route.tag & 1)
		}
	}
	benchmarkAllRootPlanningSink = total
}

// BenchmarkAllRootPublicationPlanning1024 exercises the owner-validated
// allocation-prefix fast path at a wide denominator.  The fixture and its
// 1024 allocation roots are built before the timed loop; every iteration still
// performs complete planning and lazy route projection.
func BenchmarkAllRootPublicationPlanning1024(b *testing.B) {
	const wideRootCount = 1024
	fixture := newPublicationEscapeFixtureSource(b, publicationEscapeWideSource(wideRootCount))
	if len(fixture.allocations) < wideRootCount {
		b.Fatalf("wide fixture allocation roots=%d, want at least %d", len(fixture.allocations), wideRootCount)
	}
	prepared := &preparedBatch{
		rows:     []publicationRow{{id: identity.ContentID{4}, requirement: placementdomain.SharedHeap, operation: vocabulary.Operation(1), subjectOpen: true}},
		byTag:    map[sourceTag]sourceSpec{},
		prepared: true,
	}
	rule := fixture.rule()
	gate := operationGateForTest(vocabulary.Operation(1))
	b.ReportAllocs()
	b.ResetTimer()
	total := 0
	for index := 0; index < b.N; index++ {
		routes, routesOK := rule.routeSet(fixture.placement, prepared, gate, factBuffer{})
		if !routesOK || !routes.allRoot || !routes.allRootPrefix || routes.len() != len(fixture.allocations) {
			b.Fatal("wide all-root route planning did not use the validated prefix")
		}
		for routeIndex := 0; routeIndex < routes.len(); routeIndex++ {
			route, routeOK := routes.at(routeIndex)
			if !routeOK || route.key.Kind() != heapdomain.RootAllocation || route.required != placementdomain.SharedHeap {
				b.Fatal("wide all-root route projection rejected fixture root")
			}
			total += int(route.tag & 1)
		}
		total += routes.len()
	}
	benchmarkAllRootPlanningSink = total
}

func publicationEscapeWideSource(rootCount int) string {
	var source strings.Builder
	for index := 0; index < rootCount; index++ {
		fmt.Fprintf(&source, "local allocation%d = {}; ", index)
	}
	source.WriteString("return allocation0")
	return source.String()
}
