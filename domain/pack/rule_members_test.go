package pack

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

func TestAxisMemberCatalogOwnsSourceGeometry(t *testing.T) {
	catalog := AxisMemberCatalog()
	if !catalog.Available() {
		t.Fatal("Pack axis member catalog unavailable")
	}
	candidate, candidateOK := catalog.Relation(SourceSeeds)
	destination, destinationOK := catalog.Projection(SourceCoordinate)
	reducer, reducerOK := catalog.Reducer(SourceReducer)
	if !candidateOK || len(candidate.Inputs) != 0 || candidate.Subject != SourceCarrier ||
		!destinationOK || destination.Relation != candidate.Key || destination.Role != member.Destination || destination.Result != RootCarrier ||
		!reducerOK || len(reducer.Inputs) != 0 || len(reducer.Outputs) != 1 || reducer.Outputs[0].Carrier != FactCarrier {
		t.Fatalf("source geometry incomplete: candidate=%+v destination=%+v reducer=%+v", candidate, destination, reducer)
	}
}
