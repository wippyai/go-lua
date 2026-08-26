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
	prepared := &PreparedBatch{
		rows:     []publicationRow{{id: contentID(51), requirement: placementdomain.SharedHeap, operation: 1, subjectNil: true}},
		byTag:    map[sourceTag]sourceSpec{},
		prepared: true,
	}
	routes, ok := routeSetFor(fixture.placement, fixture.values, prepared, operationGateForTest(1), factBuffer{})
	if !ok {
		t.Fatal("proven-nil subject refused the route set")
	}
	if routes.len() != 0 {
		t.Fatalf("proven-nil subject planned %d routes, want none", routes.len())
	}
}

// TestRouteSetOpenSubjectBroadcastsKnownRequirement pins the independent
// identity/policy readings. An actual tail may populate the subject formal, so
// every allocation root is possible; the authenticated Send row still requires
// SharedHeap at each root rather than changing the policy to Unknown.
func TestRouteSetOpenSubjectBroadcastsKnownRequirement(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	prepared := &PreparedBatch{
		rows:     []publicationRow{{id: contentID(52), requirement: placementdomain.SharedHeap, operation: 1, subjectOpen: true}},
		byTag:    map[sourceTag]sourceSpec{},
		prepared: true,
	}
	routes, ok := routeSetFor(fixture.placement, fixture.values, prepared, operationGateForTest(1), factBuffer{})
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
		t.Fatalf("open subject routes=%d, allocation roots=%d", routes.len(), allocationCount)
	}
	for index := 0; index < routes.len(); index++ {
		route, routeOK := routes.at(index)
		if !routeOK || route.unknown || route.required != placementdomain.SharedHeap {
			t.Fatalf("open subject route=%#v, want SharedHeap identity broadcast", route)
		}
		placement, applyOK := applyRoute(route, placementdomain.DefaultFact())
		if !applyOK || placement != (placementdomain.Fact{Class: placementdomain.SharedHeap, RetainEscape: placementdomain.EvidenceProven}) {
			t.Fatalf("open subject applied placement=%v/%t, want SharedHeap", placement, applyOK)
		}
	}
}

// TestValidPreparedRoutesRefusesContradictorySubjectShape keeps the two
// zero-member readings disjoint. A subject is either statically absent with no
// tail that can reach it, or reachable by a tail; both bits together describe
// no mounted call and must not reach the route planner.
func TestValidPreparedRoutesRefusesContradictorySubjectShape(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	prepared := &PreparedBatch{
		rows:     []publicationRow{{id: contentID(53), requirement: placementdomain.SharedHeap, operation: 1, subjectNil: true, subjectOpen: true}},
		byTag:    map[sourceTag]sourceSpec{},
		prepared: true,
	}
	if validPreparedRoutes(prepared, fixture.values) {
		t.Fatal("contradictory subject shape passed the route integrity fence")
	}
	routes, ok := routeSetFor(fixture.placement, fixture.values, prepared, operationGateForTest(1), factBuffer{})
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
	prepared := &PreparedBatch{
		rows:    []publicationRow{{id: rowID, requirement: placementdomain.SharedHeap, operation: 1, subjectNil: true}},
		sources: []sourceSpec{{tag: sourceTag(1), rowID: rowID, operation: 1, coordinate: coordinate}},
	}
	if validPreparedRoutes(prepared, fixture.values) {
		t.Fatal("proven-nil row with a Value source passed the route integrity fence")
	}
}

// TestRouteSetEmptyValueListSubjectRoutesNothing pins the third zero-member
// reading Pack publishes. A ValuesVar/AllInputs projection with no member is
// an empty value list: a positive fact about the mounted call, distinct from
// the proven-nil formal and from a tail-fed unknown. An empty value list holds
// no allocation root, so the row plans no route; refusing it would report an
// incomplete join for evidence the parent has already resolved.
func TestRouteSetEmptyValueListSubjectRoutesNothing(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	prepared := &PreparedBatch{
		rows:     []publicationRow{{id: contentID(55), requirement: placementdomain.SharedHeap, operation: 1, subjectEmpty: true}},
		byTag:    map[sourceTag]sourceSpec{},
		prepared: true,
	}
	routes, ok := routeSetFor(fixture.placement, fixture.values, prepared, operationGateForTest(1), factBuffer{})
	if !ok {
		t.Fatal("empty value list subject refused the route set")
	}
	if routes.len() != 0 {
		t.Fatalf("empty value list subject planned %d routes, want none", routes.len())
	}
}

// TestValidPreparedRoutesRefusesEmptySubjectContradictions keeps the three
// zero-member readings disjoint and keeps the empty row's source plane empty.
func TestValidPreparedRoutesRefusesEmptySubjectContradictions(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	for name, row := range map[string]publicationRow{
		"empty-and-nil":  {id: contentID(56), requirement: placementdomain.SharedHeap, operation: 1, subjectEmpty: true, subjectNil: true},
		"empty-and-open": {id: contentID(57), requirement: placementdomain.SharedHeap, operation: 1, subjectEmpty: true, subjectOpen: true},
	} {
		prepared := &PreparedBatch{rows: []publicationRow{row}, byTag: map[sourceTag]sourceSpec{}, prepared: true}
		if validPreparedRoutes(prepared, fixture.values) {
			t.Fatalf("%s subject shape passed the route integrity fence", name)
		}
	}

	coordinate, coordinateOK := fixture.values.CoordinateAt(0)
	if !coordinateOK {
		t.Fatal("Value coordinate")
	}
	rowID := contentID(58)
	prepared := &PreparedBatch{
		rows:    []publicationRow{{id: rowID, requirement: placementdomain.SharedHeap, operation: 1, subjectEmpty: true}},
		sources: []sourceSpec{{tag: sourceTag(1), rowID: rowID, operation: 1, coordinate: coordinate}},
	}
	if validPreparedRoutes(prepared, fixture.values) {
		t.Fatal("empty value list row with a Value source passed the route integrity fence")
	}
}
