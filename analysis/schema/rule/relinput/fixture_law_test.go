package relinput

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
	seal "github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// The fixture below compiles one rule catalog with three ordinals: a rule
// with one declared input port, a rule with two declared input ports fed by
// distinct joins, and a rule that declares no execution program at all. The
// catalog is built through the same axis/member/rule/plan machinery
// production schemas go through; nothing here is a relinput concern.

const (
	relinputValueAxis  schema.Key = "relinput/value"
	relinputSingleAxis schema.Key = "relinput/placement-single"
	relinputDoubleAxis schema.Key = "relinput/placement-double"

	relinputCandidates schema.Key = "relinput/value/candidates"

	relinputSingleRoutes      schema.Key = "relinput/placement-single/routes"
	relinputSingleRouteKey    schema.Key = "relinput/placement-single/key"
	relinputSinglePredicate   schema.Key = "relinput/placement-single/predicate"
	relinputSingleSelection   schema.Key = "relinput/placement-single/selection"
	relinputSingleDestination schema.Key = "relinput/placement-single/destination"
	relinputSingleReducer     schema.Key = "relinput/placement-single/reducer"
	relinputSingleOutput      schema.Key = "relinput/placement-single/facts"
	relinputSingleDenominator schema.Key = "coordinates/relinput/placement-single"
	relinputRuleSingle        schema.Key = "relinput/rule/single"

	relinputDoubleRoutes      schema.Key = "relinput/placement-double/routes"
	relinputDoubleRouteKey    schema.Key = "relinput/placement-double/key"
	relinputDoublePredicate   schema.Key = "relinput/placement-double/predicate"
	relinputDoubleSelection   schema.Key = "relinput/placement-double/selection"
	relinputDoubleDestination schema.Key = "relinput/placement-double/destination"
	relinputDoubleReducer     schema.Key = "relinput/placement-double/reducer"
	relinputDoubleOutput      schema.Key = "relinput/placement-double/facts"
	relinputDoubleDenominator schema.Key = "coordinates/relinput/placement-double"
	relinputRuleDouble        schema.Key = "relinput/rule/double"

	relinputRuleAbsent schema.Key = "relinput/rule/absent"
)

type relinputNoopSurface struct {
	kind    schema.SurfaceKind
	entries []schema.Entry
}

func (surface relinputNoopSurface) Kind() schema.SurfaceKind { return surface.kind }
func (surface relinputNoopSurface) Entries() []schema.Entry {
	return append([]schema.Entry(nil), surface.entries...)
}
func (relinputNoopSurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

func relinputAxisRef(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

// relinputJoin builds one join reading the named routes relation at the
// given input port, in Selected form through a predicate-selected route.
func relinputJoin(placementKey schema.Key, routes, routeKey, predicate, selectionKey, denominatorKey schema.Key, input program.InputRef) program.JoinDecl {
	placementAxis := relinputAxisRef(placementKey)
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

// relinputSingleDeclaration is the one-port rule program: exactly one join
// filling port 0.
func relinputSingleDeclaration() program.Program {
	valueAxis := relinputAxisRef(relinputValueAxis)
	placementAxis := relinputAxisRef(relinputSingleAxis)
	join := relinputJoin(relinputSingleAxis, relinputSingleRoutes, relinputSingleRouteKey, relinputSinglePredicate, relinputSingleSelection, relinputSingleDenominator, program.InputRef(0))
	return program.Program{
		OperandRole: vocabulary.RoleKey("relinput/operand"),
		Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: relinputCandidates}),
		Joins:       []program.JoinDecl{join},
		Fold: program.FoldDecl{
			Reducer: member.ReducerRef{Axis: placementAxis, Member: relinputSingleReducer}, Inputs: []program.JoinRef{0},
			Outputs: []program.OutputDecl{{Column: axis.OutputRef{Axis: placementAxis, Key: relinputSingleOutput}, Destination: member.ProjectionRef{Axis: placementAxis, Member: relinputSingleDestination}, Mode: program.ModeRoute, ValueSlot: 0, RouteJoin: 0, RouteJoinPresent: true}},
		},
	}
}

// relinputDoubleDeclaration is the two-port rule program: a selected join
// filling port 0 and an exact spare join filling port 1, so InputCount is 2
// and the two ports observe two independently-placeable scopes.
func relinputDoubleDeclaration() program.Program {
	valueAxis := relinputAxisRef(relinputValueAxis)
	placementAxis := relinputAxisRef(relinputDoubleAxis)
	primary := relinputJoin(relinputDoubleAxis, relinputDoubleRoutes, relinputDoubleRouteKey, relinputDoublePredicate, relinputDoubleSelection, relinputDoubleDenominator, program.InputRef(0))
	spare := primary
	spare.Predicate = member.ProjectionRef{}
	spare.Selection = member.SelectionRef{}
	spare.Read.Form = program.Exact
	spare.Read.Contract.Multiplicity = program.MultiplicityOne
	spare.Read.Input = program.InputRef(1)
	return program.Program{
		OperandRole: vocabulary.RoleKey("relinput/operand"),
		Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: relinputCandidates}),
		Joins:       []program.JoinDecl{primary, spare},
		Fold: program.FoldDecl{
			Reducer: member.ReducerRef{Axis: placementAxis, Member: relinputDoubleReducer}, Inputs: []program.JoinRef{0, 1},
			Outputs: []program.OutputDecl{{Column: axis.OutputRef{Axis: placementAxis, Key: relinputDoubleOutput}, Destination: member.ProjectionRef{Axis: placementAxis, Member: relinputDoubleDestination}, Mode: program.ModeRoute, ValueSlot: 0, RouteJoin: 0, RouteJoinPresent: true}},
		},
	}
}

func relinputValueCatalog(t *testing.T) member.Catalog {
	t.Helper()
	provider := member.RelationRef{Axis: relinputAxisRef(relinputValueAxis), Member: relinputCandidates}
	catalog, ok := member.NewCatalog(
		[]member.Relation{{Key: relinputCandidates, Subject: "carrier/relinput/value", CandidateProvider: member.AxisRelationCandidate(provider)}},
		nil, nil, nil,
	)
	if !ok {
		t.Fatal("relinput value catalog rejected")
	}
	return catalog
}

func relinputSingleCatalog(t *testing.T) member.Catalog {
	t.Helper()
	provider := member.RelationRef{Axis: relinputAxisRef(relinputValueAxis), Member: relinputCandidates}
	catalog, ok := member.NewCatalog(
		[]member.Relation{{Key: relinputSingleRoutes, Subject: "carrier/relinput/single-fact", Inputs: []member.Carrier{"carrier/relinput/value"}, CandidateProvider: member.AxisRelationCandidate(provider)}},
		[]member.Projection{
			{Key: relinputSingleRouteKey, Relation: relinputSingleRoutes, Role: member.Key, Result: "carrier/relinput/single-key", CandidateProvider: member.AxisRelationCandidate(provider)},
			{Key: relinputSinglePredicate, Relation: relinputSingleRoutes, Role: member.Predicate, Result: "carrier/relinput/single-tag", CandidateProvider: member.AxisRelationCandidate(provider)},
			{Key: relinputSingleDestination, Relation: relinputSingleRoutes, Role: member.Destination, Result: "carrier/relinput/single-key", CandidateProvider: member.AxisRelationCandidate(provider)},
		},
		[]member.Reducer{{
			Key: relinputSingleReducer,
			Inputs: []member.ReducerInput{
				{Axis: relinputAxisRef(relinputSingleAxis), Carrier: "carrier/relinput/single-fact", Form: member.Selected, Multiplicity: member.MultiplicityOne, Tag: "carrier/relinput/single-tag"},
			},
			Outputs: []member.ReducerOutput{{Axis: relinputAxisRef(relinputSingleAxis), Carrier: "carrier/relinput/single-fact"}},
		}},
		nil,
	)
	if !ok {
		t.Fatal("relinput single placement catalog rejected")
	}
	catalog, ok = catalog.WithSelections([]member.Selection{
		{Key: relinputSingleSelection, Relation: relinputSingleRoutes, Tag: relinputSinglePredicate},
	})
	if !ok {
		t.Fatal("relinput single selection catalog rejected")
	}
	return catalog
}

func relinputDoubleCatalog(t *testing.T) member.Catalog {
	t.Helper()
	provider := member.RelationRef{Axis: relinputAxisRef(relinputValueAxis), Member: relinputCandidates}
	catalog, ok := member.NewCatalog(
		[]member.Relation{{Key: relinputDoubleRoutes, Subject: "carrier/relinput/double-fact", Inputs: []member.Carrier{"carrier/relinput/value"}, CandidateProvider: member.AxisRelationCandidate(provider)}},
		[]member.Projection{
			{Key: relinputDoubleRouteKey, Relation: relinputDoubleRoutes, Role: member.Key, Result: "carrier/relinput/double-key", CandidateProvider: member.AxisRelationCandidate(provider)},
			{Key: relinputDoublePredicate, Relation: relinputDoubleRoutes, Role: member.Predicate, Result: "carrier/relinput/double-tag", CandidateProvider: member.AxisRelationCandidate(provider)},
			{Key: relinputDoubleDestination, Relation: relinputDoubleRoutes, Role: member.Destination, Result: "carrier/relinput/double-key", CandidateProvider: member.AxisRelationCandidate(provider)},
		},
		[]member.Reducer{{
			Key: relinputDoubleReducer,
			Inputs: []member.ReducerInput{
				{Axis: relinputAxisRef(relinputDoubleAxis), Carrier: "carrier/relinput/double-fact", Form: member.Selected, Multiplicity: member.MultiplicityOne, Tag: "carrier/relinput/double-tag"},
				{Axis: relinputAxisRef(relinputDoubleAxis), Carrier: "carrier/relinput/double-fact", Form: member.Exact, Multiplicity: member.MultiplicityOne},
			},
			Outputs: []member.ReducerOutput{{Axis: relinputAxisRef(relinputDoubleAxis), Carrier: "carrier/relinput/double-fact"}},
		}},
		nil,
	)
	if !ok {
		t.Fatal("relinput double placement catalog rejected")
	}
	catalog, ok = catalog.WithSelections([]member.Selection{
		{Key: relinputDoubleSelection, Relation: relinputDoubleRoutes, Tag: relinputDoublePredicate},
	})
	if !ok {
		t.Fatal("relinput double selection catalog rejected")
	}
	return catalog
}

// relinputPlanCatalog compiles the three-ordinal rule catalog: ordinal 0 is
// the one-port rule, ordinal 1 is the two-port rule, ordinal 2 declares no
// program. It fatals the test on any rejection, since a fixture that cannot
// build states nothing about relinput.
func relinputPlanCatalog(t *testing.T) ruleplan.Catalog {
	t.Helper()

	singleDeclaration := relinputSingleDeclaration()
	if problem, valid := singleDeclaration.Check(); !valid {
		t.Fatalf("single declaration rejected: %+v", problem)
	}
	doubleDeclaration := relinputDoubleDeclaration()
	if problem, valid := doubleDeclaration.Check(); !valid {
		t.Fatalf("double declaration rejected: %+v", problem)
	}

	valueAxisTemplate, ok := axis.New(axis.Spec[struct{}]{
		Key: relinputValueAxis, Storage: axis.StorageEngine, Cardinality: axis.CardinalitySparse, Lifetime: axis.LifetimeProcess, Mutability: axis.MutabilityFrozen, Concurrency: axis.ConcurrencyShared,
		Catalog: relinputValueCatalog(t), Signature: axis.Signature{Key: "carrier/relinput/value-key", Fact: "carrier/relinput/value"}, Semantic: vocabulary.RoleKey("relinput/value-axis"),
	})
	if !ok {
		t.Fatal("relinput value axis rejected")
	}
	singleAxisTemplate, ok := axis.New(axis.Spec[struct{}]{
		Key: relinputSingleAxis, Storage: axis.StorageEngine, Cardinality: axis.CardinalitySparse, Lifetime: axis.LifetimeProcess, Mutability: axis.MutabilityFrozen, Concurrency: axis.ConcurrencyShared,
		Frame:   axis.Frame{Outputs: []axis.Output{{Key: relinputSingleOutput, Writer: relinputSingleAxis}}},
		Catalog: relinputSingleCatalog(t), Signature: axis.Signature{Key: "carrier/relinput/single-key", Fact: "carrier/relinput/single-fact"}, Semantic: vocabulary.RoleKey("relinput/single-axis"),
	})
	if !ok {
		t.Fatal("relinput single placement axis rejected")
	}
	doubleAxisTemplate, ok := axis.New(axis.Spec[struct{}]{
		Key: relinputDoubleAxis, Storage: axis.StorageEngine, Cardinality: axis.CardinalitySparse, Lifetime: axis.LifetimeProcess, Mutability: axis.MutabilityFrozen, Concurrency: axis.ConcurrencyShared,
		Frame:   axis.Frame{Outputs: []axis.Output{{Key: relinputDoubleOutput, Writer: relinputDoubleAxis}}},
		Catalog: relinputDoubleCatalog(t), Signature: axis.Signature{Key: "carrier/relinput/double-key", Fact: "carrier/relinput/double-fact"}, Semantic: vocabulary.RoleKey("relinput/double-axis"),
	})
	if !ok {
		t.Fatal("relinput double placement axis rejected")
	}

	ruleSingle, ok := rule.New(rule.Spec{
		Key: relinputRuleSingle, Lane: rule.LaneLink, Writes: relinputSingleAxis, Owner: relinputSingleAxis,
		Semantic: vocabulary.RoleKey("relinput/rule-single"), Roles: []schema.Key{vocabulary.RoleKey("relinput/operand")},
		Program: singleDeclaration,
	})
	if !ok {
		t.Fatal("relinput single rule rejected")
	}
	ruleDouble, ok := rule.New(rule.Spec{
		Key: relinputRuleDouble, Lane: rule.LaneLink, Writes: relinputDoubleAxis, Owner: relinputDoubleAxis,
		Semantic: vocabulary.RoleKey("relinput/rule-double"), Roles: []schema.Key{vocabulary.RoleKey("relinput/operand")},
		Program: doubleDeclaration,
	})
	if !ok {
		t.Fatal("relinput double rule rejected")
	}
	ruleAbsent, ok := rule.New(rule.Spec{
		Key: relinputRuleAbsent, Lane: rule.LaneLink, Writes: relinputSingleAxis, Owner: relinputSingleAxis,
		Semantic: vocabulary.RoleKey("relinput/rule-absent"),
	})
	if !ok {
		t.Fatal("relinput absent rule rejected")
	}

	roleNames := []string{
		"relinput/value-axis", "relinput/single-axis", "relinput/double-axis",
		"relinput/rule-single", "relinput/rule-double", "relinput/rule-absent",
		"relinput/operand",
	}
	roles := make([]schema.Entry, 0, len(roleNames))
	for index, spelling := range roleNames {
		roleEntry, roleOK := structure.New(structure.Spec{Key: vocabulary.RoleKey(spelling), Category: structure.CategorySemanticRole, Ordinal: uint16(index + 1), Spelling: spelling, Accepted: true})
		if !roleOK {
			t.Fatalf("semantic role %q rejected", spelling)
		}
		roles = append(roles, roleEntry)
	}

	singleUniverse, ok := identity.DeriveContentID("go-lua/relinput/law", []byte(relinputSingleDenominator))
	if !ok {
		t.Fatal("single denominator universe unavailable")
	}
	singleDenominatorEntry, ok := denominator.Coordinate(relinputSingleAxis, singleUniverse)
	if !ok {
		t.Fatal("single denominator rejected")
	}
	doubleUniverse, ok := identity.DeriveContentID("go-lua/relinput/law", []byte(relinputDoubleDenominator))
	if !ok {
		t.Fatal("double denominator universe unavailable")
	}
	doubleDenominatorEntry, ok := denominator.Coordinate(relinputDoubleAxis, doubleUniverse)
	if !ok {
		t.Fatal("double denominator rejected")
	}

	builder := seal.NewBuilder()
	registered := builder.Register(relinputNoopSurface{kind: schema.SurfaceKindStructure, entries: roles}) &&
		builder.Register(axis.NewSurface([]*axis.Template[struct{}]{valueAxisTemplate, singleAxisTemplate, doubleAxisTemplate})) &&
		builder.Register(relinputNoopSurface{kind: schema.SurfaceKindIssuance}) &&
		builder.Register(rule.NewSurface([]*rule.Template{ruleSingle, ruleDouble, ruleAbsent})) &&
		builder.Register(relinputNoopSurface{kind: schema.SurfaceKindDiagnostic}) &&
		builder.Register(relinputNoopSurface{kind: schema.SurfaceKindComposite}) &&
		builder.Register(denominator.NewSurface([]*denominator.Entry{singleDenominatorEntry, doubleDenominatorEntry})) &&
		builder.Register(relinputNoopSurface{kind: schema.SurfaceKindQuery}) &&
		builder.Register(relinputNoopSurface{kind: schema.SurfaceKindObservation})
	if !registered {
		t.Fatal("relinput schema surface registration failed")
	}
	table, failure := builder.Seal()
	if failure.Available() || table == nil {
		t.Fatalf("relinput schema rejected: %+v", failure)
	}
	catalog, failure := ruleplan.Compile(table)
	if failure.Available() || !catalog.Available() {
		t.Fatalf("relinput plan rejected: catalog=%#v failure=%+v", catalog, failure)
	}
	if catalog.Count() != 3 {
		t.Fatalf("relinput fixture ordinal count = %d, want 3", catalog.Count())
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

// relinputComposition is the test double for the Composition boundary Seal
// consults. It answers exactly the placements and regions it was loaded
// with, in the order they were added.
type relinputComposition struct {
	placements map[int]Placement
	regions    map[model.ScopeID]identity.ContentID
	order      []model.ScopeID
}

func newRelinputComposition() *relinputComposition {
	return &relinputComposition{
		placements: make(map[int]Placement),
		regions:    make(map[model.ScopeID]identity.ContentID),
	}
}

func (composition *relinputComposition) place(ordinal int, placement Placement) *relinputComposition {
	composition.placements[ordinal] = placement
	return composition
}

func (composition *relinputComposition) region(scope model.ScopeID, evidence identity.ContentID) *relinputComposition {
	if _, held := composition.regions[scope]; !held {
		composition.order = append(composition.order, scope)
	}
	composition.regions[scope] = evidence
	return composition
}

func (composition *relinputComposition) Placement(ordinal int) (Placement, bool) {
	placement, held := composition.placements[ordinal]
	return placement, held
}

func (composition *relinputComposition) ScopeRegion(scope model.ScopeID) (identity.ContentID, bool) {
	evidence, held := composition.regions[scope]
	return evidence, held
}

func relinputOwner(t *testing.T, tag string) model.OwnerID {
	t.Helper()
	content, derived := identity.DeriveContentID("relinput-law/owner", []byte(tag))
	if !derived {
		t.Fatalf("owner content %q undeliverable", tag)
	}
	owner, issued := model.IssueOwnerID(content)
	if !issued {
		t.Fatalf("owner %q not issued", tag)
	}
	return owner
}

func relinputScope(t *testing.T, owner model.OwnerID, tag string) model.ScopeID {
	t.Helper()
	content, derived := identity.DeriveContentID("relinput-law/scope", []byte(tag))
	if !derived {
		t.Fatalf("scope content %q undeliverable", tag)
	}
	scope, issued := model.IssueScopeID(owner, content)
	if !issued {
		t.Fatalf("scope %q not issued", tag)
	}
	return scope
}

func relinputEvidence(t *testing.T, tag string) identity.ContentID {
	t.Helper()
	content, derived := identity.DeriveContentID("relinput-law/region", []byte(tag))
	if !derived {
		t.Fatalf("region evidence %q undeliverable", tag)
	}
	return content
}
