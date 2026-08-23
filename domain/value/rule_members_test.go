package value

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

func TestAxisMemberCatalogOwnsStorageTransferGeometry(t *testing.T) {
	catalog := AxisMemberCatalog()
	if !catalog.Available() {
		t.Fatal("Value axis member catalog unavailable")
	}
	candidate, candidateOK := catalog.Relation(StorageTransferCandidates)
	source, sourceOK := catalog.Relation(StorageTransferSources)
	key, keyOK := catalog.Projection(StorageTransferSourceKey)
	target, targetOK := catalog.Projection(StorageTransferTarget)
	reducer, reducerOK := catalog.Reducer(IdentityReducer)
	if !candidateOK || len(candidate.Inputs) != 0 || candidate.Subject != StorageTransferCarrier ||
		!sourceOK || len(source.Inputs) != 1 || source.Inputs[0] != StorageTransferCarrier ||
		!keyOK || key.Relation != source.Key || key.Role != member.Key ||
		!targetOK || target.Relation != candidate.Key || target.Role != member.Destination ||
		!reducerOK || len(reducer.Inputs) != 1 || len(reducer.Outputs) != 1 {
		t.Fatalf("storage-transfer geometry incomplete: candidate=%+v source=%+v key=%+v target=%+v reducer=%+v", candidate, source, key, target, reducer)
	}
}
