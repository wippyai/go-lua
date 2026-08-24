package program

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	seal "github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementmember "github.com/wippyai/go-lua/domain/placement/memberdefinition"
	storedomain "github.com/wippyai/go-lua/domain/placement/store"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const testRouteKey = schema.Key("placement/store/route-key")

// The structural half of this declaration's laws - its geometry, its identity,
// its agreement with the reducer call shape, and the refusal of every
// malformed edit of a term the cold ABI carries - is emitted from the
// declaration into generated_law_test.go. What stays here is what a
// declaration cannot decide alone: how the seal resolves it against a complete
// catalog, and what the route relation's Build actually is.

// TestStorageProgramSealsThroughACompleteCatalog is the seam Check cannot
// reach. Check is data-local by design, so a candidate provider on the wrong
// owner axis and a destination that names a key projection are both well
// formed to it; only the upward seal, resolving every local key against the
// complete catalog, can refuse them. That resolution is what this law states,
// over the same focused composition the canonical declaration compiles
// through.
func TestStorageProgramSealsThroughACompleteCatalog(t *testing.T) {
	if compiled, failure := compileStorageProgram(t, Storage()); failure.Available() || !compiled.Available() {
		t.Fatalf("sealed storage plan unavailable: catalog=%+v failure=%+v", compiled, failure)
	}

	wrongProvider := Storage()
	wrongProvider.Candidate.AxisRelation.Axis = schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "placement"}
	if problem, valid := wrongProvider.Check(); !valid {
		t.Fatalf("the owner-qualified provider is a seal question, not a data-local one: %+v", problem)
	}
	if _, failure := compileStorageProgram(t, wrongProvider); !failure.Available() {
		t.Fatal("wrong owner-qualified candidate provider was admitted by the seal plan")
	}

	wrongDestination := Storage()
	wrongDestination.Fold.Outputs[0].Destination.Member = testRouteKey
	if problem, valid := wrongDestination.Check(); !valid {
		t.Fatalf("the destination's projection kind is a seal question, not a data-local one: %+v", problem)
	}
	if _, failure := compileStorageProgram(t, wrongDestination); !failure.Available() {
		t.Fatal("route output accepted a key projection as its destination")
	}
}

// The route relation's Build is an authored direct call. Its four arguments
// are the two static owner schemas followed by the candidate and exact Value
// fact; no callback, fallback plan, or owner lookup can be inserted by the
// declaration layer.
func TestStorageRelationAuthorsTheFourFenceDeriveRoutesBuild(t *testing.T) {
	// Keep the authored Build ABI concrete at the seam: two static owner
	// schemas, then the Value-owned candidate and its exact Value fact. The
	// relation metadata below must continue to name this direct function rather
	// than an adapter that can smuggle in fallback or Unknown behavior.
	var authoredBuild func(placementdomain.Schema, *valuedomain.Schema, valuedomain.StorageTransfer, valuedomain.Value) (storedomain.RoutePlan, bool) = storedomain.DeriveRoutes
	if authoredBuild == nil {
		t.Fatal("Store route Build is unavailable")
	}
	source := placementmember.Storage()
	if !source.Complete() {
		t.Fatal("Placement Store member source is incomplete")
	}
	if len(source.Relations) != 1 {
		t.Fatalf("Store relation count = %d, want one dependent route relation", len(source.Relations))
	}
	relation := source.Relations[0]
	if relation.CandidateProvider.AxisRelation.Axis.Key != valueAxisKey || relation.CandidateProvider.AxisRelation.Member != StorageTransferCandidates ||
		len(relation.Inputs) != 2 || relation.Inputs[0].Carrier != "StorageTransferCarrier" || relation.Inputs[1].Carrier != "ValueFactCarrier" {
		t.Fatalf("Store route ownership/input shape = %+v", relation)
	}
	if relation.CandidateResolver.Available() || relation.CandidateOrdinal.Available() || relation.CandidateAt.Available() ||
		relation.CandidateCount.Available() || relation.Materialize.Available() || relation.CandidateIdentityAt.Available() {
		t.Fatal("foreign Value candidate directory leaked into Placement Store")
	}
	derivation := relation.Derivation
	if derivation.State.Name != "RoutePlan" || derivation.Build.Name != "DeriveRoutes" || derivation.Count.Name != "RouteCount" || derivation.At.Name != "RouteAt" ||
		derivation.Build.PackagePath != "github.com/wippyai/go-lua/domain/placement/store" || len(derivation.StaticAxes) != 2 ||
		derivation.StaticAxes[0].Key != AxisKey || derivation.StaticAxes[1].Key != valueAxisKey {
		t.Fatalf("Store route derivation = %+v", derivation)
	}
}

// storageNoopSurface fills an unrelated surface in the focused seal fixture.
// It has no declaration authority; the actual axis/rule/denominator surfaces
// below are the ones under test.
type storageNoopSurface struct{ kind schema.SurfaceKind }

func (surface storageNoopSurface) Kind() schema.SurfaceKind { return surface.kind }
func (storageNoopSurface) Entries() []schema.Entry          { return nil }
func (storageNoopSurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

func compileStorageProgram(t *testing.T, declaration ruleprogram.Program) (ruleplan.Catalog, schema.SealFailure) {
	t.Helper()
	valueCatalog := valuedomain.AxisMemberCatalog()
	placementCatalog := placementdomain.AxisMemberCatalog()
	valueAxis, ok := axis.New(axis.Spec[struct{}]{
		Key:         "value",
		Storage:     axis.StorageEngine,
		Cardinality: axis.CardinalityDense,
		Lifetime:    axis.LifetimeProcess,
		Mutability:  axis.MutabilityFrozen,
		Concurrency: axis.ConcurrencyShared,
		Frame:       axis.Frame{Outputs: []axis.Output{{Key: "value/facts", Writer: "value"}}},
		Catalog:     valueCatalog,
		Signature:   axis.Signature{Key: valuedomain.ValueCoordinateCarrier, Fact: valuedomain.ValueFactCarrier},
		Semantic:    "semantic/factor/value",
	})
	if !ok {
		t.Fatal("focused Value axis rejected")
	}
	placementAxis, ok := axis.New(axis.Spec[struct{}]{
		Key:         "placement",
		Storage:     axis.StorageEngine,
		Cardinality: axis.CardinalityDense,
		Lifetime:    axis.LifetimeProcess,
		Mutability:  axis.MutabilityFrozen,
		Concurrency: axis.ConcurrencyShared,
		Frame:       axis.Frame{Outputs: []axis.Output{{Key: "placement/facts", Writer: "placement"}}},
		Catalog:     placementCatalog,
		Signature:   axis.Signature{Key: placementdomain.PlacementKeyCarrier, Fact: placementdomain.PlacementFactCarrier},
		Semantic:    "semantic/factor/placement",
	})
	if !ok {
		t.Fatal("focused Placement axis rejected")
	}

	programRule, ok := rule.New(rule.Spec{
		Key:      "placement-storage-law",
		Lane:     rule.LaneLink,
		Writes:   "placement",
		Owner:    "placement",
		Semantic: "semantic/rule/placement/storage",
		Roles:    []schema.Key{"semantic/operand/placement/storage"},
		Program:  declaration,
	})
	if !ok {
		t.Fatal("focused Store rule rejected")
	}

	roles := []string{
		"factor/value",
		"factor/placement",
		"rule/placement/storage",
		"operand/placement/storage",
	}
	roleEntries := make([]*structure.Entry, 0, len(roles))
	for index, role := range roles {
		entry, entryOK := structure.New(structure.Spec{
			Key:      vocabulary.RoleKey(role),
			Category: structure.CategorySemanticRole,
			Ordinal:  uint16(index + 1),
			Spelling: role,
			Accepted: true,
		})
		if !entryOK {
			t.Fatalf("semantic role %q rejected", role)
		}
		roleEntries = append(roleEntries, entry)
	}
	// The structure seal is a closed-world surface: every declared category
	// needs at least one entry, even though this focused law only exercises
	// semantic roles.  These neutral entries complete the fixture; they do not
	// participate in the Store declaration or provide any runtime authority.
	for category := structure.CategoryInvalid + 1; category.Available(); category++ {
		if category == structure.CategorySemanticRole {
			continue
		}
		spelling := "storage-law/category/" + strconv.Itoa(int(category))
		entry, entryOK := structure.New(structure.Spec{
			Key:      schema.Key(spelling),
			Category: category,
			Ordinal:  1,
			Spelling: spelling,
			Accepted: true,
		})
		if !entryOK {
			t.Fatalf("focused structure filler %q rejected", spelling)
		}
		roleEntries = append(roleEntries, entry)
	}

	valueUniverse, ok := identity.DeriveContentID("go-lua/store-program/value", []byte("value"))
	if !ok {
		t.Fatal("Value denominator universe identity unavailable")
	}
	placementUniverse, ok := identity.DeriveContentID("go-lua/store-program/placement", []byte("placement"))
	if !ok {
		t.Fatal("Placement denominator universe identity unavailable")
	}
	valueDenominator, ok := denominator.Coordinate("value", valueUniverse)
	if !ok {
		t.Fatal("Value denominator rejected")
	}
	placementDenominator, ok := denominator.Coordinate("placement", placementUniverse)
	if !ok {
		t.Fatal("Placement denominator rejected")
	}

	builder := seal.NewBuilder()
	for _, surface := range []seal.Surface{
		structure.NewSurface(roleEntries),
		axis.NewSurface([]*axis.Template[struct{}]{valueAxis, placementAxis}),
		storageNoopSurface{kind: schema.SurfaceKindIssuance},
		rule.NewSurface([]*rule.Template{programRule}),
		storageNoopSurface{kind: schema.SurfaceKindDiagnostic},
		storageNoopSurface{kind: schema.SurfaceKindComposite},
		denominator.NewSurface([]*denominator.Entry{valueDenominator, placementDenominator}),
		storageNoopSurface{kind: schema.SurfaceKindQuery},
		storageNoopSurface{kind: schema.SurfaceKindObservation},
	} {
		if !builder.Register(surface) {
			t.Fatal("focused surface registration failed")
		}
	}
	table, sealFailure := builder.Seal()
	if sealFailure.Available() || table == nil {
		return ruleplan.Catalog{}, sealFailure
	}
	return ruleplan.Compile(table)
}
