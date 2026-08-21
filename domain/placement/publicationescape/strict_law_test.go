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
		prepared *preparedBatch
		gate     operationGate
		facts    factBuffer
	}{
		{
			name: "missing coordinate",
			prepared: &preparedBatch{
				rows:    []publicationRow{{id: rowID, requirement: placementdomain.SharedHeap, operation: 1}},
				sources: []sourceSpec{{tag: sourceTag(1), rowID: rowID, operation: 1, coordinate: valuedomain.Coordinate{}}},
			},
			gate: func() operationGate { gate := operationGate{}; gate.opaque = true; return gate }(),
		},
		{
			name: "malformed requirement",
			prepared: &preparedBatch{
				rows: []publicationRow{{id: rowID, requirement: placementdomain.Bottom, operation: 1}},
			},
			gate: func() operationGate { gate := operationGate{}; gate.opaque = true; return gate }(),
		},
		{
			name: "incomplete value join",
			prepared: &preparedBatch{
				rows:    []publicationRow{{id: rowID, requirement: placementdomain.SharedHeap, operation: 1}},
				sources: []sourceSpec{{tag: sourceTag(1), rowID: rowID, operation: 1, coordinate: coordinate}},
			},
			gate:  operationGateForTest(1),
			facts: factBuffer{},
		},
		{
			name: "unauthenticated open bit",
			prepared: &preparedBatch{
				rows: []publicationRow{{id: rowID, requirement: placementdomain.SharedHeap, operation: 1, subjectOpen: true}},
			},
			gate: operationGateForTest(1),
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			routes, ok := (&HotRule{values: fixture.valueOwner}).routeSet(fixture.placement, test.prepared, test.gate, test.facts)
			if ok || routes.len() != 0 {
				t.Fatalf("invalid route input produced routes=%d ok=%t", routes.len(), ok)
			}
		})
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
