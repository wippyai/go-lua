package publicationescape

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Invalid route inputs are refusals. In particular, an opaque operation gate
// is not permission to turn a malformed row or an unresolvable source into a
// synthetic all-root Unknown publication.
func TestRouteSetRefusesInvalidInputsBeforeOpaqueWidening(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	coordinate, coordinateOK := fixture.values.CoordinateAt(0)
	if !coordinateOK {
		t.Fatal("Value coordinate")
	}
	rowID := identity.ContentID{41}
	cases := []struct {
		name     string
		prepared *PreparedBatch
		gate     operationGate
		facts    factBuffer
	}{
		{
			name: "missing coordinate",
			prepared: &PreparedBatch{
				rows:    []publicationRow{{id: rowID, requirement: placementdomain.SharedHeap, operation: 1}},
				sources: []sourceSpec{{tag: sourceTag(1), rowID: rowID, operation: 1, coordinate: valuedomain.Coordinate{}}},
			},
			gate: func() operationGate { gate := operationGate{}; gate.opaque = true; return gate }(),
		},
		{
			name: "malformed requirement",
			prepared: &PreparedBatch{
				rows: []publicationRow{{id: rowID, requirement: placementdomain.Bottom, operation: 1}},
			},
			gate: func() operationGate { gate := operationGate{}; gate.opaque = true; return gate }(),
		},
		{
			name: "incomplete value join",
			prepared: &PreparedBatch{
				rows:    []publicationRow{{id: rowID, requirement: placementdomain.SharedHeap, operation: 1}},
				sources: []sourceSpec{{tag: sourceTag(1), rowID: rowID, operation: 1, coordinate: coordinate}},
			},
			gate:  operationGateForTest(1),
			facts: factBuffer{},
		},
		{
			name: "unauthenticated open bit",
			prepared: &PreparedBatch{
				rows: []publicationRow{{id: rowID, requirement: placementdomain.SharedHeap, operation: 1, subjectOpen: true}},
			},
			gate: operationGateForTest(1),
		},
		{
			name: "unauthenticated proven-nil bit",
			prepared: &PreparedBatch{
				rows: []publicationRow{{id: rowID, requirement: placementdomain.SharedHeap, operation: 1, subjectNil: true}},
			},
			gate: operationGateForTest(1),
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			routes, ok := routeSetFor(fixture.placement, fixture.values, test.prepared, test.gate, test.facts)
			if ok || routes.len() != 0 {
				t.Fatalf("invalid route input produced routes=%d ok=%t", routes.len(), ok)
			}
		})
	}
}

func TestRouteSetRefusesForeignPlacementSchemaBeforeBroadcast(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	foreign := newPublicationEscapeFixture(t)
	prepared := &PreparedBatch{
		rows:     []publicationRow{{id: identity.ContentID{45}, requirement: placementdomain.SharedHeap, operation: 1, subjectOpen: true}},
		byTag:    map[sourceTag]sourceSpec{},
		prepared: true,
	}
	routes, ok := routeSetFor(foreign.placement, fixture.values, prepared, operationGateForTest(1), factBuffer{})
	if ok || routes.len() != 0 {
		t.Fatalf("foreign Placement schema produced routes=%d ok=%t", routes.len(), ok)
	}
}

// A present Value fact must be owned by the exact Value schema. A malformed
// fact is rejected at the join boundary rather than being treated as absent
// and allowing a later widening branch to hide it.
func TestFactBufferRejectsMalformedPresentValue(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	if (&factBuffer{}).merge(fixture.values, factEntry{rowID: identity.ContentID{42}, present: true}) {
		t.Fatal("malformed present Value fact was accepted")
	}
}

// Sparse Value observations are accepted only when the typed owner supplied
// its exact Bottom default.  That Bottom is neutral across a heterogeneous
// publication row: it cannot erase an authenticated present member, and a
// malformed zero value cannot masquerade as sparse absence.
func TestFactBufferSparseBottomIsAuthenticatedAndNeutral(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	rowID := identity.ContentID{43}
	bottom := fixture.values.Bottom()
	present := fixture.values.Top()

	var first factBuffer
	if !first.merge(fixture.values, factEntry{rowID: rowID, value: bottom, present: false}) ||
		!first.merge(fixture.values, factEntry{rowID: rowID, value: present, present: true}) {
		t.Fatal("authenticated sparse Bottom did not join with a present member")
	}
	got, gotPresent, found := first.get(rowID)
	if !found || !gotPresent || !fixture.values.Equal(got, present) || !first.valid(fixture.values) {
		t.Fatalf("sparse-first aggregate = found:%t present:%t equal:%t", found, gotPresent, fixture.values.Equal(got, present))
	}

	var second factBuffer
	if !second.merge(fixture.values, factEntry{rowID: rowID, value: present, present: true}) ||
		!second.merge(fixture.values, factEntry{rowID: rowID, value: bottom, present: false}) {
		t.Fatal("sparse Bottom was not neutral after a present member")
	}
	got, gotPresent, found = second.get(rowID)
	if !found || !gotPresent || !fixture.values.Equal(got, present) || !second.valid(fixture.values) {
		t.Fatalf("present-first aggregate = found:%t present:%t equal:%t", found, gotPresent, fixture.values.Equal(got, present))
	}

	var malformed factBuffer
	if malformed.merge(fixture.values, factEntry{rowID: rowID, present: false}) {
		t.Fatal("zero Value was accepted as authenticated sparse Bottom")
	}
}

// A missing admitted source row is not a conservative route.  routeSet must
// refuse the join before any opaque/open widening branch can run.
func TestRouteSetRefusesMissingValueRow(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	rowID := identity.ContentID{44}
	prepared := &PreparedBatch{
		rows:    []publicationRow{{id: rowID, requirement: placementdomain.SharedHeap, operation: 1}},
		sources: []sourceSpec{{tag: sourceTag(1), rowID: rowID, operation: 1}},
	}
	routes, ok := routeSetFor(fixture.placement, fixture.values, prepared, operationGateForTest(1), factBuffer{})
	if ok || routes.len() != 0 {
		t.Fatalf("missing admitted Value row produced routes=%d ok=%t", routes.len(), ok)
	}
}

// A closed heterogeneous subject may use a sparse member only when the Value
// owner authenticated its exact Bottom default. A zero/malformed absent cell
// is still an incomplete join and must refuse.
func TestRouteSetRequiresAuthenticatedSparseBottom(t *testing.T) {
	fixture := newPublicationEscapeFixture(t)
	rowID := identity.ContentID{43}
	var facts factBuffer
	if !facts.set(factEntry{rowID: rowID, value: fixture.values.Bottom(), present: false}) {
		t.Fatal("seed sparse fact")
	}
	prepared := &PreparedBatch{rows: []publicationRow{{id: rowID, requirement: placementdomain.SharedHeap, operation: 1}}}
	routes, ok := routeSetFor(fixture.placement, fixture.values, prepared, operationGateForTest(1), facts)
	if !ok || routes.len() != 0 {
		t.Fatalf("authenticated sparse Bottom was rejected/routes=%d ok=%t", routes.len(), ok)
	}
	facts = factBuffer{}
	if !facts.set(factEntry{rowID: rowID, present: false}) {
		t.Fatal("seed malformed sparse fact")
	}
	routes, ok = routeSetFor(fixture.placement, fixture.values, prepared, operationGateForTest(1), facts)
	if ok || routes.len() != 0 {
		t.Fatalf("malformed sparse fact produced routes=%d ok=%t", routes.len(), ok)
	}
}
