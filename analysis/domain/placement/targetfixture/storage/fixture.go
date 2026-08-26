// Package storage supplies the family-owned target specimen for Placement
// Storage. It receives real owner-issued Value payloads solely from the
// canonical domain relation fixture; all target schema, population, seed,
// mount, solve, and snapshot work goes through targetfixture.Build.
package storage

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
	"github.com/wippyai/go-lua/analysis/schema/structure"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementstore "github.com/wippyai/go-lua/domain/placement/store"
	"github.com/wippyai/go-lua/domain/relationfixture"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const fixtureDomain = "analysis/engine/relation/runtime/testdata/targetfixture/placement/storage/v1"

// Fixture holds only Storage's typed query contract beside the generic target
// world. The generic kit owns no domain codec, concrete Fact, or source value.
type Fixture struct {
	world     targetfixture.World
	output    model.ColumnID
	placement *relbindgen.Column[placementdomain.Fact]
	expected  placementdomain.Fact
}

// New builds the declared Storage family through targetfixture.Build. The
// relationfixture world is an owner-authenticated payload factory only: it is
// never used as a target declaration, mounted witness, geometry, root, or
// snapshot authority.
func New(t targetfixture.Probe) Fixture {
	t.Helper()
	source, candidate := ownerValues(t)
	ids := newIDs(t)
	codecs := newCodecs(t, ids)
	seed, operation, declaration, input, output := newDeclaration(t, ids)
	columns, ok := placementrelation.NewPlacementStorageColumns(codecs.value, codecs.placement, codecs.candidate, codecs.routeTag)
	if !ok {
		t.Fatal("storage typed binding columns")
	}
	factory, ok := placementrelation.BindPlacementStorage(
		operation,
		placementrelation.PlacementStorageOperation{},
		columns,
		ids.identity.Refusal(t, "storage"),
	)
	if !ok {
		t.Fatal("storage typed binding")
	}
	expected, reduction := placementstore.StorageFold(candidate, source, 1, placementdomain.DefaultFact())
	if reduction != structure.Concrete {
		t.Fatal("storage owner fold")
	}
	world := targetfixture.Build(t, targetfixture.Spec{
		Identity:    ids.identity,
		Declaration: declaration,
		Bindings:    []binding.Factory{factory},
		Populations: []targetfixture.Population{
			{Denominator: input, Rows: []model.RowID{ids.inputRow}},
			{Denominator: output, Rows: []model.RowID{ids.outputRow}},
		},
		Scopes: []targetfixture.Scope{{ID: ids.scope, Region: "storage"}},
		Initials: []targetfixture.Initial{{
			Operation: seed,
			Scope:     ids.scope,
			Cells: func(issuer binding.Issuer) ([]targetfixture.Cell, bool) {
				address, addressOK := codecs.coordinate.Encode(issuer, ids.address)
				candidateValue, candidateOK := codecs.candidate.Encode(issuer, candidate)
				sourceValue, sourceOK := codecs.value.Encode(issuer, source)
				route, routeOK := codecs.routeTag.Encode(issuer, uint64(1))
				selected, selectedOK := codecs.placement.Encode(issuer, placementdomain.DefaultFact())
				if !addressOK || !candidateOK || !sourceOK || !routeOK || !selectedOK {
					return nil, false
				}
				addressCell, addressCellOK := targetfixture.Opaque(input, ids.inputRow, ids.inputAddress, address)
				candidateCell, candidateCellOK := targetfixture.Opaque(input, ids.inputRow, ids.candidate, candidateValue)
				sourceCell, sourceCellOK := targetfixture.Opaque(input, ids.inputRow, ids.source, sourceValue)
				routeCell, routeCellOK := targetfixture.Opaque(input, ids.inputRow, ids.routeTag, route)
				selectedCell, selectedCellOK := targetfixture.Present(input, ids.inputRow, ids.selected, selected)
				if !addressCellOK || !candidateCellOK || !sourceCellOK || !routeCellOK || !selectedCellOK {
					return nil, false
				}
				return []targetfixture.Cell{addressCell, candidateCell, sourceCell, routeCell, selectedCell}, true
			},
		}},
		Authorities: func(issuer binding.Issuer) (targetfixture.Registry, bool) {
			lattice, latticeOK := placementrelation.NewPlacementLattice()
			placementAlgebra, algebraOK := relbindgen.NewAlgebra[placementdomain.Fact, placementrelation.PlacementLattice](codecs.placement, issuer, lattice)
			coordinateEquality, equalityOK := relbindgen.NewEquality(codecs.coordinate, contentEquality{})
			if !latticeOK || !algebraOK || !equalityOK {
				return targetfixture.Registry{}, false
			}
			return targetfixture.Registry{Algebras: []binding.ValueAlgebra{placementAlgebra}, Equalities: []binding.ValueEquality{coordinateEquality}}, true
		},
		MountByte: 0xF3,
	})
	return Fixture{world: world, output: ids.outputFact, placement: codecs.placement, expected: expected}
}

// Solve executes Storage through the production target relation runtime.
func (value Fixture) Solve() (terminal.Result, bool) { return value.world.Solve() }

// Facts redeems Storage's output through the canonical typed Placement query.
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

func (value Fixture) Expected() placementdomain.Fact { return value.expected }

type ids struct {
	identity targetfixture.Identity
	schema   model.SchemaID

	coordinateType model.TypeID
	candidateType  model.TypeID
	valueType      model.TypeID
	routeType      model.TypeID
	placementType  model.TypeID
	scope          model.ScopeID

	input  model.RelationID
	output model.RelationID

	inputAddress  model.ColumnID
	candidate     model.ColumnID
	source        model.ColumnID
	routeTag      model.ColumnID
	selected      model.ColumnID
	outputAddress model.ColumnID
	outputFact    model.ColumnID

	inputKey  model.KeyID
	outputKey model.KeyID
	inputRow  model.RowID
	outputRow model.RowID
	address   identity.ContentID

	seed       model.OperationID
	operation  model.OperationID
	expression model.ExpressionID
	dependency model.DependencyID
}

func newIDs(t targetfixture.Probe) ids {
	t.Helper()
	identity := targetfixture.NewIdentity(t, fixtureDomain)
	value := ids{
		identity:       identity,
		schema:         identity.Schema(t, "storage"),
		coordinateType: identity.Type(t, "coordinate"),
		candidateType:  identity.Type(t, "storage-transfer"),
		valueType:      identity.Type(t, "value"),
		routeType:      identity.Type(t, "route-tag"),
		placementType:  identity.Type(t, "placement-fact"),
		scope:          identity.Scope(t, "storage"),
		input:          identity.Relation(t, "storage-input"),
		output:         identity.Relation(t, "placement-output"),
		seed:           identity.Operation(t, "seed"),
		operation:      identity.Operation(t, "storage"),
		expression:     identity.Expression(t, "storage"),
		dependency:     identity.Dependency(t, "storage"),
	}
	value.inputAddress = identity.Column(t, value.input, "address")
	value.candidate = identity.Column(t, value.input, "candidate")
	value.source = identity.Column(t, value.input, "source")
	value.routeTag = identity.Column(t, value.input, "route-tag")
	value.selected = identity.Column(t, value.input, "selected")
	value.outputAddress = identity.Column(t, value.output, "address")
	value.outputFact = identity.Column(t, value.output, "fact")
	value.inputKey = identity.Key(t, value.input, "address")
	value.outputKey = identity.Key(t, value.output, "address")
	value.address = content(t, identity, "row/storage")
	value.inputRow = identity.RowFromContent(t, value.input, value.address)
	value.outputRow = identity.RowFromContent(t, value.output, value.address)
	return value
}

type codecs struct {
	coordinate *relbindgen.Column[identity.ContentID]
	candidate  *relbindgen.Column[valuedomain.StorageTransfer]
	value      *relbindgen.Column[valuedomain.Value]
	routeTag   *relbindgen.Column[uint64]
	placement  *relbindgen.Column[placementdomain.Fact]
}

func newCodecs(t targetfixture.Probe, ids ids) codecs {
	t.Helper()
	coordinateStore, ok := relbindgen.NewStore[identity.ContentID](content(t, ids.identity, "codec/coordinate"), 1)
	if !ok {
		t.Fatal("storage coordinate store")
	}
	coordinate, ok := relbindgen.NewColumn(ids.coordinateType, coordinateStore)
	if !ok {
		t.Fatal("storage coordinate column")
	}
	candidateStore, ok := relbindgen.NewStore[valuedomain.StorageTransfer](content(t, ids.identity, "codec/candidate"), 1)
	if !ok {
		t.Fatal("storage candidate store")
	}
	candidate, ok := relbindgen.NewColumn(ids.candidateType, candidateStore)
	if !ok {
		t.Fatal("storage candidate column")
	}
	valueStore, ok := relbindgen.NewStore[valuedomain.Value](content(t, ids.identity, "codec/value"), 1)
	if !ok {
		t.Fatal("storage value store")
	}
	value, ok := relbindgen.NewColumn(ids.valueType, valueStore)
	if !ok {
		t.Fatal("storage value column")
	}
	routeStore, ok := relbindgen.NewStore[uint64](content(t, ids.identity, "codec/route"), 1)
	if !ok {
		t.Fatal("storage route store")
	}
	routeTag, ok := relbindgen.NewColumn(ids.routeType, routeStore)
	if !ok {
		t.Fatal("storage route column")
	}
	placementStore, ok := relbindgen.NewStore[placementdomain.Fact](content(t, ids.identity, "codec/placement"), 1)
	if !ok {
		t.Fatal("storage placement store")
	}
	placement, ok := relbindgen.NewColumn(ids.placementType, placementStore)
	if !ok {
		t.Fatal("storage placement column")
	}
	return codecs{coordinate: coordinate, candidate: candidate, value: value, routeTag: routeTag, placement: placement}
}

func newDeclaration(t targetfixture.Probe, ids ids) (signature.Signature, signature.Signature, relcompile.Declaration, model.DenominatorRef, model.DenominatorRef) {
	t.Helper()
	input, ok := model.NewDenominatorRef(ids.input, ids.inputKey)
	if !ok {
		t.Fatal("storage input denominator")
	}
	output, ok := model.NewDenominatorRef(ids.output, ids.outputKey)
	if !ok {
		t.Fatal("storage output denominator")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("storage cardinality")
	}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	if !ok {
		t.Fatal("storage outcomes")
	}
	scalar, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("storage scalar delivery")
	}
	seed := seal(t, ids, ids.seed, nil, []signature.Output{
		{Relation: ids.input, Column: ids.inputAddress, Type: ids.coordinateType, Presence: signature.ProduceOpaque, Denominator: input},
		{Relation: ids.input, Column: ids.candidate, Type: ids.candidateType, Presence: signature.ProduceOpaque, Denominator: input},
		{Relation: ids.input, Column: ids.source, Type: ids.valueType, Presence: signature.ProduceOpaque, Denominator: input},
		{Relation: ids.input, Column: ids.routeTag, Type: ids.routeType, Presence: signature.ProduceOpaque, Denominator: input},
		{Relation: ids.input, Column: ids.selected, Type: ids.placementType, Presence: signature.ProducePresent, Denominator: input},
	}, cardinality, outcomes)
	operation := seal(t, ids, ids.operation, []signature.Input{
		{Relation: ids.input, Column: ids.candidate, Type: ids.candidateType, Presence: signature.RequireOpaque, Delivery: scalar, Denominator: input},
		{Relation: ids.input, Column: ids.source, Type: ids.valueType, Presence: signature.RequireOpaque, Delivery: scalar, Denominator: input},
		{Relation: ids.input, Column: ids.routeTag, Type: ids.routeType, Presence: signature.RequireOpaque, Delivery: scalar, Denominator: input},
		{Relation: ids.input, Column: ids.selected, Type: ids.placementType, Presence: signature.RequirePresent, Delivery: scalar, Denominator: input},
	}, []signature.Output{{Relation: ids.output, Column: ids.outputFact, Type: ids.placementType, Presence: signature.ProducePresent, Denominator: output}}, cardinality, outcomes)
	coordinateCapability, ok := model.NewEquatableCapability(ids.coordinateType)
	if !ok {
		t.Fatal("storage coordinate capability")
	}
	candidateCapability, ok := model.NewDecodeOnlyCapability(ids.candidateType)
	if !ok {
		t.Fatal("storage candidate capability")
	}
	valueCapability, ok := model.NewDecodeOnlyCapability(ids.valueType)
	if !ok {
		t.Fatal("storage value capability")
	}
	routeCapability, ok := model.NewDecodeOnlyCapability(ids.routeType)
	if !ok {
		t.Fatal("storage route capability")
	}
	placementCapability, ok := model.NewAscendingCapability(ids.placementType)
	if !ok {
		t.Fatal("storage placement capability")
	}
	declaration := relcompile.Declaration{
		SchemaID: ids.schema,
		Relations: []model.RelationSchema{
			model.DefineRelationSchema(ids.input, []model.ColumnID{ids.inputAddress, ids.candidate, ids.source, ids.routeTag, ids.selected}, []model.KeyID{ids.inputKey}, ids.scope),
			model.DefineRelationSchema(ids.output, []model.ColumnID{ids.outputAddress, ids.outputFact}, []model.KeyID{ids.outputKey}, ids.scope),
		},
		Columns: []model.ColumnSchema{
			model.DefineColumnSchema(ids.inputAddress, ids.coordinateType),
			model.DefineColumnSchema(ids.candidate, ids.candidateType),
			model.DefineColumnSchema(ids.source, ids.valueType),
			model.DefineColumnSchema(ids.routeTag, ids.routeType),
			model.DefineColumnSchema(ids.selected, ids.placementType),
			model.DefineColumnSchema(ids.outputAddress, ids.coordinateType),
			model.DefineColumnSchema(ids.outputFact, ids.placementType),
		},
		TypeCapabilities: []model.TypeCapability{coordinateCapability, candidateCapability, valueCapability, routeCapability, placementCapability},
		Keys: []model.KeySchema{
			model.DefineKeySchema(ids.inputKey, []model.ColumnID{ids.inputAddress}),
			model.DefineKeySchema(ids.outputKey, []model.ColumnID{ids.outputAddress}),
		},
		Scopes:     []model.ScopeSchema{model.DefineScopeSchema(ids.scope, nil, region.True())},
		Signatures: []signature.Signature{seed, operation},
		Rules: []relcompile.Rule{{
			ID:         ids.dependency,
			Expression: ids.expression,
			Candidate:  ids.input,
			ApplySlots: []relcompile.ReadOccurrence{relcompile.CandidateOccurrence(), relcompile.CandidateOccurrence(), relcompile.CandidateOccurrence(), relcompile.CandidateOccurrence()},
			Scope:      ids.scope,
			Apply:      operation.Identity(),
			Output:     algebra.ScalarSource(algebra.NewSlotSource(0, 4)),
			Publish:    &relcompile.Publication{Relation: ids.output, Key: ids.outputKey, Columns: []model.ColumnID{ids.outputFact}},
		}},
	}
	return seed, operation, declaration, input, output
}

func seal(t targetfixture.Probe, ids ids, operation model.OperationID, inputs []signature.Input, outputs []signature.Output, cardinality model.Cardinality, outcomes outcome.Set) signature.Signature {
	t.Helper()
	value, ok := signature.Seal(signature.Spec{
		Identity:    signature.Identity{Operation: operation, Version: 1},
		Fence:       signature.Fence{Owner: ids.identity.Owner(), Schema: ids.schema},
		Inputs:      inputs,
		Outputs:     outputs,
		Cardinality: cardinality,
		Outcomes:    outcomes,
	})
	if !ok || !value.Available() {
		t.Fatal("storage signature")
	}
	return value
}

type contentEquality struct{}

func (contentEquality) Equal(left, right identity.ContentID) bool { return left == right }

func content(t targetfixture.Probe, issuer targetfixture.Identity, label string) identity.ContentID {
	t.Helper()
	value, ok := issuer.Content(label)
	if !ok {
		t.Fatalf("storage content %q", label)
	}
	return value
}

func ownerValues(t targetfixture.Probe) (valuedomain.Value, valuedomain.StorageTransfer) {
	t.Helper()
	world := relationfixture.New(t)
	if world.Values == nil || !world.Values.Valid() {
		t.Fatal("storage owner payload factory")
	}
	for index := 0; index < world.Values.StorageTransferCount(); index++ {
		candidate, ok := world.Values.StorageTransferAt(index)
		if ok && candidate.Persistent() {
			return world.Receiver, candidate
		}
	}
	t.Fatal("storage owner payload has no persistent transfer")
	return valuedomain.Value{}, valuedomain.StorageTransfer{}
}
