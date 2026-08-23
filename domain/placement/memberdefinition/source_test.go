package memberdefinition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

func TestStorageDefinitionIsCompleteAndForeignProviderOwned(t *testing.T) {
	source := Storage()
	if !source.Complete() {
		t.Fatal("Placement Store definition is incomplete")
	}
	if source.Axis != schema.Key("placement") || len(source.Relations) != 1 || len(source.Projections) != 3 || len(source.Reducers) != 1 {
		t.Fatalf("unexpected source shape: axis=%q relations=%d projections=%d reducers=%d", source.Axis, len(source.Relations), len(source.Projections), len(source.Reducers))
	}

	relation := source.Relations[0]
	provider := relation.CandidateProvider
	if provider.Axis.Key != schema.Key("value") || provider.Member != schema.Key("value/storage-transfer/candidates") {
		t.Fatalf("route relation provider=%+v, want Value storage-transfer candidates", provider)
	}
	if len(relation.Inputs) != 2 || relation.Inputs[0] != "StorageTransferCarrier" || relation.Inputs[1] != "ValueFactCarrier" {
		t.Fatalf("route inputs=%v, want candidate then exact Value fact", relation.Inputs)
	}
	if relation.CandidateResolver.Available() || relation.CandidateOrdinal.Available() || relation.CandidateAt.Available() || relation.Materialize.Available() {
		t.Fatal("foreign candidate directory was copied into Placement")
	}
	derivation := relation.Derivation
	if derivation.State.Name != "RoutePlan" || derivation.Build.Name != "DeriveRoutes" || derivation.Count.Name != "RouteCount" || derivation.At.Name != "RouteAt" ||
		len(derivation.StaticAxes) != 2 || derivation.StaticAxes[0].Surface != schema.SurfaceKindAxis || derivation.StaticAxes[0].Key != "placement" || derivation.StaticAxes[1].Key != "value" {
		t.Fatalf("Store route derivation=%+v, want explicit placement/value Build/Count/At", derivation)
	}

	wantProjections := []struct {
		name, result, accessor string
		role                   member.Role
	}{
		{"StorageRouteKey", "PlacementKeyCarrier", "Coordinates", member.Key},
		{"StorageRouteTag", "RouteTagCarrier", "Predicate", member.Predicate},
		{"StorageRouteDestination", "PlacementKeyCarrier", "Coordinates", member.Destination},
	}
	for index, want := range wantProjections {
		projection := source.Projections[index]
		if projection.Name != want.name || projection.Result != want.result || projection.Accessor.Name != want.accessor || projection.CandidateProvider != provider {
			t.Fatalf("projection[%d]=%+v, want name=%s result=%s accessor=%s role=%d provider=%+v", index, projection, want.name, want.result, want.accessor, want.role, provider)
		}
		if projection.Role != want.role {
			t.Fatalf("projection[%d] role=%d, want %d", index, projection.Role, want.role)
		}
	}

	reducer := source.Reducers[0]
	if reducer.Candidate != "StorageTransferCarrier" || len(reducer.Inputs) != 2 || len(reducer.Outputs) != 1 || reducer.Implementation.Name != "StorageFold" {
		t.Fatalf("storage reducer=%+v", reducer)
	}
	if reducer.Inputs[0].Axis.Key != schema.Key("value") || reducer.Inputs[0].Carrier != "ValueFactCarrier" || reducer.Inputs[0].Form != member.ReadFormExact || reducer.Inputs[0].Multiplicity != member.MultiplicityOne || reducer.Inputs[0].Tag != "" {
		t.Fatalf("source reducer input=%+v", reducer.Inputs[0])
	}
	if reducer.Inputs[1].Axis.Key != schema.Key("placement") || reducer.Inputs[1].Carrier != "PlacementFactCarrier" || reducer.Inputs[1].Form != member.ReadFormSelected || reducer.Inputs[1].Multiplicity != member.MultiplicityOne || reducer.Inputs[1].Tag != "RouteTagCarrier" {
		t.Fatalf("selected reducer input=%+v", reducer.Inputs[1])
	}
	if reducer.Outputs[0].Axis.Key != schema.Key("placement") || reducer.Outputs[0].Carrier != "PlacementFactCarrier" {
		t.Fatalf("storage reducer output=%+v", reducer.Outputs[0])
	}
}
