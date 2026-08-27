package member

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

// TestRawCatalogRowsHaveNoIdentityUntilTheirOwnerIssuesThem states the
// construction/seal boundary: generated declarations carry no identity, and
// a catalog that already crossed the boundary cannot be admitted as new raw
// construction input or issued a second time.
func TestRawCatalogRowsHaveNoIdentityUntilTheirOwnerIssuesThem(t *testing.T) {
	raw := completeCatalog()
	for index, relation := range raw.Relations {
		if relation.ID().Available() {
			t.Fatalf("raw relation %d already has an identity", index)
		}
	}
	owner := axisRef("axis/owner")
	issued, ok := raw.Issue(owner)
	if !ok {
		t.Fatal("owner refused a complete raw catalog")
	}
	if !issued.Relations[0].ID().Available() || !issued.Projections[0].ID().Available() || !issued.Reducers[0].ID().Available() {
		t.Fatal("owner-issued catalog contains an unavailable member identity")
	}
	for _, authority := range issued.Authorities {
		rawAuthority := carrier.Authority{Carrier: authority.Carrier, Capability: authority.Capability}
		expected, expectedOK := carrier.Issue(owner, rawAuthority)
		if !authority.ID().Available() || !expectedOK || authority.ID() != expected.ID() {
			t.Fatalf("carrier authority %q has the wrong issued identity: %v", authority.Carrier, authority.ID())
		}
	}
	if issued.Authorities[0].ID() == issued.Relations[0].ID() {
		t.Fatal("carrier authority and member row share one issuance domain")
	}
	if _, ok := raw.Issue(schema.EntryReference{Surface: schema.SurfaceKindRule, Key: "foreign"}); ok {
		t.Fatal("foreign surface issued an axis member catalog")
	}
	if _, ok := issued.Issue(owner); ok {
		t.Fatal("already-issued catalog was reissued")
	}
	if _, ok := NewCatalog(issued.Authorities, issued.CarrierRefs, issued.Relations, issued.Projections, issued.Reducers, issued.CarryTransforms); ok {
		t.Fatal("already-issued rows were admitted as raw construction input")
	}
}

// TestIssuedCatalogCloneAndContentPreserveMemberIdentities states that the
// identity is row-owned data, not a lookup recovered from the member key after
// cloning or canonical content emission.
func TestIssuedCatalogCloneAndContentPreserveMemberIdentities(t *testing.T) {
	issued, ok := completeCatalog().Issue(axisRef("axis/owner"))
	if !ok {
		t.Fatal("issue complete catalog")
	}
	clone := issued.Clone()
	for index := range issued.Relations {
		if clone.Relations[index].ID() != issued.Relations[index].ID() {
			t.Fatal("relation clone dropped its identity")
		}
	}
	for index := range issued.Projections {
		if clone.Projections[index].ID() != issued.Projections[index].ID() {
			t.Fatal("projection clone dropped its identity")
		}
	}
	for index := range issued.Reducers {
		if clone.Reducers[index].ID() != issued.Reducers[index].ID() {
			t.Fatal("reducer clone dropped its identity")
		}
	}
	if got, ok := clone.Reducer(issued.Reducers[0].Key); !ok || got.ID() != issued.Reducers[0].ID() {
		t.Fatal("reducer lookup dropped its identity")
	}
	if catalogStream(t, clone) != catalogStream(t, issued) {
		t.Fatal("cloning an issued catalog changed canonical content")
	}
}
