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

const (
	testValueCandidates  = schema.Key("value/storage-transfer/candidates")
	testValueSources     = schema.Key("value/storage-transfer/sources")
	testValueSourceKey   = schema.Key("value/storage-transfer/source-key")
	testRouteRelation    = schema.Key("placement/store/storage-routes")
	testRouteKey         = schema.Key("placement/store/route-key")
	testRouteTag         = schema.Key("placement/store/route-tag")
	testRouteDestination = schema.Key("placement/store/route-destination")
)

func TestStorageProgramSealsExactSelectedRouteWithoutCarry(t *testing.T) {
	declaration := Storage()
	if problem, valid := declaration.Check(); !valid {
		t.Fatalf("storage declaration rejected: %+v", problem)
	}
	reducer, reducerOK := placementdomain.AxisMemberCatalog().Reducer(placementdomain.StorageReducer)
	if !reducerOK {
		t.Fatal("generated Placement Store reducer unavailable")
	}
	if problem, valid := declaration.CheckAgainst(reducer); !valid {
		t.Fatalf("storage declaration does not agree with its generated reducer call shape: %+v", problem)
	}
	if compiled, failure := compileStorageProgram(t, declaration); failure.Available() || !compiled.Available() {
		t.Fatalf("sealed storage plan unavailable: catalog=%+v failure=%+v", compiled, failure)
	}
	if declaration.Candidate.AxisRelation.Member != testValueCandidates || declaration.Candidate.AxisRelation.Axis.Key != "value" {
		t.Fatalf("candidate provider=%+v, want Value storage-transfer candidates", declaration.Candidate)
	}
	if got, want := declaration.JoinCount(), 2; got != want {
		t.Fatalf("join count=%d, want %d", got, want)
	}

	first, firstOK := declaration.JoinAt(0)
	if !firstOK || first.Read.Form != ruleprogram.Exact || first.Read.Axis.EntryReference().Key != "value" ||
		first.Relation.Member != testValueSources || first.Key.Member != testValueSourceKey ||
		len(first.Sources) != 1 || !first.Sources[0].Candidate {
		t.Fatalf("exact Value join=%+v, want candidate-only exact source read", first)
	}
	if first.Read.Contract.DenominatorRef.Declared() {
		t.Fatal("exact sparse Value read acquired a fabricated denominator")
	}

	second, secondOK := declaration.JoinAt(1)
	if !secondOK || second.Read.Form != ruleprogram.Selected || second.Read.Axis.EntryReference().Key != "placement" ||
		second.Relation.Member != testRouteRelation || second.Key.Member != testRouteKey ||
		second.Predicate.Member != testRouteTag || len(second.Sources) != 2 ||
		!second.Sources[0].Candidate || second.Sources[1] != ruleprogram.PriorSource(0) {
		t.Fatalf("selected Placement join=%+v, want candidate + prior exact source", second)
	}
	if second.Read.Contract.DenominatorRef.EntryReference().Key != schema.Key("coordinates/placement") {
		t.Fatalf("selected route denominator=%+v, want coordinates/placement", second.Read.Contract.DenominatorRef)
	}

	if declaration.Carry != nil {
		t.Fatalf("carry=%+v, want no carry on a routed Store rule", declaration.Carry)
	}
	if len(declaration.Fold.Inputs) != 2 || declaration.Fold.Inputs[0] != 0 || declaration.Fold.Inputs[1] != 1 {
		t.Fatalf("fold inputs=%v, want [0 1]", declaration.Fold.Inputs)
	}
	output := declaration.Fold.Outputs[0]
	if output.Mode != ruleprogram.ModeRoute || !output.RouteJoinPresent || output.RouteJoin != 1 ||
		output.Destination.Member != testRouteDestination || output.Column.Key != OutputKey {
		t.Fatalf("route output=%+v, want explicit selected-join route publication", output)
	}
}

func TestStorageRuleEntryCarriesTheCanonicalProgramAndIssuance(t *testing.T) {
	spec := RuleEntry()
	if spec.Key != RuleKey || spec.Writes != AxisKey || spec.Owner != AxisKey || spec.Lane != rule.LaneMounted ||
		spec.Semantic != vocabulary.RoleKey(RuleRole) || len(spec.Roles) != 1 || spec.Roles[0] != vocabulary.RoleKey(OperandRole) {
		t.Fatalf("Store RuleEntry identity = %+v", spec)
	}
	if len(spec.Issues) != 3 {
		t.Fatalf("Store issuance count = %d, want the three storage bind/write forms", len(spec.Issues))
	}
	for index, issue := range spec.Issues {
		if !issue.Available() {
			t.Fatalf("Store issuance[%d] unavailable: %+v", index, issue)
		}
	}
	if problem, valid := spec.Program.Check(); !valid {
		t.Fatalf("RuleEntry Program rejected: %+v", problem)
	}
	// RuleEntry receives a fresh issue slice. Mutating a caller-owned copy must
	// not alter the next declaration value.
	spec.Issues[0].Occurrence = "mutated"
	if RuleEntry().Issues[0].Occurrence == "mutated" {
		t.Fatal("Store issuance geometry is shared between declarations")
	}
}

func TestStorageReadsUseOneStrictMaterializationContract(t *testing.T) {
	declaration := Storage()
	for index := 0; index < declaration.JoinCount(); index++ {
		join, ok := declaration.JoinAt(index)
		if !ok {
			t.Fatalf("join %d unavailable", index)
		}
		contract := join.Read.Contract
		if contract.Order != ruleprogram.OrderCanonical || contract.Sparse != ruleprogram.SparseExplicit ||
			contract.OnOpaque != ruleprogram.OnOpaqueRefuse || contract.Multiplicity != ruleprogram.MultiplicityOne {
			t.Fatalf("join %d contract = %+v, want strict canonical one-cell materialization", index, contract)
		}
	}
	first, _ := declaration.JoinAt(0)
	second, _ := declaration.JoinAt(1)
	if first.Read.Contract.DenominatorRef.Declared() {
		t.Fatal("foreign Value exact read acquired a fallback denominator")
	}
	if second.Read.Contract.DenominatorRef.EntryReference().Key != schema.Key("coordinates/placement") {
		t.Fatalf("Placement selected read denominator = %+v", second.Read.Contract.DenominatorRef)
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
	if relation.CandidateProvider.Axis.Key != valueAxisKey || relation.CandidateProvider.Member != StorageTransferCandidates ||
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

func TestStorageProgramRejectsMissingProviderAndRouteGeometry(t *testing.T) {
	missingProvider := Storage()
	missingProvider.Candidate = ruleprogram.Program{}.Candidate
	if problem, valid := missingProvider.Check(); valid || problem.Kind != ruleprogram.ProblemCandidate {
		t.Fatalf("missing candidate provider valid=%v problem=%+v", valid, problem)
	}

	wrongProvider := Storage()
	wrongProvider.Candidate.AxisRelation.Axis = schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "placement"}
	if _, failure := compileStorageProgram(t, wrongProvider); !failure.Available() {
		t.Fatal("wrong owner-qualified candidate provider was admitted by the seal plan")
	}

	missingRouteJoin := Storage()
	missingRouteJoin.Fold.Outputs[0].RouteJoinPresent = false
	if problem, valid := missingRouteJoin.Check(); valid || problem.Kind != ruleprogram.ProblemOutput {
		t.Fatalf("missing route join valid=%v problem=%+v", valid, problem)
	}

	wrongRouteJoin := Storage()
	wrongRouteJoin.Fold.Outputs[0].RouteJoin = 0
	if problem, valid := wrongRouteJoin.Check(); valid || problem.Kind != ruleprogram.ProblemJoin {
		t.Fatalf("exact join used as route source valid=%v problem=%+v", valid, problem)
	}

	unboundedRoute := Storage()
	unboundedRoute.Joins[1].Read.Contract.Multiplicity = ruleprogram.MultiplicityMany
	if problem, valid := unboundedRoute.Check(); valid || problem.Kind != ruleprogram.ProblemJoin {
		t.Fatalf("unbounded route valid=%v problem=%+v", valid, problem)
	}

	wrongDestination := Storage()
	wrongDestination.Fold.Outputs[0].Destination.Member = testRouteKey
	if _, failure := compileStorageProgram(t, wrongDestination); !failure.Available() {
		t.Fatal("route output accepted a key projection as its destination")
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
