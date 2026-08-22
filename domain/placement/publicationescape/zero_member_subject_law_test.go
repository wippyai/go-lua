package publicationescape

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// TestRouteSetProvenNilSubjectRoutesNothing pins the proven-nil reading. Lua
// under-application leaves the descriptor's subject formal nil, so the
// publication carries no allocation root out of the call. The row has no Value
// source by construction, and its absence is knowledge rather than an
// incomplete join: routeSet plans no route and does not refuse.
func TestRouteSetProvenNilSubjectRoutesNothing(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	prepared := &preparedBatch{
		rows:     []publicationRow{{id: contentID(51), requirement: placementdomain.SharedHeap, operation: 1, subjectNil: true}},
		byTag:    map[sourceTag]sourceSpec{},
		prepared: true,
	}
	routes, ok := fixture.rule().routeSet(fixture.placement, prepared, operationGateForTest(1), factBuffer{})
	if !ok {
		t.Fatal("proven-nil subject refused the route set")
	}
	if routes.len() != 0 {
		t.Fatalf("proven-nil subject planned %d routes, want none", routes.len())
	}
}

// TestRouteSetUnknownSubjectWidensEveryAllocationRoot pins the unknown
// reading. An actual tail may populate the subject formal, so the rule may not
// conclude containment for any root: every allocation root widens to Unknown.
func TestRouteSetUnknownSubjectWidensEveryAllocationRoot(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	prepared := &preparedBatch{
		rows:     []publicationRow{{id: contentID(52), requirement: placementdomain.SharedHeap, operation: 1, subjectOpen: true}},
		byTag:    map[sourceTag]sourceSpec{},
		prepared: true,
	}
	routes, ok := fixture.rule().routeSet(fixture.placement, prepared, operationGateForTest(1), factBuffer{})
	if !ok {
		t.Fatal("unknown subject refused the route set")
	}
	allocationCount := 0
	for dense := 0; dense < fixture.placement.DenseKeyCount(); dense++ {
		key, keyOK := fixture.placement.KeyAt(dense)
		if keyOK && key.Kind() == heapdomain.RootAllocation {
			allocationCount++
		}
	}
	if allocationCount == 0 || routes.len() != allocationCount {
		t.Fatalf("unknown subject routes=%d, allocation roots=%d", routes.len(), allocationCount)
	}
	for index := 0; index < routes.len(); index++ {
		route, routeOK := routes.at(index)
		if !routeOK || !route.unknown || route.required != placementdomain.Unknown {
			t.Fatalf("unknown subject route=%#v, want Unknown", route)
		}
		placement, applyOK := applyRoute(route, placementdomain.Bottom)
		if !applyOK || placement != placementdomain.Unknown {
			t.Fatalf("unknown subject applied placement=%v/%t, want Unknown", placement, applyOK)
		}
	}
}

// TestValidPreparedRoutesRefusesContradictorySubjectShape keeps the two
// zero-member readings disjoint. A subject is either statically absent with no
// tail that can reach it, or reachable by a tail; both bits together describe
// no mounted call and must not reach the route planner.
func TestValidPreparedRoutesRefusesContradictorySubjectShape(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	prepared := &preparedBatch{
		rows:     []publicationRow{{id: contentID(53), requirement: placementdomain.SharedHeap, operation: 1, subjectNil: true, subjectOpen: true}},
		byTag:    map[sourceTag]sourceSpec{},
		prepared: true,
	}
	if validPreparedRoutes(prepared, fixture.values) {
		t.Fatal("contradictory subject shape passed the route integrity fence")
	}
	routes, ok := fixture.rule().routeSet(fixture.placement, prepared, operationGateForTest(1), factBuffer{})
	if ok || routes.len() != 0 {
		t.Fatalf("contradictory subject shape produced routes=%d ok=%t", routes.len(), ok)
	}
}

// TestValidPreparedRoutesRefusesProvenNilRowWithSource keeps the row and its
// source plane consistent. A proven-nil subject selects no mounted semantic
// source, so a source naming that row contradicts the row itself.
func TestValidPreparedRoutesRefusesProvenNilRowWithSource(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	coordinate, coordinateOK := fixture.values.CoordinateAt(0)
	if !coordinateOK {
		t.Fatal("Value coordinate")
	}
	rowID := identity.ContentID{54}
	prepared := &preparedBatch{
		rows:    []publicationRow{{id: rowID, requirement: placementdomain.SharedHeap, operation: 1, subjectNil: true}},
		sources: []sourceSpec{{tag: sourceTag(1), rowID: rowID, operation: 1, coordinate: coordinate}},
	}
	if validPreparedRoutes(prepared, fixture.values) {
		t.Fatal("proven-nil row with a Value source passed the route integrity fence")
	}
}
