package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// TestAxisMemberCatalogOwnsTypeFactTransferGeometry states what this axis
// owns of the transfer geometry and what it does not.
//
// The type facts are Static's rows, so it declares them; which storage
// transfers exist, and in what order, is Value's, so this catalog names that
// directory instead of restating it. A local copy would enumerate one subject
// twice, and a rule joining both axes would index one enumeration with the
// other's ordinal.
// TestAxisMemberCatalogOwnsTypeFactTransferGeometry states the transfer
// geometry this axis declares, and which order it is addressed in.
//
// The type facts are Static's rows. Which storage transfers exist, and in what
// order, is Value's: Static's candidate directory resolves through Value's own
// methods, so the two enumerations are one and the catalog says so. Without
// that statement a rule keyed by Value's candidate would index Static's
// directory with an ordinal nothing declared the two to share.
func TestAxisMemberCatalogOwnsTypeFactTransferGeometry(t *testing.T) {
	catalog := AxisMemberCatalog()
	if !catalog.Available() {
		t.Fatal("Static axis member catalog unavailable")
	}
	candidate, candidateOK := catalog.Relation(TypeFactTransfers)
	source, sourceOK := catalog.Relation(TypeFactSources)
	key, keyOK := catalog.Projection(TypeFactSourceKey)
	reducer, reducerOK := catalog.Reducer(IdentityTypeFactReducer)
	if !candidateOK || len(candidate.Inputs) != 0 || candidate.Subject != StorageTransferCarrier ||
		!sourceOK || len(source.Inputs) != 1 || source.Inputs[0] != StorageTransferCarrier ||
		!keyOK || key.Relation != source.Key || key.Role != member.Key || key.Result != CoordinateCarrier ||
		!reducerOK || len(reducer.Inputs) != 1 || len(reducer.Outputs) != 1 {
		t.Fatalf("type-fact transfer geometry incomplete: candidate=%+v source=%+v key=%+v reducer=%+v", candidate, source, key, reducer)
	}
	if len(candidate.Correspondences) != 1 {
		t.Fatalf("the transfer directory states %d correspondences, want Value's order", len(candidate.Correspondences))
	}
	stated := candidate.Correspondences[0]
	if stated.Axis.Key != "value" || stated.Member != "value/storage-transfer/candidates" {
		t.Fatalf("the transfer directory corresponds to %+v, want Value's storage-transfer order", stated)
	}
}
