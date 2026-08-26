package inputscope

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/rule/relinput"
	seal "github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// The fixture below compiles one rule catalog with three ordinals: a rule
// with one declared input port, a rule with two declared input ports fed by
// distinct joins, and a rule that declares no execution program at all. It is
// ported from the relinput package's own fixture: this package reads a
// relinput.View from the mount side, so its laws need the same catalog shape
// sealed into a real relinput.Bundle and frozen.

const (
	inputscopeValueAxis  schema.Key = "inputscope/value"
	inputscopeSingleAxis schema.Key = "inputscope/placement-single"
	inputscopeDoubleAxis schema.Key = "inputscope/placement-double"

	inputscopeCandidates schema.Key = "inputscope/value/candidates"

	inputscopeSingleRoutes      schema.Key = "inputscope/placement-single/routes"
	inputscopeSingleRouteKey    schema.Key = "inputscope/placement-single/key"
	inputscopeSinglePredicate   schema.Key = "inputscope/placement-single/predicate"
	inputscopeSingleSelection   schema.Key = "inputscope/placement-single/selection"
	inputscopeSingleDestination schema.Key = "inputscope/placement-single/destination"
	inputscopeSingleReducer     schema.Key = "inputscope/placement-single/reducer"
	inputscopeSingleOutput      schema.Key = "inputscope/placement-single/facts"
	inputscopeSingleDenominator schema.Key = "coordinates/inputscope/placement-single"
	inputscopeRuleSingle        schema.Key = "inputscope/rule/single"

	inputscopeDoubleRoutes      schema.Key = "inputscope/placement-double/routes"
	inputscopeDoubleRouteKey    schema.Key = "inputscope/placement-double/key"
	inputscopeDoublePredicate   schema.Key = "inputscope/placement-double/predicate"
	inputscopeDoubleSelection   schema.Key = "inputscope/placement-double/selection"
	inputscopeDoubleDestination schema.Key = "inputscope/placement-double/destination"
	inputscopeDoubleReducer     schema.Key = "inputscope/placement-double/reducer"
	inputscopeDoubleOutput      schema.Key = "inputscope/placement-double/facts"
	inputscopeDoubleDenominator schema.Key = "coordinates/inputscope/placement-double"
	inputscopeRuleDouble        schema.Key = "inputscope/rule/double"

	inputscopeRuleAbsent schema.Key = "inputscope/rule/absent"
)

type inputscopeNoopSurface struct {
	kind    schema.SurfaceKind
	entries []schema.Entry
}

func (surface inputscopeNoopSurface) Kind() schema.SurfaceKind { return surface.kind }
func (surface inputscopeNoopSurface) Entries() []schema.Entry {
	return append([]schema.Entry(nil), surface.entries...)
}
func (inputscopeNoopSurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

func inputscopeAxisRef(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

// inputscopeJoin builds one join reading the named routes relation at the
// given input port, in Selected form through a predicate-selected route.
func inputscopeJoin(placementKey schema.Key, routes, routeKey, predicate, selectionKey, denominatorKey schema.Key, input program.InputRef) program.JoinDecl {
	placementAxis := inputscopeAxisRef(placementKey)
	return program.JoinDecl{
		Sources:   []program.SourceRef{program.CandidateSource()},
		Relation:  member.RelationRef{Axis: placementAxis, Member: routes},
		Key:       member.ProjectionRef{Axis: placementAxis, Member: routeKey},
		Predicate: member.ProjectionRef{Axis: placementAxis, Member: predicate},
		Selection: member.SelectionRef{Axis: placementAxis, Member: selectionKey},
		Read: program.ReadDecl{
			Input: input, Axis: program.AxisRef(placementAxis), Form: program.Selected, PointBound: program.PointBound,
			Contract: program.ReadContract{Order: program.OrderCanonical, Sparse: program.SparseExplicit, OnOpaque: program.OnOpaqueRefuse, Multiplicity: program.MultiplicityOne, DenominatorRef: program.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: denominatorKey}},
		},
	}
}

// inputscopeSingleDeclaration is the one-port rule program: exactly one join
// filling port 0.
func inputscopeSingleDeclaration() program.Program {
	valueAxis := inputscopeAxisRef(inputscopeValueAxis)
	placementAxis := inputscopeAxisRef(inputscopeSingleAxis)
	join := inputscopeJoin(inputscopeSingleAxis, inputscopeSingleRoutes, inputscopeSingleRouteKey, inputscopeSinglePredicate, inputscopeSingleSelection, inputscopeSingleDenominator, program.InputRef(0))
	return program.Program{
		OperandRole: vocabulary.RoleKey("inputscope/operand"),
		Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: inputscopeCandidates}),
		Joins:       []program.JoinDecl{join},
		Fold: program.FoldDecl{
			Reducer: member.ReducerRef{Axis: placementAxis, Member: inputscopeSingleReducer}, Inputs: []program.JoinRef{0},
			Outputs: []program.OutputDecl{{Column: axis.OutputRef{Axis: placementAxis, Key: inputscopeSingleOutput}, Destination: member.ProjectionRef{Axis: placementAxis, Member: inputscopeSingleDestination}, Mode: program.ModeRoute, ValueSlot: 0, RouteJoin: 0, RouteJoinPresent: true}},
		},
	}
}

// inputscopeDoubleDeclaration is the two-port rule program: a selected join
// filling port 0 and an exact spare join filling port 1, so InputCount is 2
// and the two ports observe two independently-placeable scopes.
func inputscopeDoubleDeclaration() program.Program {
	valueAxis := inputscopeAxisRef(inputscopeValueAxis)
	placementAxis := inputscopeAxisRef(inputscopeDoubleAxis)
	primary := inputscopeJoin(inputscopeDoubleAxis, inputscopeDoubleRoutes, inputscopeDoubleRouteKey, inputscopeDoublePredicate, inputscopeDoubleSelection, inputscopeDoubleDenominator, program.InputRef(0))
	spare := primary
	spare.Predicate = member.ProjectionRef{}
	spare.Selection = member.SelectionRef{}
	spare.Read.Form = program.Exact
	spare.Read.Contract.Multiplicity = program.MultiplicityOne
	spare.Read.Input = program.InputRef(1)
	return program.Program{
		OperandRole: vocabulary.RoleKey("inputscope/operand"),
		Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: inputscopeCandidates}),
		Joins:       []program.JoinDecl{primary, spare},
		Fold: program.FoldDecl{
			Reducer: member.ReducerRef{Axis: placementAxis, Member: inputscopeDoubleReducer}, Inputs: []program.JoinRef{0, 1},
			Outputs: []program.OutputDecl{{Column: axis.OutputRef{Axis: placementAxis, Key: inputscopeDoubleOutput}, Destination: member.ProjectionRef{Axis: placementAxis, Member: inputscopeDoubleDestination}, Mode: program.ModeRoute, ValueSlot: 0, RouteJoin: 0, RouteJoinPresent: true}},
		},
	}
}

func inputscopeValueCatalog(t *testing.T) member.Catalog {
	t.Helper()
	provider := member.RelationRef{Axis: inputscopeAxisRef(inputscopeValueAxis), Member: inputscopeCandidates}
	catalog, ok := member.NewCatalog(
		[]member.Relation{{Key: inputscopeCandidates, Subject: "carrier/inputscope/value", CandidateProvider: member.AxisRelationCandidate(provider)}},
		nil, nil, nil,
	)
	if !ok {
		t.Fatal("inputscope value catalog rejected")
	}
	return catalog
}

func inputscopeSingleCatalog(t *testing.T) member.Catalog {
	t.Helper()
	provider := member.RelationRef{Axis: inputscopeAxisRef(inputscopeValueAxis), Member: inputscopeCandidates}
	catalog, ok := member.NewCatalog(
		[]member.Relation{{Key: inputscopeSingleRoutes, Subject: "carrier/inputscope/single-fact", Inputs: []member.Carrier{"carrier/inputscope/value"}, CandidateProvider: member.AxisRelationCandidate(provider)}},
		[]member.Projection{
			{Key: inputscopeSingleRouteKey, Relation: inputscopeSingleRoutes, Role: member.Key, Result: "carrier/inputscope/single-key", CandidateProvider: member.AxisRelationCandidate(provider)},
			{Key: inputscopeSinglePredicate, Relation: inputscopeSingleRoutes, Role: member.Predicate, Result: "carrier/inputscope/single-tag", CandidateProvider: member.AxisRelationCandidate(provider)},
			{Key: inputscopeSingleDestination, Relation: inputscopeSingleRoutes, Role: member.Destination, Result: "carrier/inputscope/single-key", CandidateProvider: member.AxisRelationCandidate(provider)},
		},
		[]member.Reducer{{
			Key: inputscopeSingleReducer,
			Inputs: []member.ReducerInput{
				{Axis: inputscopeAxisRef(inputscopeSingleAxis), Carrier: "carrier/inputscope/single-fact", Form: member.Selected, Multiplicity: member.MultiplicityOne, Tag: "carrier/inputscope/single-tag"},
			},
			Outputs: []member.ReducerOutput{{Axis: inputscopeAxisRef(inputscopeSingleAxis), Carrier: "carrier/inputscope/single-fact"}},
		}},
		nil,
	)
	if !ok {
		t.Fatal("inputscope single placement catalog rejected")
	}
	catalog, ok = catalog.WithSelections([]member.Selection{
		{Key: inputscopeSingleSelection, Relation: inputscopeSingleRoutes, Tag: inputscopeSinglePredicate},
	})
	if !ok {
		t.Fatal("inputscope single selection catalog rejected")
	}
	return catalog
}

func inputscopeDoubleCatalog(t *testing.T) member.Catalog {
	t.Helper()
	provider := member.RelationRef{Axis: inputscopeAxisRef(inputscopeValueAxis), Member: inputscopeCandidates}
	catalog, ok := member.NewCatalog(
		[]member.Relation{{Key: inputscopeDoubleRoutes, Subject: "carrier/inputscope/double-fact", Inputs: []member.Carrier{"carrier/inputscope/value"}, CandidateProvider: member.AxisRelationCandidate(provider)}},
		[]member.Projection{
			{Key: inputscopeDoubleRouteKey, Relation: inputscopeDoubleRoutes, Role: member.Key, Result: "carrier/inputscope/double-key", CandidateProvider: member.AxisRelationCandidate(provider)},
			{Key: inputscopeDoublePredicate, Relation: inputscopeDoubleRoutes, Role: member.Predicate, Result: "carrier/inputscope/double-tag", CandidateProvider: member.AxisRelationCandidate(provider)},
			{Key: inputscopeDoubleDestination, Relation: inputscopeDoubleRoutes, Role: member.Destination, Result: "carrier/inputscope/double-key", CandidateProvider: member.AxisRelationCandidate(provider)},
		},
		[]member.Reducer{{
			Key: inputscopeDoubleReducer,
			Inputs: []member.ReducerInput{
				{Axis: inputscopeAxisRef(inputscopeDoubleAxis), Carrier: "carrier/inputscope/double-fact", Form: member.Selected, Multiplicity: member.MultiplicityOne, Tag: "carrier/inputscope/double-tag"},
				{Axis: inputscopeAxisRef(inputscopeDoubleAxis), Carrier: "carrier/inputscope/double-fact", Form: member.Exact, Multiplicity: member.MultiplicityOne},
			},
			Outputs: []member.ReducerOutput{{Axis: inputscopeAxisRef(inputscopeDoubleAxis), Carrier: "carrier/inputscope/double-fact"}},
		}},
		nil,
	)
	if !ok {
		t.Fatal("inputscope double placement catalog rejected")
	}
	catalog, ok = catalog.WithSelections([]member.Selection{
		{Key: inputscopeDoubleSelection, Relation: inputscopeDoubleRoutes, Tag: inputscopeDoublePredicate},
	})
	if !ok {
		t.Fatal("inputscope double selection catalog rejected")
	}
	return catalog
}

// inputscopePlanCatalog compiles the three-ordinal rule catalog: ordinal 0 is
// the one-port rule, ordinal 1 is the two-port rule, ordinal 2 declares no
// program. It fatals the test on any rejection, since a fixture that cannot
// build states nothing about inputscope.
func inputscopePlanCatalog(t *testing.T) ruleplan.Catalog {
	t.Helper()

	singleDeclaration := inputscopeSingleDeclaration()
	if problem, valid := singleDeclaration.Check(); !valid {
		t.Fatalf("single declaration rejected: %+v", problem)
	}
	doubleDeclaration := inputscopeDoubleDeclaration()
	if problem, valid := doubleDeclaration.Check(); !valid {
		t.Fatalf("double declaration rejected: %+v", problem)
	}

	valueAxisTemplate, ok := axis.New(axis.Spec[struct{}]{
		Key: inputscopeValueAxis, Storage: axis.StorageEngine, Cardinality: axis.CardinalitySparse, Lifetime: axis.LifetimeProcess, Mutability: axis.MutabilityFrozen, Concurrency: axis.ConcurrencyShared,
		Catalog: inputscopeValueCatalog(t), Signature: axis.Signature{Key: "carrier/inputscope/value-key", Fact: "carrier/inputscope/value"}, Semantic: vocabulary.RoleKey("inputscope/value-axis"),
	})
	if !ok {
		t.Fatal("inputscope value axis rejected")
	}
	singleAxisTemplate, ok := axis.New(axis.Spec[struct{}]{
		Key: inputscopeSingleAxis, Storage: axis.StorageEngine, Cardinality: axis.CardinalitySparse, Lifetime: axis.LifetimeProcess, Mutability: axis.MutabilityFrozen, Concurrency: axis.ConcurrencyShared,
		Frame:   axis.Frame{Outputs: []axis.Output{{Key: inputscopeSingleOutput, Writer: inputscopeSingleAxis}}},
		Catalog: inputscopeSingleCatalog(t), Signature: axis.Signature{Key: "carrier/inputscope/single-key", Fact: "carrier/inputscope/single-fact"}, Semantic: vocabulary.RoleKey("inputscope/single-axis"),
	})
	if !ok {
		t.Fatal("inputscope single placement axis rejected")
	}
	doubleAxisTemplate, ok := axis.New(axis.Spec[struct{}]{
		Key: inputscopeDoubleAxis, Storage: axis.StorageEngine, Cardinality: axis.CardinalitySparse, Lifetime: axis.LifetimeProcess, Mutability: axis.MutabilityFrozen, Concurrency: axis.ConcurrencyShared,
		Frame:   axis.Frame{Outputs: []axis.Output{{Key: inputscopeDoubleOutput, Writer: inputscopeDoubleAxis}}},
		Catalog: inputscopeDoubleCatalog(t), Signature: axis.Signature{Key: "carrier/inputscope/double-key", Fact: "carrier/inputscope/double-fact"}, Semantic: vocabulary.RoleKey("inputscope/double-axis"),
	})
	if !ok {
		t.Fatal("inputscope double placement axis rejected")
	}

	ruleSingle, ok := rule.New(rule.Spec{
		Key: inputscopeRuleSingle, Lane: rule.LaneLink, Writes: inputscopeSingleAxis, Owner: inputscopeSingleAxis,
		Semantic: vocabulary.RoleKey("inputscope/rule-single"), Roles: []schema.Key{vocabulary.RoleKey("inputscope/operand")},
		Program: singleDeclaration,
	})
	if !ok {
		t.Fatal("inputscope single rule rejected")
	}
	ruleDouble, ok := rule.New(rule.Spec{
		Key: inputscopeRuleDouble, Lane: rule.LaneLink, Writes: inputscopeDoubleAxis, Owner: inputscopeDoubleAxis,
		Semantic: vocabulary.RoleKey("inputscope/rule-double"), Roles: []schema.Key{vocabulary.RoleKey("inputscope/operand")},
		Program: doubleDeclaration,
	})
	if !ok {
		t.Fatal("inputscope double rule rejected")
	}
	ruleAbsent, ok := rule.New(rule.Spec{
		Key: inputscopeRuleAbsent, Lane: rule.LaneLink, Writes: inputscopeSingleAxis, Owner: inputscopeSingleAxis,
		Semantic: vocabulary.RoleKey("inputscope/rule-absent"),
	})
	if !ok {
		t.Fatal("inputscope absent rule rejected")
	}

	roleNames := []string{
		"inputscope/value-axis", "inputscope/single-axis", "inputscope/double-axis",
		"inputscope/rule-single", "inputscope/rule-double", "inputscope/rule-absent",
		"inputscope/operand",
	}
	roles := make([]schema.Entry, 0, len(roleNames))
	for index, spelling := range roleNames {
		roleEntry, roleOK := structure.New(structure.Spec{Key: vocabulary.RoleKey(spelling), Category: structure.CategorySemanticRole, Ordinal: uint16(index + 1), Spelling: spelling, Accepted: true})
		if !roleOK {
			t.Fatalf("semantic role %q rejected", spelling)
		}
		roles = append(roles, roleEntry)
	}

	singleUniverse, ok := identity.DeriveContentID("go-lua/inputscope/law", []byte(inputscopeSingleDenominator))
	if !ok {
		t.Fatal("single denominator universe unavailable")
	}
	singleDenominatorEntry, ok := denominator.Coordinate(inputscopeSingleAxis, singleUniverse)
	if !ok {
		t.Fatal("single denominator rejected")
	}
	doubleUniverse, ok := identity.DeriveContentID("go-lua/inputscope/law", []byte(inputscopeDoubleDenominator))
	if !ok {
		t.Fatal("double denominator universe unavailable")
	}
	doubleDenominatorEntry, ok := denominator.Coordinate(inputscopeDoubleAxis, doubleUniverse)
	if !ok {
		t.Fatal("double denominator rejected")
	}

	builder := seal.NewBuilder()
	registered := builder.Register(inputscopeNoopSurface{kind: schema.SurfaceKindStructure, entries: roles}) &&
		builder.Register(axis.NewSurface([]*axis.Template[struct{}]{valueAxisTemplate, singleAxisTemplate, doubleAxisTemplate})) &&
		builder.Register(inputscopeNoopSurface{kind: schema.SurfaceKindIssuance}) &&
		builder.Register(rule.NewSurface([]*rule.Template{ruleSingle, ruleDouble, ruleAbsent})) &&
		builder.Register(inputscopeNoopSurface{kind: schema.SurfaceKindDiagnostic}) &&
		builder.Register(inputscopeNoopSurface{kind: schema.SurfaceKindComposite}) &&
		builder.Register(denominator.NewSurface([]*denominator.Entry{singleDenominatorEntry, doubleDenominatorEntry})) &&
		builder.Register(inputscopeNoopSurface{kind: schema.SurfaceKindQuery}) &&
		builder.Register(inputscopeNoopSurface{kind: schema.SurfaceKindObservation})
	if !registered {
		t.Fatal("inputscope schema surface registration failed")
	}
	table, failure := builder.Seal()
	if failure.Available() || table == nil {
		t.Fatalf("inputscope schema rejected: %+v", failure)
	}
	catalog, failure := ruleplan.Compile(table)
	if failure.Available() || !catalog.Available() {
		t.Fatalf("inputscope plan rejected: catalog=%#v failure=%+v", catalog, failure)
	}
	if catalog.Count() != 3 {
		t.Fatalf("inputscope fixture ordinal count = %d, want 3", catalog.Count())
	}

	single, held := catalog.At(0)
	if !held || !single.Present() || single.InputCount() != 1 {
		t.Fatalf("ordinal 0 (single) = present=%t inputCount=%d, want present=true inputCount=1", single.Present(), single.InputCount())
	}
	double, held := catalog.At(1)
	if !held || !double.Present() || double.InputCount() != 2 {
		t.Fatalf("ordinal 1 (double) = present=%t inputCount=%d, want present=true inputCount=2", double.Present(), double.InputCount())
	}
	absent, held := catalog.At(2)
	if !held || absent.Present() || absent.InputCount() != 0 {
		t.Fatalf("ordinal 2 (absent) = present=%t inputCount=%d, want present=false inputCount=0", absent.Present(), absent.InputCount())
	}

	return catalog
}

// inputscopeComposition is the test double for the relinput.Composition
// boundary Seal consults. It answers exactly the placements and regions it
// was loaded with, in the order they were added.
type inputscopeComposition struct {
	placements map[int]relinput.Placement
	regions    map[model.ScopeID]identity.ContentID
	order      []model.ScopeID
}

func newInputscopeComposition() *inputscopeComposition {
	return &inputscopeComposition{
		placements: make(map[int]relinput.Placement),
		regions:    make(map[model.ScopeID]identity.ContentID),
	}
}

func (composition *inputscopeComposition) place(ordinal int, placement relinput.Placement) *inputscopeComposition {
	composition.placements[ordinal] = placement
	return composition
}

func (composition *inputscopeComposition) region(scope model.ScopeID, evidence identity.ContentID) *inputscopeComposition {
	if _, held := composition.regions[scope]; !held {
		composition.order = append(composition.order, scope)
	}
	composition.regions[scope] = evidence
	return composition
}

func (composition *inputscopeComposition) Placement(ordinal int) (relinput.Placement, bool) {
	placement, held := composition.placements[ordinal]
	return placement, held
}

func (composition *inputscopeComposition) ScopeRegion(scope model.ScopeID) (identity.ContentID, bool) {
	evidence, held := composition.regions[scope]
	return evidence, held
}

func inputscopeOwner(t *testing.T, tag string) model.OwnerID {
	t.Helper()
	content, derived := identity.DeriveContentID("inputscope-law/owner", []byte(tag))
	if !derived {
		t.Fatalf("owner content %q undeliverable", tag)
	}
	owner, issued := model.IssueOwnerID(content)
	if !issued {
		t.Fatalf("owner %q not issued", tag)
	}
	return owner
}

func inputscopeScope(t *testing.T, owner model.OwnerID, tag string) model.ScopeID {
	t.Helper()
	content, derived := identity.DeriveContentID("inputscope-law/scope", []byte(tag))
	if !derived {
		t.Fatalf("scope content %q undeliverable", tag)
	}
	scope, issued := model.IssueScopeID(owner, content)
	if !issued {
		t.Fatalf("scope %q not issued", tag)
	}
	return scope
}

func inputscopeEvidence(t *testing.T, tag string) identity.ContentID {
	t.Helper()
	content, derived := identity.DeriveContentID("inputscope-law/region", []byte(tag))
	if !derived {
		t.Fatalf("region evidence %q undeliverable", tag)
	}
	return content
}

// inputscopeProjection seals a bundle over the fixture catalog, freezes it,
// opens the frozen view, and projects it, fataling on any refusal along the
// way. It returns the bundle alongside the projection so a law can compare
// the projection's answers against the sealed bundle's own.
func inputscopeProjection(t *testing.T, owner model.OwnerID, composition relinput.Composition) (*relinput.Bundle, Projection) {
	t.Helper()
	catalog := inputscopePlanCatalog(t)
	bundle, refusal := relinput.Seal(catalog, owner, composition)
	if refusal != nil {
		t.Fatalf("seal refused: %v", refusal)
	}
	store, storeIssued := identity.IssueStore()
	if !storeIssued {
		t.Fatal("store not issued")
	}
	frozen, frozeOK := bundle.Freeze(store)
	if !frozeOK {
		t.Fatal("bundle did not freeze")
	}
	view, opened := relinput.Open(&frozen, bundle.Catalog(), bundle.Owner())
	if !opened {
		t.Fatal("frozen publication did not open")
	}
	projection, projected := Project(view)
	if !projected {
		t.Fatal("view did not project")
	}
	return bundle, projection
}

// TestTheProjectionAnswersExactlyWhatTheOwnerIssued is the A/B identity law:
// every accessor the projection exposes answers identically to the sealed
// bundle it was frozen and reopened from, for every ordinal, every port, and
// every named scope.
func TestTheProjectionAnswersExactlyWhatTheOwnerIssued(t *testing.T) {
	owner := inputscopeOwner(t, "identity")

	candidateSingle := inputscopeScope(t, owner, "identity/candidate-single")
	portSingle := inputscopeScope(t, owner, "identity/port-single")
	candidateDouble := inputscopeScope(t, owner, "identity/candidate-double")
	portFirst := inputscopeScope(t, owner, "identity/port-first")
	portSecond := inputscopeScope(t, owner, "identity/port-second")

	composition := newInputscopeComposition().
		place(0, relinput.Placement{Candidate: candidateSingle, Ports: []model.ScopeID{portSingle}}).
		place(1, relinput.Placement{Candidate: candidateDouble, Ports: []model.ScopeID{portFirst, portSecond}}).
		region(candidateSingle, inputscopeEvidence(t, "identity/candidate-single")).
		region(portSingle, inputscopeEvidence(t, "identity/port-single")).
		region(candidateDouble, inputscopeEvidence(t, "identity/candidate-double")).
		region(portFirst, inputscopeEvidence(t, "identity/port-first")).
		region(portSecond, inputscopeEvidence(t, "identity/port-second"))

	bundle, projection := inputscopeProjection(t, owner, composition)

	if projection.RuleCount() != bundle.Count() {
		t.Fatalf("projection.RuleCount() = %d, want bundle.Count() = %d", projection.RuleCount(), bundle.Count())
	}
	for ordinal := 0; ordinal < bundle.Count(); ordinal++ {
		bundleCandidate, bundleCandidateHeld := bundle.CandidateScope(ordinal)
		projectionCandidate, projectionCandidateHeld := projection.CandidateScope(ordinal)
		if bundleCandidateHeld != projectionCandidateHeld || bundleCandidate != projectionCandidate {
			t.Fatalf("ordinal %d: bundle.CandidateScope = %v/%t, projection.CandidateScope = %v/%t", ordinal, bundleCandidate, bundleCandidateHeld, projectionCandidate, projectionCandidateHeld)
		}

		bundlePortCount, bundlePortCountHeld := bundle.PortCount(ordinal)
		projectionPortCount, projectionPortCountHeld := projection.PortCount(ordinal)
		if bundlePortCountHeld != projectionPortCountHeld || bundlePortCount != projectionPortCount {
			t.Fatalf("ordinal %d: bundle.PortCount = %d/%t, projection.PortCount = %d/%t", ordinal, bundlePortCount, bundlePortCountHeld, projectionPortCount, projectionPortCountHeld)
		}

		for port := 0; port < bundlePortCount+1; port++ {
			bundleScope, bundleScopeHeld := bundle.PortScope(ordinal, port)
			projectionScope, projectionScopeHeld := projection.PortScope(ordinal, port)
			if bundleScopeHeld != projectionScopeHeld || bundleScope != projectionScope {
				t.Fatalf("ordinal %d port %d: bundle.PortScope = %v/%t, projection.PortScope = %v/%t", ordinal, port, bundleScope, bundleScopeHeld, projectionScope, projectionScopeHeld)
			}
		}
	}

	if projection.ScopeCount() != bundle.RegionCount() {
		t.Fatalf("projection.ScopeCount() = %d, want bundle.RegionCount() = %d", projection.ScopeCount(), bundle.RegionCount())
	}
	for index := 0; index < bundle.RegionCount(); index++ {
		bundleRegion, bundleRegionHeld := bundle.RegionAt(index)
		projectionScope, projectionEvidence, projectionHeld := projection.ScopeAt(index)
		if bundleRegionHeld != projectionHeld || bundleRegion.Scope() != projectionScope || bundleRegion.Evidence() != projectionEvidence {
			t.Fatalf("region %d: bundle.RegionAt = %+v/%t, projection.ScopeAt = %v/%v/%t", index, bundleRegion, bundleRegionHeld, projectionScope, projectionEvidence, projectionHeld)
		}

		bundleEvidence, bundleEvidenceHeld := bundle.ScopeRegion(bundleRegion.Scope())
		projectionEvidenceByScope, projectionEvidenceHeld := projection.Evidence(bundleRegion.Scope())
		if bundleEvidenceHeld != projectionEvidenceHeld || bundleEvidence != projectionEvidenceByScope {
			t.Fatalf("scope %v: bundle.ScopeRegion = %v/%t, projection.Evidence = %v/%t", bundleRegion.Scope(), bundleEvidence, bundleEvidenceHeld, projectionEvidenceByScope, projectionEvidenceHeld)
		}
	}
}

// TestAProjectionAdmitsOnlyTheRegionIdentityItsOwnerIssued states the
// admission law: a physical region is admitted for a scope exactly when the
// identity it claims is the identity the bundle's owner issued for that
// scope, and never otherwise.
func TestAProjectionAdmitsOnlyTheRegionIdentityItsOwnerIssued(t *testing.T) {
	owner := inputscopeOwner(t, "admission")
	candidate := inputscopeScope(t, owner, "admission/candidate")
	port := inputscopeScope(t, owner, "admission/port")
	unnamed := inputscopeScope(t, owner, "admission/unnamed")
	candidateDouble := inputscopeScope(t, owner, "admission/candidate-double")
	portFirst := inputscopeScope(t, owner, "admission/port-first")
	portSecond := inputscopeScope(t, owner, "admission/port-second")

	candidateEvidence := inputscopeEvidence(t, "admission/candidate")
	portEvidence := inputscopeEvidence(t, "admission/port")
	otherEvidence := inputscopeEvidence(t, "admission/other")

	composition := newInputscopeComposition().
		place(0, relinput.Placement{Candidate: candidate, Ports: []model.ScopeID{port}}).
		place(1, relinput.Placement{Candidate: candidateDouble, Ports: []model.ScopeID{portFirst, portSecond}}).
		region(candidate, candidateEvidence).
		region(port, portEvidence).
		region(candidateDouble, inputscopeEvidence(t, "admission/candidate-double")).
		region(portFirst, inputscopeEvidence(t, "admission/port-first")).
		region(portSecond, inputscopeEvidence(t, "admission/port-second"))

	_, projection := inputscopeProjection(t, owner, composition)

	for _, named := range []struct {
		scope    model.ScopeID
		evidence identity.ContentID
	}{
		{candidate, candidateEvidence},
		{port, portEvidence},
	} {
		if !projection.Admits(named.scope, named.evidence) {
			t.Fatalf("scope %v did not admit its own issued evidence", named.scope)
		}
		if projection.Admits(named.scope, otherEvidence) {
			t.Fatalf("scope %v admitted an evidence identity it was not issued", named.scope)
		}
		if projection.Admits(named.scope, identity.ContentID{}) {
			t.Fatalf("scope %v admitted an unavailable claimed identity", named.scope)
		}
	}

	if projection.Admits(unnamed, candidateEvidence) {
		t.Fatal("projection admitted evidence for a scope the bundle never named")
	}
}

// TestAnUnavailableViewProjectsNothing states the fence on an absent
// publication: Project refuses a view that names no sealed bundle, and the
// zero Projection it returns answers nothing rather than defaulting.
func TestAnUnavailableViewProjectsNothing(t *testing.T) {
	projection, projected := Project(relinput.View{})
	if projected {
		t.Fatal("Project admitted an unavailable view")
	}
	if projection.Available() {
		t.Fatal("the zero Projection reports itself available")
	}
	if projection.RuleCount() != 0 {
		t.Fatalf("RuleCount() = %d, want 0", projection.RuleCount())
	}
	if _, held := projection.CandidateScope(0); held {
		t.Fatal("CandidateScope answered on an unavailable projection")
	}
	if _, held := projection.PortCount(0); held {
		t.Fatal("PortCount answered on an unavailable projection")
	}
	if _, held := projection.PortScope(0, 0); held {
		t.Fatal("PortScope answered on an unavailable projection")
	}
	if _, held := projection.Evidence(model.ScopeID{}); held {
		t.Fatal("Evidence answered on an unavailable projection")
	}
	if _, _, held := projection.ScopeAt(0); held {
		t.Fatal("ScopeAt answered on an unavailable projection")
	}
	if projection.Admits(model.ScopeID{}, identity.ContentID{}) {
		t.Fatal("Admits admitted on an unavailable projection")
	}
}

// TestTheProjectionCarriesItsOwnersFence states that a mount reading this
// projection is fenced by the same owner and catalog digest the bundle was
// sealed under.
func TestTheProjectionCarriesItsOwnersFence(t *testing.T) {
	owner := inputscopeOwner(t, "fence")
	candidate := inputscopeScope(t, owner, "fence/candidate")
	port := inputscopeScope(t, owner, "fence/port")
	candidateDouble := inputscopeScope(t, owner, "fence/candidate-double")
	portFirst := inputscopeScope(t, owner, "fence/port-first")
	portSecond := inputscopeScope(t, owner, "fence/port-second")

	composition := newInputscopeComposition().
		place(0, relinput.Placement{Candidate: candidate, Ports: []model.ScopeID{port}}).
		place(1, relinput.Placement{Candidate: candidateDouble, Ports: []model.ScopeID{portFirst, portSecond}}).
		region(candidate, inputscopeEvidence(t, "fence/candidate")).
		region(port, inputscopeEvidence(t, "fence/port")).
		region(candidateDouble, inputscopeEvidence(t, "fence/candidate-double")).
		region(portFirst, inputscopeEvidence(t, "fence/port-first")).
		region(portSecond, inputscopeEvidence(t, "fence/port-second"))

	bundle, projection := inputscopeProjection(t, owner, composition)

	if projection.Owner() != bundle.Owner() {
		t.Fatalf("projection.Owner() = %v, want bundle.Owner() = %v", projection.Owner(), bundle.Owner())
	}
	if projection.Catalog() != bundle.Catalog() {
		t.Fatalf("projection.Catalog() = %v, want bundle.Catalog() = %v", projection.Catalog(), bundle.Catalog())
	}
}
