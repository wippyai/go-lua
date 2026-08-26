// Package scalar supplies the shared target-runtime specimens for Placement's
// scalar route families. The package owns declaration data, codecs, and typed
// binding handoff; targetfixture owns every generic compile/check/mount/
// geometry/bootstrap operation.
package scalar

import (
	placementrelation "github.com/wippyai/go-lua/analysis/domain/placement/relation"
	placementquery "github.com/wippyai/go-lua/analysis/domain/placement/relation/query"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/terminal"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/testdata/targetfixture"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// Probe is the narrow test failure surface accepted by all scalar fixture
// constructors. It is an alias so external probes can depend on this package
// without importing the generic kit solely for the argument type.
type Probe = targetfixture.Probe

// Family selects one Placement scalar route declaration and its typed binding.
type Family uint8

const (
	FamilyPublicationEscape Family = iota + 1
	FamilyReturnEscape
	FamilyTransfer
)

// Fixture is one fully built target specimen. It retains only the generic
// target World and the owning Fact query contract; callers cannot access a
// second geometry, database, or runtime authority.
type Fixture struct {
	world     targetfixture.World
	output    model.ColumnID
	placement *relbindgen.Column[placementdomain.Fact]
	expected  placementdomain.Fact
}

// New builds one scalar family through targetfixture.Build.
func New(t Probe, family Family) Fixture {
	t.Helper()
	config, ok := familyConfigFor(family)
	if !ok {
		t.Fatalf("unknown scalar family %d", family)
		return Fixture{}
	}
	specimen := newScalarFixture(t, config)
	factory := bindFactory(t, specimen, family)
	world := targetfixture.Build(t, specimen.spec(t, factory))
	return Fixture{world: world, output: specimen.ids.outputFact, placement: specimen.fact, expected: specimen.expected}
}

// NewPublicationEscape builds the PublicationEscape scalar specimen.
func NewPublicationEscape(t Probe) Fixture { return New(t, FamilyPublicationEscape) }

// NewReturnEscape builds the ReturnEscape scalar specimen.
func NewReturnEscape(t Probe) Fixture { return New(t, FamilyReturnEscape) }

// NewTransfer builds the Transfer scalar specimen.
func NewTransfer(t Probe) Fixture { return New(t, FamilyTransfer) }

// PublicationEscape is the concise family constructor alias.
func PublicationEscape(t Probe) Fixture { return NewPublicationEscape(t) }

// ReturnEscape is the concise family constructor alias.
func ReturnEscape(t Probe) Fixture { return NewReturnEscape(t) }

// Transfer is the concise family constructor alias.
func Transfer(t Probe) Fixture { return NewTransfer(t) }

// Solve executes the scalar specimen through the production target relation
// runtime.
func (value Fixture) Solve() (terminal.Result, bool) { return value.world.Solve() }

// Facts redeems the scalar output through Placement's canonical typed query.
func (value Fixture) Facts(result terminal.Result) (placementquery.Rows, bool) {
	if value.placement == nil || !value.placement.Available() {
		return placementquery.Rows{}, false
	}
	projection, ok := value.world.Snapshot(result)
	if !ok || !projection.Available() {
		return placementquery.Rows{}, false
	}
	column, ok := placementquery.NewFactColumn(value.output, value.placement)
	if !ok {
		return placementquery.Rows{}, false
	}
	return placementquery.Read(projection, column)
}

// Expected returns the owner-computed Fact expected from this family route.
func (value Fixture) Expected() placementdomain.Fact { return value.expected }

type familyConfig struct {
	domain      string
	payload     payloadKind
	mountByte   byte
	requirement placementdomain.Placement
	routeTag    uint64
	escape      placementdomain.Escape
}

func familyConfigFor(family Family) (familyConfig, bool) {
	switch family {
	case FamilyPublicationEscape:
		return familyConfig{
			domain:  "placement-publication-escape",
			payload: payloadRequirement, mountByte: 0x91,
			requirement: placementdomain.OwnedHeap, escape: placementdomain.Retain,
		}, true
	case FamilyReturnEscape:
		return familyConfig{
			domain:  "placement-return-escape",
			payload: payloadRouteTag, mountByte: 0x92,
			requirement: placementdomain.Bottom, routeTag: 1, escape: placementdomain.Return,
		}, true
	case FamilyTransfer:
		return familyConfig{
			domain:  "placement-transfer",
			payload: payloadRouteTag, mountByte: 0x93,
			requirement: placementdomain.Bottom,
			routeTag:    uint64(1)<<4 | uint64(placementdomain.Send+1), escape: placementdomain.Send,
		}, true
	default:
		return familyConfig{}, false
	}
}

func bindFactory(t Probe, fixture *scalarFixture, family Family) binding.Factory {
	t.Helper()
	switch family {
	case FamilyPublicationEscape:
		columns, ok := placementrelation.NewPlacementPublicationEscapeColumns(fixture.fact, fixture.requirement)
		if !ok {
			t.Fatal("publication escape columns")
		}
		factory, ok := placementrelation.BindPlacementPublicationEscape(
			fixture.operation,
			placementrelation.PlacementPublicationEscapeOperation{},
			columns,
			fixture.identity.Refusal(t, "publication"),
		)
		if !ok {
			t.Fatal("publication escape binding")
		}
		return factory
	case FamilyReturnEscape:
		columns, ok := placementrelation.NewPlacementReturnEscapeColumns(fixture.fact, fixture.routeTag)
		if !ok {
			t.Fatal("return escape columns")
		}
		factory, ok := placementrelation.BindPlacementReturnEscape(
			fixture.operation,
			placementrelation.PlacementReturnEscapeOperation{},
			columns,
			fixture.identity.Refusal(t, "return"),
		)
		if !ok {
			t.Fatal("return escape binding")
		}
		return factory
	case FamilyTransfer:
		columns, ok := placementrelation.NewPlacementTransferColumns(fixture.fact, fixture.routeTag)
		if !ok {
			t.Fatal("transfer columns")
		}
		factory, ok := placementrelation.BindPlacementTransfer(
			fixture.operation,
			placementrelation.PlacementTransferOperation{},
			columns,
			fixture.identity.Refusal(t, "transfer"),
		)
		if !ok {
			t.Fatal("transfer binding")
		}
		return factory
	default:
		t.Fatalf("unknown scalar family %d", family)
		return nil
	}
}

type payloadKind uint8

const (
	payloadRequirement payloadKind = iota + 1
	payloadRouteTag
)

// scalarFixture is family-owned specimen state.  It contains no mounted
// address, database root, geometry, or runtime worker; those are all returned
// by targetfixture.Build.
type scalarFixture struct {
	identity targetfixture.Identity
	ids      scalarIDs

	payload payloadKind

	coordinateCandidate *relbindgen.Column[identity.ContentID]
	coordinatePlacement *relbindgen.Column[identity.ContentID]
	fact                *relbindgen.Column[placementdomain.Fact]
	requirement         *relbindgen.Column[placementdomain.Placement]
	routeTag            *relbindgen.Column[uint64]

	declaration      relcompile.Declaration
	operation        signature.Signature
	seedCandidate    signature.Signature
	seedPlacement    signature.Signature
	candidateDenom   model.DenominatorRef
	placementDenom   model.DenominatorRef
	outputDenom      model.DenominatorRef
	candidateRow     model.RowID
	placementRow     model.RowID
	outputRow        model.RowID
	address          identity.ContentID
	mountByte        byte
	requirementValue placementdomain.Placement
	routeValue       uint64
	expected         placementdomain.Fact
}

type scalarIDs struct {
	schema model.SchemaID
	scope  model.ScopeID

	coordinateType  model.TypeID
	factType        model.TypeID
	requirementType model.TypeID
	routeTagType    model.TypeID

	candidate         model.RelationID
	placement         model.RelationID
	output            model.RelationID
	candidateAddress  model.ColumnID
	candidatePayload  model.ColumnID
	candidateRouteTag model.ColumnID
	placementAddress  model.ColumnID
	placementFact     model.ColumnID
	outputAddress     model.ColumnID
	outputFact        model.ColumnID
	candidateKey      model.KeyID
	placementKey      model.KeyID
	outputKey         model.KeyID

	operation     model.OperationID
	seedCandidate model.OperationID
	seedPlacement model.OperationID
	expression    model.ExpressionID
	dependency    model.DependencyID
}

type factLattice struct{}

func (factLattice) Join(left, right placementdomain.Fact) (placementdomain.Fact, bool) {
	return placementdomain.JoinFactChecked(left, right)
}

func (factLattice) Widen(left, right placementdomain.Fact) (placementdomain.Fact, bool) {
	return placementdomain.JoinFactChecked(left, right)
}

func (factLattice) LessOrEq(left, right placementdomain.Fact) bool {
	return placementdomain.LessOrEqFact(left, right)
}

type contentEquality struct{}

func (contentEquality) Equal(left, right identity.ContentID) bool { return left == right }

func newScalarFixture(t targetfixture.Probe, config familyConfig) *scalarFixture {
	t.Helper()
	owner := targetfixture.NewIdentity(t, config.domain)
	ids := scalarIDs{
		schema:          owner.Schema(t, "schema"),
		scope:           owner.Scope(t, "scope"),
		coordinateType:  owner.Type(t, "coordinate"),
		factType:        owner.Type(t, "fact"),
		requirementType: owner.Type(t, "requirement"),
		routeTagType:    owner.Type(t, "route-tag"),
		candidate:       owner.Relation(t, "candidate"),
		placement:       owner.Relation(t, "placement"),
		output:          owner.Relation(t, "output"),
		operation:       owner.Operation(t, "route"),
		seedCandidate:   owner.Operation(t, "seed-candidate"),
		seedPlacement:   owner.Operation(t, "seed-placement"),
		expression:      owner.Expression(t, "route"),
		dependency:      owner.Dependency(t, "route"),
	}
	ids.candidateAddress = owner.Column(t, ids.candidate, "address")
	ids.candidatePayload = owner.Column(t, ids.candidate, "payload")
	ids.candidateRouteTag = owner.Column(t, ids.candidate, "route-tag")
	ids.placementAddress = owner.Column(t, ids.placement, "address")
	ids.placementFact = owner.Column(t, ids.placement, "fact")
	ids.outputAddress = owner.Column(t, ids.output, "address")
	ids.outputFact = owner.Column(t, ids.output, "fact")
	ids.candidateKey = owner.Key(t, ids.candidate, "address")
	ids.placementKey = owner.Key(t, ids.placement, "address")
	ids.outputKey = owner.Key(t, ids.output, "address")

	candidateDenom, ok := model.NewDenominatorRef(ids.candidate, ids.candidateKey)
	if !ok {
		t.Fatal("candidate denominator")
	}
	placementDenom, ok := model.NewDenominatorRef(ids.placement, ids.placementKey)
	if !ok {
		t.Fatal("placement denominator")
	}
	outputDenom, ok := model.NewDenominatorRef(ids.output, ids.outputKey)
	if !ok {
		t.Fatal("output denominator")
	}
	address, ok := owner.Content("address/root")
	if !ok {
		t.Fatal("address content")
	}
	candidateRow := owner.RowFromContent(t, ids.candidate, address)
	placementRow := owner.RowFromContent(t, ids.placement, address)
	outputRow := owner.RowFromContent(t, ids.output, address)

	coordStore, ok := relbindgen.NewStore[identity.ContentID](mustContent(t, owner, "store/coordinate"), 4)
	if !ok {
		t.Fatal("coordinate store")
	}
	coordinateCandidate, ok := relbindgen.NewColumn(ids.coordinateType, coordStore)
	if !ok {
		t.Fatal("candidate coordinate column")
	}
	coordinatePlacement, ok := relbindgen.NewColumn(ids.coordinateType, coordStore)
	if !ok {
		t.Fatal("placement coordinate column")
	}
	factStore, ok := relbindgen.NewStore[placementdomain.Fact](mustContent(t, owner, "store/fact"), 4)
	if !ok {
		t.Fatal("fact store")
	}
	fact, ok := relbindgen.NewColumn(ids.factType, factStore)
	if !ok {
		t.Fatal("fact column")
	}
	requirementStore, ok := relbindgen.NewStore[placementdomain.Placement](mustContent(t, owner, "store/requirement"), 2)
	if !ok {
		t.Fatal("requirement store")
	}
	requirementColumn, ok := relbindgen.NewColumn(ids.requirementType, requirementStore)
	if !ok {
		t.Fatal("requirement column")
	}
	routeStore, ok := relbindgen.NewStore[uint64](mustContent(t, owner, "store/route-tag"), 2)
	if !ok {
		t.Fatal("route tag store")
	}
	routeColumn, ok := relbindgen.NewColumn(ids.routeTagType, routeStore)
	if !ok {
		t.Fatal("route tag column")
	}

	seedCandidateOutputs := []signature.Output{
		{Relation: ids.candidate, Column: ids.candidateAddress, Type: ids.coordinateType, Presence: signature.ProduceOpaque, Denominator: candidateDenom},
	}
	inputType := ids.requirementType
	inputColumn := ids.candidatePayload
	if config.payload == payloadRouteTag {
		seedCandidateOutputs = append(seedCandidateOutputs, signature.Output{Relation: ids.candidate, Column: ids.candidateRouteTag, Type: ids.routeTagType, Presence: signature.ProduceOpaque, Denominator: candidateDenom})
		inputType = ids.routeTagType
		inputColumn = ids.candidateRouteTag
	} else {
		seedCandidateOutputs = append(seedCandidateOutputs, signature.Output{Relation: ids.candidate, Column: ids.candidatePayload, Type: ids.requirementType, Presence: signature.ProduceOpaque, Denominator: candidateDenom})
	}
	seedCandidate := seal(t, owner.Owner(), ids.schema, ids.seedCandidate, nil, seedCandidateOutputs)
	seedPlacement := seal(t, owner.Owner(), ids.schema, ids.seedPlacement, nil, []signature.Output{
		{Relation: ids.placement, Column: ids.placementAddress, Type: ids.coordinateType, Presence: signature.ProduceOpaque, Denominator: placementDenom},
		{Relation: ids.placement, Column: ids.placementFact, Type: ids.factType, Presence: signature.ProducePresent, Denominator: placementDenom},
	})
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	operation := seal(t, owner.Owner(), ids.schema, ids.operation, []signature.Input{
		{Relation: ids.candidate, Column: inputColumn, Type: inputType, Presence: signature.RequireOpaque, Delivery: delivery, Denominator: candidateDenom},
		{Relation: ids.placement, Column: ids.placementFact, Type: ids.factType, Presence: signature.RequirePresent, Delivery: delivery, Denominator: placementDenom},
	}, []signature.Output{{Relation: ids.output, Column: ids.outputFact, Type: ids.factType, Presence: signature.ProducePresent, Denominator: outputDenom}})

	coordinateCapability, ok := model.NewEquatableCapability(ids.coordinateType)
	if !ok {
		t.Fatal("coordinate capability")
	}
	factCapability, ok := model.NewAscendingCapability(ids.factType)
	if !ok {
		t.Fatal("fact capability")
	}
	requirementCapability, ok := model.NewDecodeOnlyCapability(ids.requirementType)
	if !ok {
		t.Fatal("requirement capability")
	}
	routeCapability, ok := model.NewDecodeOnlyCapability(ids.routeTagType)
	if !ok {
		t.Fatal("route tag capability")
	}
	candidateColumns := []model.ColumnID{ids.candidateAddress}
	if config.payload == payloadRequirement {
		candidateColumns = append(candidateColumns, ids.candidatePayload)
	} else {
		candidateColumns = append(candidateColumns, ids.candidateRouteTag)
	}
	candidateColumnSchemas := []model.ColumnSchema{model.DefineColumnSchema(ids.candidateAddress, ids.coordinateType)}
	if config.payload == payloadRequirement {
		candidateColumnSchemas = append(candidateColumnSchemas, model.DefineColumnSchema(ids.candidatePayload, ids.requirementType))
	} else {
		candidateColumnSchemas = append(candidateColumnSchemas, model.DefineColumnSchema(ids.candidateRouteTag, ids.routeTagType))
	}
	declaration := relcompile.Declaration{
		SchemaID: ids.schema,
		Relations: []model.RelationSchema{
			model.DefineRelationSchema(ids.candidate, candidateColumns, []model.KeyID{ids.candidateKey}, ids.scope),
			model.DefineRelationSchema(ids.placement, []model.ColumnID{ids.placementAddress, ids.placementFact}, []model.KeyID{ids.placementKey}, ids.scope),
			model.DefineRelationSchema(ids.output, []model.ColumnID{ids.outputAddress, ids.outputFact}, []model.KeyID{ids.outputKey}, ids.scope),
		},
		Columns: append(candidateColumnSchemas, []model.ColumnSchema{
			model.DefineColumnSchema(ids.placementAddress, ids.coordinateType),
			model.DefineColumnSchema(ids.placementFact, ids.factType),
			model.DefineColumnSchema(ids.outputAddress, ids.coordinateType),
			model.DefineColumnSchema(ids.outputFact, ids.factType),
		}...),
		TypeCapabilities: []model.TypeCapability{coordinateCapability, factCapability, requirementCapability, routeCapability},
		Keys: []model.KeySchema{
			model.DefineKeySchema(ids.candidateKey, []model.ColumnID{ids.candidateAddress}),
			model.DefineKeySchema(ids.placementKey, []model.ColumnID{ids.placementAddress}),
			model.DefineKeySchema(ids.outputKey, []model.ColumnID{ids.outputAddress}),
		},
		Scopes:     []model.ScopeSchema{model.DefineScopeSchema(ids.scope, nil, region.True())},
		Signatures: []signature.Signature{seedCandidate, seedPlacement, operation},
		Rules: []relcompile.Rule{{
			ID: ids.dependency, Expression: ids.expression, Candidate: ids.candidate,
			Joins:      []relcompile.JoinSpec{{Relation: ids.placement, LeftColumns: []model.ColumnID{ids.candidateAddress}, RightColumns: []model.ColumnID{ids.placementAddress}, Scope: ids.scope}},
			ApplySlots: []relcompile.ReadOccurrence{relcompile.CandidateOccurrence(), relcompile.JoinOccurrence(0)},
			Scope:      ids.scope,
			Apply:      operation.Identity(),
			Output:     algebra.ScalarSource(algebra.NewSlotSource(0, 3)),
			Publish:    &relcompile.Publication{Relation: ids.output, Key: ids.outputKey, Columns: []model.ColumnID{ids.outputFact}},
		}},
	}

	expected, ok := placementdomain.DisplaceFactChecked(placementdomain.DefaultFact(), config.escape)
	if !ok {
		t.Fatal("expected placement displacement")
	}
	return &scalarFixture{
		identity: owner, ids: ids, payload: config.payload,
		coordinateCandidate: coordinateCandidate, coordinatePlacement: coordinatePlacement,
		fact: fact, requirement: requirementColumn, routeTag: routeColumn,
		declaration: declaration, operation: operation, seedCandidate: seedCandidate, seedPlacement: seedPlacement,
		candidateDenom: candidateDenom, placementDenom: placementDenom,
		outputDenom: outputDenom, candidateRow: candidateRow, placementRow: placementRow, outputRow: outputRow, address: address, mountByte: config.mountByte,
		requirementValue: config.requirement, routeValue: config.routeTag, expected: expected,
	}
}

func seal(t targetfixture.Probe, owner model.OwnerID, schema model.SchemaID, operation model.OperationID, inputs []signature.Input, outputs []signature.Output) signature.Signature {
	t.Helper()
	codes, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	if !ok {
		t.Fatal("target scalar outcomes")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("target scalar cardinality")
	}
	value, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operation, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schema},
		Inputs:   inputs, Outputs: outputs,
		Cardinality: cardinality,
		Outcomes:    codes,
	})
	if !ok {
		t.Fatal("target scalar signature")
	}
	return value
}

func (fixture *scalarFixture) spec(t targetfixture.Probe, factories ...binding.Factory) targetfixture.Spec {
	t.Helper()
	return targetfixture.Spec{
		Identity:    fixture.identity,
		Declaration: fixture.declaration,
		Bindings:    factories,
		Populations: []targetfixture.Population{
			{Denominator: fixture.candidateDenom, Rows: []model.RowID{fixture.candidateRow}},
			{Denominator: fixture.placementDenom, Rows: []model.RowID{fixture.placementRow}},
			{Denominator: fixture.outputDenom, Rows: []model.RowID{fixture.outputRow}},
		},
		Scopes:      []targetfixture.Scope{{ID: fixture.ids.scope, Region: "main"}},
		Initials:    []targetfixture.Initial{fixture.candidateInitial(t), fixture.placementInitial(t)},
		Authorities: fixture.authorities,
		MountByte:   fixture.mountByte,
	}
}

func (fixture *scalarFixture) candidateInitial(t targetfixture.Probe) targetfixture.Initial {
	t.Helper()
	return targetfixture.Initial{Operation: fixture.seedCandidate, Scope: fixture.ids.scope, Cells: func(issuer binding.Issuer) ([]targetfixture.Cell, bool) {
		address, ok := fixture.coordinateCandidate.Encode(issuer, fixture.address)
		if !ok {
			return nil, false
		}
		addressCell, ok := opaqueCell(fixture.candidateDenom, fixture.candidateRow, fixture.ids.candidateAddress, address)
		if !ok {
			return nil, false
		}
		payloadColumn := fixture.ids.candidatePayload
		var payload binding.ValueToken
		if fixture.payload == payloadRequirement {
			payload, ok = fixture.requirement.Encode(issuer, fixture.requirementValue)
		} else {
			payloadColumn = fixture.ids.candidateRouteTag
			payload, ok = fixture.routeTag.Encode(issuer, fixture.routeValue)
		}
		if !ok {
			return nil, false
		}
		payloadCell, ok := opaqueCell(fixture.candidateDenom, fixture.candidateRow, payloadColumn, payload)
		if !ok {
			return nil, false
		}
		return []targetfixture.Cell{addressCell, payloadCell}, true
	}}
}

func (fixture *scalarFixture) placementInitial(t targetfixture.Probe) targetfixture.Initial {
	t.Helper()
	return targetfixture.Initial{Operation: fixture.seedPlacement, Scope: fixture.ids.scope, Cells: func(issuer binding.Issuer) ([]targetfixture.Cell, bool) {
		address, ok := fixture.coordinatePlacement.Encode(issuer, fixture.address)
		if !ok {
			return nil, false
		}
		fact, ok := fixture.fact.Encode(issuer, placementdomain.DefaultFact())
		if !ok {
			return nil, false
		}
		addressCell, ok := opaqueCell(fixture.placementDenom, fixture.placementRow, fixture.ids.placementAddress, address)
		if !ok {
			return nil, false
		}
		factCell, ok := targetfixture.Present(fixture.placementDenom, fixture.placementRow, fixture.ids.placementFact, fact)
		if !ok {
			return nil, false
		}
		return []targetfixture.Cell{addressCell, factCell}, true
	}}
}

func (fixture *scalarFixture) authorities(issuer binding.Issuer) (targetfixture.Registry, bool) {
	factAlgebra, ok := relbindgen.NewAlgebra(fixture.fact, issuer, factLattice{})
	if !ok {
		return targetfixture.Registry{}, false
	}
	coordinateEquality, ok := relbindgen.NewEquality(fixture.coordinateCandidate, contentEquality{})
	if !ok {
		return targetfixture.Registry{}, false
	}
	return targetfixture.Registry{Algebras: []binding.ValueAlgebra{factAlgebra}, Equalities: []binding.ValueEquality{coordinateEquality}}, true
}

func mustContent(t targetfixture.Probe, owner targetfixture.Identity, label string) identity.ContentID {
	t.Helper()
	value, ok := owner.Content(label)
	if !ok {
		t.Fatalf("target scalar content %q", label)
	}
	return value
}

func opaqueCell(denominator model.DenominatorRef, row model.RowID, column model.ColumnID, value binding.ValueToken) (targetfixture.Cell, bool) {
	presence, ok := model.NewPresence(model.AuthenticatedOpaque)
	if !ok {
		return targetfixture.Cell{}, false
	}
	return targetfixture.Cell{Denominator: denominator, Row: row, Column: column, Value: value, Presence: presence}, true
}
