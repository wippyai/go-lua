package memberdefinition_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	definition "github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/domain/memberroster"
)

// storePackagePath is the package whose symbols the Store route derivation
// names. It is spelled here rather than imported, because the law is over what
// the DECLARATION says, and reaching into the judged package for the constant
// would let a rename agree with itself.
const storePackagePath = "github.com/wippyai/go-lua/domain/placement/store"

// composedStorage is Placement's whole member definition: the Store base this
// package declares folded with the Store rule's own reducer contribution. The
// law is stated over the composition rather than over the base, because the
// base deliberately declares no reducer of its own.
func composedStorage(t *testing.T) definition.Definition {
	t.Helper()
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		t.Fatal("member definition roster is not admissible")
	}
	_, composed, composedOK := roster.Definition("placement")
	if !composedOK {
		t.Fatal("Placement member definition does not compose")
	}
	return composed
}

func TestStorageDefinitionIsCompleteAndForeignProviderOwned(t *testing.T) {
	source := composedStorage(t)
	if !source.Complete() {
		t.Fatal("Placement Store definition is incomplete")
	}
	if source.Axis != schema.Key("placement") {
		t.Fatalf("unexpected source axis: %q", source.Axis)
	}

	var relation definition.Relation
	for _, candidate := range source.Relations {
		if candidate.Key == "placement/store/storage-routes" {
			relation = candidate
			break
		}
	}
	if relation.Key == "" {
		t.Fatal("composed Placement definition omitted Store's route relation")
	}
	provider := relation.CandidateProvider
	if provider.AxisRelation.Axis.Key != schema.Key("value") || provider.AxisRelation.Member != schema.Key("value/storage-transfer/candidates") {
		t.Fatalf("route relation provider=%+v, want Value storage-transfer candidates", provider)
	}
	if len(relation.Inputs) != 2 || relation.Inputs[0].Carrier != "StorageTransferCarrier" || relation.Inputs[1].Carrier != "ValueFactCarrier" {
		t.Fatalf("route inputs=%v, want candidate then exact Value fact", relation.Inputs)
	}
	if relation.CandidateResolver.Available() || relation.CandidateOrdinal.Available() || relation.CandidateAt.Available() || relation.Materialize.Available() {
		t.Fatal("foreign candidate directory was copied into Placement")
	}
	// The route relation states its construction and authors only the judgments
	// inside it. The authored quartet is gone, and its absence is the
	// statement: a relation carrying both forms would be two answers to what
	// its rows are.
	derivation := relation.Derivation
	if derivation.AuthoredDerivation() {
		t.Fatalf("Store route derivation still states an authored construction: %+v", derivation)
	}
	if !derivation.DeclaredDerivation() {
		t.Fatalf("Store route derivation states no construction at all: %+v", derivation)
	}
	if len(derivation.StaticAxes) != 2 || derivation.StaticAxes[0].Surface != schema.SurfaceKindAxis || derivation.StaticAxes[0].Key != "placement" || derivation.StaticAxes[1].Key != "value" {
		t.Fatalf("Store route static axes=%+v, want explicit placement then value", derivation.StaticAxes)
	}
	// The rows are the atoms of the Value the relation is given, read through
	// Value's own schema, and each is judged by Store's own symbol.
	if len(derivation.Source) != 1 || derivation.Source[0].Axis.Key != "value" || derivation.Source[0].Name != "Atoms" {
		t.Fatalf("Store route source=%+v, want Value's own atom enumeration", derivation.Source)
	}
	if derivation.Resolve.Name != "ResolveRoute" || derivation.Resolve.PackagePath != storePackagePath {
		t.Fatalf("Store route judgment=%+v", derivation.Resolve)
	}
	// The width the set holds BY VALUE before it spills. A store transfer
	// routes to one allocation, or to a couple where a value carries
	// alternatives, so the ordinary answer never allocates a slice just to be
	// returned. It is pinned rather than merely required positive: the number
	// is the relation's own statement of how many rows it ordinarily answers,
	// and a change to it is a change to that statement.
	if derivation.InlineWidth != 8 {
		t.Fatalf("Store route inline width=%d, want the declared 8", derivation.InlineWidth)
	}
	// A Value that named no closed list of allocations widens to Placement's
	// whole directory, whose rows are Heap keys rather than atoms - so the
	// endpoint states its own judgment for what one of those means.
	widen := derivation.Widen
	if !widen.Declared() || widen.Predicate.Name != "BeyondAllocations" || widen.Resolve.Name != "ResolveDirectoryRoute" ||
		len(widen.Source) != 1 || widen.Source[0].Axis.Key != "placement" || widen.Source[0].Name != "AllocationDirectory" {
		t.Fatalf("Store route widen endpoint=%+v", widen)
	}

	wantProjections := []struct {
		name, result, accessor string
		role                   member.Role
	}{
		{"StorageRouteKey", "PlacementKeyCarrier", "Coordinates", member.Key},
		{"StorageRouteTag", "RouteTagCarrier", "Predicate", member.Predicate},
		{"StorageRouteDestination", "PlacementKeyCarrier", "Coordinates", member.Destination},
	}
	for _, want := range wantProjections {
		var projection definition.Projection
		for _, candidate := range source.Projections {
			if candidate.Name == want.name {
				projection = candidate
				break
			}
		}
		if projection.Name == "" {
			t.Fatalf("composed Placement definition omitted projection %s", want.name)
		}
		if projection.Name != want.name || projection.Result != want.result || projection.Accessor.Name != want.accessor || projection.CandidateProvider != provider {
			t.Fatalf("projection=%+v, want name=%s result=%s accessor=%s role=%d provider=%+v", projection, want.name, want.result, want.accessor, want.role, provider)
		}
		if projection.Role != want.role {
			t.Fatalf("projection %s role=%d, want %d", want.name, projection.Role, want.role)
		}
	}

	var reducer definition.Reducer
	for _, candidate := range source.Reducers {
		if candidate.Key == "placement/store/reducer/storage" {
			reducer = candidate
			break
		}
	}
	if reducer.Key == "" {
		t.Fatal("composed Placement definition omitted Store's reducer")
	}
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
