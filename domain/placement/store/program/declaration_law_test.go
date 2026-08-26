package program

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	seal "github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	calldomain "github.com/wippyai/go-lua/domain/call"
	effectdomain "github.com/wippyai/go-lua/domain/effect"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementmember "github.com/wippyai/go-lua/domain/placement/memberdefinition"
	storedomain "github.com/wippyai/go-lua/domain/placement/store"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const (
	testRouteKey     = schema.Key("placement/store/route-key")
	storePackagePath = "github.com/wippyai/go-lua/domain/placement/store"
)

// The structural half of this declaration's laws - its geometry, its identity,
// its agreement with the reducer call shape, and the refusal of every
// malformed edit of a term the cold ABI carries - is emitted from the
// declaration into generated_law_test.go. What stays here is what a
// declaration cannot decide alone: how the seal resolves it against a complete
// catalog, and what the route relation's authored judgments actually are.

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

// The route relation states its construction and authors only the judgments
// inside it. What survives from the authored Build's law is the fence it
// existed for: every symbol the declaration names is a DIRECT function with a
// concrete ABI, so no callback, fallback plan, or owner lookup can be inserted
// by the declaration layer and no adapter can smuggle in Unknown behaviour.
// There are three of them now instead of one, and each answers a strictly
// smaller question.
func TestStorageRelationAuthorsItsThreeRouteJudgments(t *testing.T) {
	// Keep each authored ABI concrete at the seam. The two static owner
	// schemas and the Value-owned candidate lead every one of them; what
	// differs is the last position, which is the thing being judged: one atom
	// of the Value, one row of Placement's own directory, or the whole Value
	// when the question is whether it named a closed list at all.
	var resolveAtom func(placementdomain.Schema, *valuedomain.Schema, valuedomain.StorageTransfer, valuedomain.Atom) (storedomain.Route, bool, bool) = storedomain.ResolveRoute
	var resolveDirectory func(placementdomain.Schema, *valuedomain.Schema, valuedomain.StorageTransfer, heapdomain.Key) (storedomain.Route, bool, bool) = storedomain.ResolveDirectoryRoute
	var beyond func(placementdomain.Schema, *valuedomain.Schema, valuedomain.StorageTransfer, valuedomain.Value) (bool, bool) = storedomain.BeyondAllocations
	if resolveAtom == nil || resolveDirectory == nil || beyond == nil {
		t.Fatal("a Store route judgment is unavailable")
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
	// The authored quartet is gone, and its absence is the statement: a
	// relation carrying both forms would be two answers to what its rows are.
	if derivation.AuthoredDerivation() {
		t.Fatalf("Store route derivation still states an authored construction: %+v", derivation)
	}
	if !derivation.DeclaredDerivation() {
		t.Fatalf("Store route derivation states no construction at all: %+v", derivation)
	}
	if len(derivation.StaticAxes) != 2 || derivation.StaticAxes[0].Key != AxisKey || derivation.StaticAxes[1].Key != valueAxisKey {
		t.Fatalf("Store route static axes = %+v", derivation.StaticAxes)
	}
	// The rows are the atoms of the Value the relation is given, read through
	// Value's own schema, and each is judged by this package's own symbol.
	if len(derivation.Source) != 1 || derivation.Source[0].Axis.Key != valueAxisKey || derivation.Source[0].Name != "Atoms" {
		t.Fatalf("Store route source = %+v, want Value's own atom enumeration", derivation.Source)
	}
	if derivation.Resolve.Name != "ResolveRoute" || derivation.Resolve.PackagePath != storePackagePath {
		t.Fatalf("Store route judgment = %+v", derivation.Resolve)
	}
	if derivation.InlineWidth <= 0 {
		t.Fatalf("Store route set states no inline width, so every answer would allocate: %d", derivation.InlineWidth)
	}
	// A Value that named no closed list of allocations widens to Placement's
	// whole directory, whose rows are Heap keys rather than atoms - so the
	// endpoint states its own judgment for what one of those means.
	widen := derivation.Widen
	if !widen.Declared() {
		t.Fatal("Store route set declares no lattice endpoint, so an opaque Value would answer an exact set")
	}
	if widen.Predicate.Name != "BeyondAllocations" || widen.Predicate.PackagePath != storePackagePath {
		t.Fatalf("Store route endpoint = %+v", widen.Predicate)
	}
	if len(widen.Source) != 1 || widen.Source[0].Axis.Key != AxisKey || widen.Source[0].Name != "AllocationDirectory" {
		t.Fatalf("Store widened source = %+v, want Placement's own coordinate directory", widen.Source)
	}
	if widen.Resolve.Name != "ResolveDirectoryRoute" || widen.Resolve.PackagePath != storePackagePath {
		t.Fatalf("Store widened judgment = %+v", widen.Resolve)
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

// focusedAxis builds one axis of the focused table. Every axis this law
// registers is the same shape - a dense engine factor publishing its own fact
// column under its own semantic role - so the shape is stated once and the
// three things that actually differ per axis are the arguments.
func focusedAxis(key schema.Key, catalog member.Catalog, signature axis.Signature) (*axis.Template[struct{}], bool) {
	return axis.New(axis.Spec[struct{}]{
		Key:         key,
		Storage:     axis.StorageEngine,
		Cardinality: axis.CardinalityDense,
		Lifetime:    axis.LifetimeProcess,
		Mutability:  axis.MutabilityFrozen,
		Concurrency: axis.ConcurrencyShared,
		Frame:       axis.Frame{Outputs: []axis.Output{{Key: key + "/facts", Writer: key}}},
		Catalog:     catalog,
		Signature:   signature,
		Semantic:    "semantic/factor/" + key,
	})
}

func compileStorageProgram(t *testing.T, declaration ruleprogram.Program) (ruleplan.Catalog, schema.SealFailure) {
	t.Helper()
	valueCatalog := valuedomain.AxisMemberCatalog()
	placementCatalog := placementdomain.AxisMemberCatalog()
	valueAxis, ok := focusedAxis("value", valueCatalog,
		axis.Signature{Key: valuedomain.ValueCoordinateCarrier, Fact: valuedomain.ValueFactCarrier})
	if !ok {
		t.Fatal("focused Value axis rejected")
	}
	placementAxis, ok := focusedAxis("placement", placementCatalog,
		axis.Signature{Key: placementdomain.PlacementKeyCarrier, Fact: placementdomain.PlacementFactCarrier})
	if !ok {
		t.Fatal("focused Placement axis rejected")
	}
	// Value and Placement do not close the world. Their member catalogs name
	// the Call, Heap and Effect axes - through the reducer inputs and relation
	// providers the call-result, module-load, runtime-kind and
	// closed-allocation rules read them with - and the reference law resolves
	// every one of those against this table. The transitive closure of what
	// the two declared axes reach is exactly these five, so registering them
	// is what makes this catalog COMPLETE rather than merely focused.
	callAxis, ok := focusedAxis("call", calldomain.AxisMemberCatalog(),
		axis.Signature{Key: calldomain.CallKeyCarrier, Fact: calldomain.CallFactCarrier})
	if !ok {
		t.Fatal("focused Call axis rejected")
	}
	heapAxis, ok := focusedAxis("heap", heapdomain.AxisMemberCatalog(),
		axis.Signature{Key: heapdomain.HeapKeyCarrier, Fact: heapdomain.HeapFactCarrier})
	if !ok {
		t.Fatal("focused Heap axis rejected")
	}
	effectAxis, ok := focusedAxis("effect", effectdomain.AxisMemberCatalog(),
		axis.Signature{Key: effectdomain.EffectKeyCarrier, Fact: effectdomain.EffectFactCarrier})
	if !ok {
		t.Fatal("focused Effect axis rejected")
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
		"factor/call",
		"factor/heap",
		"factor/effect",
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
	denominators := []*denominator.Entry{valueDenominator, placementDenominator}
	for _, key := range []string{"call", "heap", "effect"} {
		universe, universeOK := identity.DeriveContentID("go-lua/store-program/"+key, []byte(key))
		if !universeOK {
			t.Fatalf("%s denominator universe identity unavailable", key)
		}
		entry, entryOK := denominator.Coordinate(schema.Key(key), universe)
		if !entryOK {
			t.Fatalf("%s denominator rejected", key)
		}
		denominators = append(denominators, entry)
	}

	// The value axis's member catalog names issued-Program candidate providers -
	// the closure-proof and subject-liveness occurrence relations the capture,
	// suspension and suspension-evidence rules are addressed through - so the
	// issuance surface has to carry those declarations for the reference law to
	// resolve them. A no-op issuance surface makes this catalog incomplete, not
	// focused.
	issuanceEntries, issuanceOK := programissuance.Entries()
	if !issuanceOK {
		t.Fatal("Program issuance entries unavailable")
	}

	builder := seal.NewBuilder()
	for _, surface := range []seal.Surface{
		structure.NewSurface(roleEntries),
		axis.NewSurface([]*axis.Template[struct{}]{valueAxis, placementAxis, callAxis, heapAxis, effectAxis}),
		schemaissuance.NewSurface(issuanceEntries),
		rule.NewSurface([]*rule.Template{programRule}),
		storageNoopSurface{kind: schema.SurfaceKindDiagnostic},
		storageNoopSurface{kind: schema.SurfaceKindComposite},
		denominator.NewSurface(denominators),
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
