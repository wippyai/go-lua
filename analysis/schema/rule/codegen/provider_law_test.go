package codegen

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	memberdefinition "github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	membergenerator "github.com/wippyai/go-lua/analysis/schema/axis/member/generator"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
	seal "github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

const (
	providerValueAxis     schema.Key = "value"
	providerPlacementAxis schema.Key = "placement"
	providerCandidates    schema.Key = "value/storage-transfer/candidates"
	providerRoutes        schema.Key = "placement/storage-transfer/routes"
	providerRouteKey      schema.Key = "placement/storage-transfer/key"
	providerPredicate     schema.Key = "placement/storage-transfer/predicate"
	providerDestination   schema.Key = "placement/storage-transfer/destination"
	providerReducer       schema.Key = "placement/storage-transfer/reducer"
	providerOutput        schema.Key = "placement/facts"
	providerDenominator   schema.Key = "coordinates/placement"
	providerRule          schema.Key = "rule/placement-storage-transfer"
)

type providerNoopSurface struct {
	kind    schema.SurfaceKind
	entries []schema.Entry
}

func (surface providerNoopSurface) Kind() schema.SurfaceKind { return surface.kind }
func (surface providerNoopSurface) Entries() []schema.Entry {
	return append([]schema.Entry(nil), surface.entries...)
}
func (providerNoopSurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

func providerAxisRef(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func providerType(pkg, name string) memberdefinition.GoType {
	return memberdefinition.GoType{PackagePath: pkg, Name: name}
}

func providerSymbol(pkg, name string, receiver memberdefinition.GoType) memberdefinition.GoSymbol {
	return memberdefinition.GoSymbol{PackagePath: pkg, Name: name, Receiver: receiver, ResultIndex: 0}
}

func providerDirectoryMetadata() membergenerator.Metadata {
	owner := providerType("example/value", "Schema")
	candidate := providerType("example/value", "StorageTransfer")
	provider := member.RelationRef{Axis: providerAxisRef(providerValueAxis), Member: providerCandidates}
	return membergenerator.Metadata{
		Axis:        providerValueAxis,
		FactCarrier: "carrier/value/fact",
		FactType:    providerType("example/value", "Value"),
		Key: membergenerator.KeyBinding{
			Carrier:    "carrier/value/key",
			Input:      providerType("example/value", "Coordinate"),
			Dense:      memberdefinition.GoType{Name: "uint32"},
			Normalizer: providerSymbol("example/value", "CoordinateIndex", owner),
		},
		Relations: []membergenerator.RelationBinding{{
			Key:                    providerCandidates,
			Subject:                candidate,
			CandidateProvider:      member.AxisRelationCandidate(provider),
			CandidateResolver:      providerSymbol("example/value", "StorageTransferForOccurrence", owner),
			CandidateOrdinal:       providerSymbol("example/value", "StorageTransferOrdinal", owner),
			CandidateAt:            providerSymbol("example/value", "StorageTransferAt", owner),
			CandidateRelation:      0,
			HasCandidateRelation:   true,
			CandidateProviderLocal: true,
		}},
	}
}

func providerConsumerMetadata() membergenerator.Metadata {
	owner := providerType("example/placement", "Schema")
	fact := providerType("example/placement", "Fact")
	candidate := providerType("example/value", "StorageTransfer")
	provider := member.RelationRef{Axis: providerAxisRef(providerValueAxis), Member: providerCandidates}
	return membergenerator.Metadata{
		Axis:        providerPlacementAxis,
		FactCarrier: "carrier/placement/fact",
		FactType:    fact,
		Key: membergenerator.KeyBinding{
			Carrier:    "carrier/placement/key",
			Input:      providerType("example/placement", "Coordinate"),
			Dense:      memberdefinition.GoType{Name: "uint32"},
			Normalizer: providerSymbol("example/placement", "CoordinateIndex", owner),
		},
		Relations: []membergenerator.RelationBinding{{
			Key:                    providerRoutes,
			Subject:                fact,
			Inputs:                 []memberdefinition.GoType{candidate},
			CandidateProvider:      member.AxisRelationCandidate(provider),
			CandidateRelation:      0,
			CandidateProviderLocal: false,
		}},
		Projections: []membergenerator.ProjectionBinding{
			{
				Key: providerRouteKey, Relation: providerRoutes, Role: member.Key,
				Result:            providerType("example/placement", "Coordinate"),
				Accessor:          providerSymbol("example/placement", "RouteKey", fact),
				CandidateProvider: member.AxisRelationCandidate(provider), CandidateRelation: 0, CandidateProviderLocal: false,
			},
			{
				Key: providerPredicate, Relation: providerRoutes, Role: member.Predicate,
				Result:            providerType("example/placement", "Tag"),
				Accessor:          providerSymbol("example/placement", "Predicate", fact),
				CandidateProvider: member.AxisRelationCandidate(provider), CandidateRelation: 0, CandidateProviderLocal: false,
			},
			{
				Key: providerDestination, Relation: providerRoutes, Role: member.Destination,
				Result:            providerType("example/placement", "Coordinate"),
				Accessor:          providerSymbol("example/placement", "Destination", fact),
				CandidateProvider: member.AxisRelationCandidate(provider), CandidateRelation: 0, CandidateProviderLocal: false,
			},
		},
		Reducers: []membergenerator.ReducerBinding{{
			Key: providerReducer, Implementation: memberdefinition.GoSymbol{PackagePath: "example/placement", Name: "IdentityFact", ResultIndex: 0},
			CandidateConstant: true,
			Inputs: []membergenerator.ReducerInputBinding{{
				Axis: providerAxisRef(providerPlacementAxis), Type: fact, Form: member.Selected, Multiplicity: member.MultiplicityOne, Tag: providerType("example/placement", "Tag"),
			}},
			Outputs: []membergenerator.ReducerOutputBinding{{Axis: providerAxisRef(providerPlacementAxis), Type: fact}},
		}},
	}
}

func providerCatalogs(t *testing.T) (member.Catalog, member.Catalog) {
	t.Helper()
	return providerCatalogsFor(t, false)
}

func providerCatalogsFor(t *testing.T, spareJoin bool) (member.Catalog, member.Catalog) {
	t.Helper()
	valueProvider := member.RelationRef{Axis: providerAxisRef(providerValueAxis), Member: providerCandidates}
	valueCatalog, valueOK := member.NewCatalog(
		[]member.Relation{{Key: providerCandidates, Subject: "carrier/value/storage-transfer", CandidateProvider: member.AxisRelationCandidate(valueProvider)}},
		nil, nil, nil,
	)
	if !valueOK {
		t.Fatal("value provider catalog rejected")
	}
	placementProvider := member.RelationRef{Axis: providerAxisRef(providerValueAxis), Member: providerCandidates}
	placementCatalog, placementOK := member.NewCatalog(
		[]member.Relation{{Key: providerRoutes, Subject: "carrier/placement/fact", Inputs: []member.Carrier{"carrier/value/storage-transfer"}, CandidateProvider: member.AxisRelationCandidate(placementProvider)}},
		[]member.Projection{
			{Key: providerRouteKey, Relation: providerRoutes, Role: member.Key, Result: "carrier/placement/key", CandidateProvider: member.AxisRelationCandidate(placementProvider)},
			{Key: providerPredicate, Relation: providerRoutes, Role: member.Predicate, Result: "carrier/placement/tag", CandidateProvider: member.AxisRelationCandidate(placementProvider)},
			{Key: providerDestination, Relation: providerRoutes, Role: member.Destination, Result: "carrier/placement/key", CandidateProvider: member.AxisRelationCandidate(placementProvider)},
		},
		[]member.Reducer{{Key: providerReducer, Inputs: providerReducerInputs(spareJoin), Outputs: []member.ReducerOutput{{Axis: providerAxisRef(providerPlacementAxis), Carrier: "carrier/placement/fact"}}}},
		nil,
	)
	if !placementOK {
		t.Fatal("placement consumer catalog rejected")
	}
	return valueCatalog, placementCatalog
}

// providerJoin is the fixture's one selected join over the route relation. The
// spare-join variant reads it twice so a fold has an input no output routes
// through, which is the only shape that can state what a route carrier on an
// unrouted input does.
func providerJoin() program.JoinDecl {
	placementAxis := providerAxisRef(providerPlacementAxis)
	return program.JoinDecl{
		Sources:   []program.SourceRef{program.CandidateSource()},
		Relation:  member.RelationRef{Axis: placementAxis, Member: providerRoutes},
		Key:       member.ProjectionRef{Axis: placementAxis, Member: providerRouteKey},
		Predicate: member.ProjectionRef{Axis: placementAxis, Member: providerPredicate},
		Read: program.ReadDecl{
			Input: 0, Axis: program.AxisRef(placementAxis), Form: program.Selected, PointBound: program.PointBound,
			Contract: program.ReadContract{Order: program.OrderCanonical, Sparse: program.SparseExplicit, OnOpaque: program.OnOpaqueRefuse, Multiplicity: program.MultiplicityOne, DenominatorRef: program.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: providerDenominator}},
		},
	}
}

func providerDeclaration(spareJoin bool) program.Program {
	valueAxis := providerAxisRef(providerValueAxis)
	placementAxis := providerAxisRef(providerPlacementAxis)
	joins := []program.JoinDecl{providerJoin()}
	inputs := []program.JoinRef{0}
	if spareJoin {
		spare := providerJoin()
		spare.Predicate = member.ProjectionRef{}
		spare.Read.Form = program.Exact
		spare.Read.Contract.Multiplicity = program.MultiplicityOne
		// The spare join reads at its own port. A port is where the fold
		// receives one join's value and where the runtime resolves that join's
		// Factor requirement, so two joins cannot share one.
		spare.Read.Input = 1
		joins = append(joins, spare)
		inputs = append(inputs, 1)
	}
	return program.Program{
		OperandRole: vocabulary.RoleKey("provider/operand"),
		Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: valueAxis, Member: providerCandidates}),
		Joins:       joins,
		Fold: program.FoldDecl{
			Reducer: member.ReducerRef{Axis: placementAxis, Member: providerReducer}, Inputs: inputs,
			Outputs: []program.OutputDecl{{Column: axis.OutputRef{Axis: placementAxis, Key: providerOutput}, Destination: member.ProjectionRef{Axis: placementAxis, Member: providerDestination}, Mode: program.ModeRoute, ValueSlot: 0, RouteJoin: 0, RouteJoinPresent: true}},
		},
	}
}

func providerPlanCatalog(t *testing.T) ruleplan.Catalog {
	t.Helper()
	return providerPlanCatalogFor(t, providerDeclaration(false))
}

func providerReducerInputs(spareJoin bool) []member.ReducerInput {
	inputs := []member.ReducerInput{{Axis: providerAxisRef(providerPlacementAxis), Carrier: "carrier/placement/fact", Form: member.Selected, Multiplicity: member.MultiplicityOne, Tag: "carrier/placement/tag"}}
	if spareJoin {
		inputs = append(inputs, member.ReducerInput{Axis: providerAxisRef(providerPlacementAxis), Carrier: "carrier/placement/fact", Form: member.Exact, Multiplicity: member.MultiplicityOne})
	}
	return inputs
}

func providerPlanCatalogFor(t *testing.T, declaration program.Program) ruleplan.Catalog {
	t.Helper()
	valueCatalog, placementCatalog := providerCatalogsFor(t, declaration.JoinCount() > 1)
	if problem, valid := declaration.Check(); !valid {
		t.Fatalf("provider declaration rejected: %+v", problem)
	}

	valueAxisTemplate, valueOK := axis.New(axis.Spec[struct{}]{
		Key: providerValueAxis, Storage: axis.StorageEngine, Cardinality: axis.CardinalitySparse, Lifetime: axis.LifetimeProcess, Mutability: axis.MutabilityFrozen, Concurrency: axis.ConcurrencyShared,
		Catalog: valueCatalog, Signature: axis.Signature{Key: "carrier/value/key", Fact: "carrier/value/fact"}, Semantic: vocabulary.RoleKey("provider/value-axis"),
	})
	if !valueOK {
		t.Fatal("value axis rejected")
	}
	placementAxisTemplate, placementOK := axis.New(axis.Spec[struct{}]{
		Key: providerPlacementAxis, Storage: axis.StorageEngine, Cardinality: axis.CardinalitySparse, Lifetime: axis.LifetimeProcess, Mutability: axis.MutabilityFrozen, Concurrency: axis.ConcurrencyShared,
		Frame:   axis.Frame{Outputs: []axis.Output{{Key: providerOutput, Writer: providerPlacementAxis}}},
		Catalog: placementCatalog, Signature: axis.Signature{Key: "carrier/placement/key", Fact: "carrier/placement/fact"}, Semantic: vocabulary.RoleKey("provider/placement-axis"),
	})
	if !placementOK {
		t.Fatal("placement axis rejected")
	}
	ruleEntry, ruleOK := rule.New(rule.Spec{Key: providerRule, Lane: rule.LaneLink, Writes: providerPlacementAxis, Owner: providerPlacementAxis, Semantic: vocabulary.RoleKey("provider/rule"), Roles: []schema.Key{vocabulary.RoleKey("provider/operand")}, Program: declaration})
	if !ruleOK {
		t.Fatal("provider rule rejected")
	}
	roleNames := []string{"provider/value-axis", "provider/placement-axis", "provider/rule", "provider/operand"}
	roles := make([]schema.Entry, 0, len(roleNames))
	for index, spelling := range roleNames {
		roleEntry, roleOK := structure.New(structure.Spec{Key: vocabulary.RoleKey(spelling), Category: structure.CategorySemanticRole, Ordinal: uint16(index + 1), Spelling: spelling, Accepted: true})
		if !roleOK {
			t.Fatalf("semantic role %q rejected", spelling)
		}
		roles = append(roles, roleEntry)
	}
	universe, universeOK := identity.DeriveContentID("go-lua/codegen/provider", []byte(providerDenominator))
	if !universeOK {
		t.Fatal("provider denominator universe unavailable")
	}
	denominatorEntry, denominatorOK := denominator.Coordinate(providerPlacementAxis, universe)
	if !denominatorOK {
		t.Fatal("provider denominator rejected")
	}
	builder := seal.NewBuilder()
	if !builder.Register(providerNoopSurface{kind: schema.SurfaceKindStructure, entries: roles}) || !builder.Register(axis.NewSurface([]*axis.Template[struct{}]{valueAxisTemplate, placementAxisTemplate})) ||
		!builder.Register(providerNoopSurface{kind: schema.SurfaceKindIssuance}) || !builder.Register(rule.NewSurface([]*rule.Template{ruleEntry})) ||
		!builder.Register(providerNoopSurface{kind: schema.SurfaceKindDiagnostic}) || !builder.Register(providerNoopSurface{kind: schema.SurfaceKindComposite}) ||
		!builder.Register(denominator.NewSurface([]*denominator.Entry{denominatorEntry})) || !builder.Register(providerNoopSurface{kind: schema.SurfaceKindQuery}) || !builder.Register(providerNoopSurface{kind: schema.SurfaceKindObservation}) {
		t.Fatal("provider schema surface registration failed")
	}
	table, failure := builder.Seal()
	if failure.Available() || table == nil {
		t.Fatalf("provider schema rejected: %+v", failure)
	}
	catalog, failure := ruleplan.Compile(table)
	if failure.Available() || !catalog.Available() {
		t.Fatalf("provider plan rejected: catalog=%#v failure=%+v", catalog, failure)
	}
	return catalog
}

func TestBuildResolvesForeignProviderAndRetainsConsumerAccessors(t *testing.T) {
	catalog := providerPlanCatalog(t)
	model, err := Build([]membergenerator.Metadata{providerConsumerMetadata(), providerDirectoryMetadata()}, catalog)
	if err != nil || !model.Available() {
		t.Fatalf("foreign provider model rejected: model=%#v err=%#v", model, err)
	}
	row, rowOK := model.At(0)
	if !rowOK {
		t.Fatal("foreign provider rule missing")
	}
	join, joinOK := row.JoinAt(0)
	if !joinOK || join.RelationCandidate != (ruleplan.RelationAddr{Axis: 0, Member: 0}) || join.RelationResolver.Name != "StorageTransferForOccurrence" || join.KeyAccessor.Name != "RouteKey" {
		t.Fatalf("foreign provider join=%#v/%t", join, joinOK)
	}
	output, outputOK := row.OutputAt(0)
	if !outputOK || output.DestinationAccessor().Name != "Destination" {
		t.Fatalf("consumer destination accessor=%#v/%t", output.DestinationAccessor(), outputOK)
	}
}

func TestBuildRejectsForeignProviderRosterDrift(t *testing.T) {
	catalog := providerPlanCatalog(t)
	tests := []struct {
		name   string
		mutate func([]membergenerator.Metadata)
	}{
		{name: "missing-axis", mutate: func(roster []membergenerator.Metadata) { roster[0].Axis = "value/missing" }},
		{name: "missing-member", mutate: func(roster []membergenerator.Metadata) { roster[1].Relations[0].Key = "value/other-candidates" }},
		{name: "wrong-carrier", mutate: func(roster []membergenerator.Metadata) {
			roster[0].Relations[0].Subject = providerType("example/value", "WrongCandidate")
		}},
		{name: "ownership-drift", mutate: func(roster []membergenerator.Metadata) {
			roster[0].Relations[0].CandidateProvider.AxisRelation.Member = "value/other-candidates"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			roster := []membergenerator.Metadata{providerConsumerMetadata(), providerDirectoryMetadata()}
			test.mutate(roster)
			if _, err := Build(roster, catalog); err == nil {
				t.Fatal("provider roster drift admitted")
			}
		})
	}
}
