package placement

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

func TestAxisMemberCatalogOwnsStorageRouteGeometry(t *testing.T) {
	catalog := AxisMemberCatalog()
	if !catalog.Available() {
		t.Fatal("Placement axis member catalog unavailable")
	}
	relation, relationOK := catalog.Relation(StorageRoutes)
	key, keyOK := catalog.Projection(StorageRouteKey)
	tag, tagOK := catalog.Projection(StorageRouteTag)
	destination, destinationOK := catalog.Projection(StorageRouteDestination)
	reducer, reducerOK := catalog.Reducer(StorageReducer)
	provider := member.RelationRef{
		Axis:   schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"},
		Member: "value/storage-transfer/candidates",
	}
	if !relationOK || relation.Subject != StorageRouteCarrier || len(relation.Inputs) != 2 ||
		relation.Inputs[0] != StorageTransferCarrier || relation.Inputs[1] != ValueFactCarrier || relation.CandidateProvider != provider ||
		!keyOK || key.Relation != StorageRoutes || key.Role != member.Key || key.Result != PlacementKeyCarrier || key.CandidateProvider != provider ||
		!tagOK || tag.Relation != StorageRoutes || tag.Role != member.Predicate || tag.Result != RouteTagCarrier || tag.CandidateProvider != provider ||
		!destinationOK || destination.Relation != StorageRoutes || destination.Role != member.Destination || destination.Result != PlacementKeyCarrier || destination.CandidateProvider != provider ||
		!reducerOK || len(reducer.Inputs) != 2 || len(reducer.Outputs) != 1 {
		t.Fatalf("storage route geometry incomplete: relation=%+v key=%+v tag=%+v destination=%+v reducer=%+v", relation, key, tag, destination, reducer)
	}
	if reducer.Inputs[0].Axis.Key != "value" || reducer.Inputs[0].Carrier != ValueFactCarrier || reducer.Inputs[0].Form != member.ReadFormExact ||
		reducer.Inputs[1].Axis.Key != "placement" || reducer.Inputs[1].Carrier != PlacementFactCarrier || reducer.Inputs[1].Form != member.ReadFormSelected || reducer.Inputs[1].Tag != RouteTagCarrier ||
		reducer.Outputs[0].Axis.Key != "placement" || reducer.Outputs[0].Carrier != PlacementFactCarrier {
		t.Fatalf("storage reducer signature incomplete: %+v", reducer)
	}
}
