package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

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
}
