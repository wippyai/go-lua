// Package allocationbirth supplies the AllocationBirth family declaration to
// the generic target-runtime fixture kit. It owns typed receipts and expected
// Placement output; targetfixture owns all runtime mechanics.
package allocationbirth

import (
	placementrelation "github.com/wippyai/go-lua/analysis/domain/placement/relation"
	placementquery "github.com/wippyai/go-lua/analysis/domain/placement/relation/query"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/terminal"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/testdata/targetfixture"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	realfixture "github.com/wippyai/go-lua/domain/relationfixture"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const fixtureDomain = "analysis/engine/relation/runtime/testdata/targetfixture/placement/allocationbirth/v1"

// IDs are the typed query surface consumers need after the family has been
// solved. They remain owner-issued logical identities, never dense addresses.
type IDs struct {
	PlacementOwner model.OwnerID
	Output         model.RelationID
	OutputPayload  model.ColumnID
}

// Fixture is AllocationBirth's typed data over one generic target-runtime
// World. It does not retain a legacy relation store or evaluator.
type Fixture struct {
	world     targetfixture.World
	ids       IDs
	placement *relbindgen.Column[placementdomain.Fact]
	expected  placementdomain.Fact
}

// New obtains one real Value-owned allocation receipt solely as the typed
// family seed, then runs it through the target relation declaration/mount/
// solve substrate. No legacy Placement rule or runtime result is consulted.
func New(t targetfixture.Probe, mountBytes ...byte) Fixture {
	t.Helper()
	if len(mountBytes) > 1 {
		t.Fatalf("allocationbirth fixture: at most one mount byte, got %d", len(mountBytes))
	}
	mountByte := byte(0xF3)
	if len(mountBytes) == 1 {
		mountByte = mountBytes[0]
	}
	real := realfixture.New(t)
	if real.Values == nil || real.Values.AllocationResultCount() == 0 {
		t.Fatal("allocationbirth real Value receipt")
	}
	candidate, ok := real.Values.AllocationResultAt(0)
	if !ok || candidate == nil || !candidate.Owns(real.Values) {
		t.Fatal("allocationbirth typed candidate")
	}
	coordinate, ok := candidate.Coordinate()
	if !ok {
		t.Fatal("allocationbirth candidate coordinate")
	}
	fresh, ok := candidate.Fresh()
	if !ok {
		t.Fatal("allocationbirth candidate fresh value")
	}
	rowContent, ok := candidate.KeyID()
	if !ok {
		t.Fatal("allocationbirth candidate row content")
	}
	ids := newIDs(t, rowContent)
	codecs := newCodecs(t, ids)
	seedCandidate, seedFacts, operation, declaration, candidates, facts, output := newDeclaration(t, ids)
	columns, ok := placementrelation.NewPlacementAllocationBirthColumns(codecs.value, codecs.placement, codecs.candidate)
	if !ok {
		t.Fatal("allocationbirth typed columns")
	}
	factory, ok := placementrelation.BindPlacementAllocationBirth(
		operation,
		placementrelation.PlacementAllocationBirthOperation{},
		columns,
		ids.identity.Refusal(t, "allocationbirth"),
	)
	if !ok {
		t.Fatal("allocationbirth typed binding")
	}
	world := targetfixture.Build(t, targetfixture.Spec{
		Identity:    ids.identity,
		Declaration: declaration,
		Bindings:    []binding.Factory{factory},
		Populations: []targetfixture.Population{
			{Denominator: candidates, Rows: []model.RowID{ids.candidateRow}},
			{Denominator: facts, Rows: []model.RowID{ids.factsRow}},
			{Denominator: output, Rows: []model.RowID{ids.outputRow}},
		},
		Scopes: []targetfixture.Scope{{ID: ids.scope, Region: "allocationbirth"}},
		Initials: []targetfixture.Initial{
			{
				Operation: seedCandidate,
				Scope:     ids.scope,
				Cells: func(issuer binding.Issuer) ([]targetfixture.Cell, bool) {
					address, ok := codecs.coordinate.Encode(issuer, coordinate)
					if !ok {
						return nil, false
					}
					allocation, ok := codecs.candidate.Encode(issuer, candidate)
					if !ok {
						return nil, false
					}
					addressCell, ok := targetfixture.Opaque(candidates, ids.candidateRow, ids.candidateAddress, address)
					if !ok {
						return nil, false
					}
					candidateCell, ok := targetfixture.Opaque(candidates, ids.candidateRow, ids.candidatePayload, allocation)
					if !ok {
						return nil, false
					}
					return []targetfixture.Cell{addressCell, candidateCell}, true
				},
			},
			{
				Operation: seedFacts,
				Scope:     ids.scope,
				Cells: func(issuer binding.Issuer) ([]targetfixture.Cell, bool) {
					address, ok := codecs.coordinate.Encode(issuer, coordinate)
					if !ok {
						return nil, false
					}
					value, ok := codecs.value.Encode(issuer, fresh)
					if !ok {
						return nil, false
					}
					addressCell, ok := targetfixture.Opaque(facts, ids.factsRow, ids.factsAddress, address)
					if !ok {
						return nil, false
					}
					valueCell, ok := targetfixture.Opaque(facts, ids.factsRow, ids.factValue, value)
					if !ok {
						return nil, false
					}
					return []targetfixture.Cell{addressCell, valueCell}, true
				},
			},
		},
		Authorities: func(issuer binding.Issuer) (targetfixture.Registry, bool) {
			lattice, ok := placementrelation.NewPlacementLattice()
			if !ok {
				return targetfixture.Registry{}, false
			}
			placementAlgebra, ok := relbindgen.NewAlgebra[placementdomain.Fact, placementrelation.PlacementLattice](codecs.placement, issuer, lattice)
			if !ok {
				return targetfixture.Registry{}, false
			}
			coordinateEquality, ok := relbindgen.NewEquality(codecs.coordinate, coordinateEquality{})
			if !ok {
				return targetfixture.Registry{}, false
			}
			return targetfixture.Registry{Algebras: []binding.ValueAlgebra{placementAlgebra}, Equalities: []binding.ValueEquality{coordinateEquality}}, true
		},
		MountByte: mountByte,
	})
	return Fixture{world: world, ids: IDs{PlacementOwner: ids.identity.Owner(), Output: ids.output, OutputPayload: ids.outputPayload}, placement: codecs.placement, expected: placementdomain.DefaultFact()}
}

func (value Fixture) IDs() IDs                       { return value.ids }
func (value Fixture) Mounted() witness.Mounted       { return value.world.Mounted() }
func (value Fixture) View() geometry.Geometry        { return value.world.View() }
func (value Fixture) Base() database.Version         { return value.world.Base() }
func (value Fixture) Expected() placementdomain.Fact { return value.expected }
func (value Fixture) PlacementCodec() *relbindgen.Column[placementdomain.Fact] {
	return value.placement
}

// Solve runs the target runtime for this allocation-birth specimen.
func (value Fixture) Solve() (terminal.Result, bool) { return value.world.Solve() }

// Facts projects the target terminal root and performs the owner typed query.
func (value Fixture) Facts(result terminal.Result) (placementquery.Rows, bool) {
	if value.placement == nil || !value.placement.Available() {
		return placementquery.Rows{}, false
	}
	projection, ok := value.world.Snapshot(result)
	if !ok || !projection.Available() {
		return placementquery.Rows{}, false
	}
	column, ok := placementquery.NewFactColumn(value.ids.OutputPayload, value.placement)
	if !ok {
		return placementquery.Rows{}, false
	}
	return placementquery.Read(projection, column)
}

type fixtureIDs struct {
	identity targetfixture.Identity
	schema   model.SchemaID

	coordinateType model.TypeID
	candidateType  model.TypeID
	valueType      model.TypeID
	placementType  model.TypeID
	scope          model.ScopeID

	candidate model.RelationID
	facts     model.RelationID
	output    model.RelationID

	candidateAddress model.ColumnID
	candidatePayload model.ColumnID
	factsAddress     model.ColumnID
	factValue        model.ColumnID
	outputAddress    model.ColumnID
	outputPayload    model.ColumnID

	candidateKey model.KeyID
	factsKey     model.KeyID
	outputKey    model.KeyID

	candidateRow model.RowID
	factsRow     model.RowID
	outputRow    model.RowID

	seedCandidate model.OperationID
	seedFacts     model.OperationID
	operation     model.OperationID
	expression    model.ExpressionID
	dependency    model.DependencyID
}

func newIDs(t targetfixture.Probe, rowContent identity.ContentID) fixtureIDs {
	t.Helper()
	identity := targetfixture.NewIdentity(t, fixtureDomain)
	value := fixtureIDs{
		identity:       identity,
		schema:         identity.Schema(t, "allocationbirth"),
		coordinateType: identity.Type(t, "coordinate"),
		candidateType:  identity.Type(t, "allocation-candidate"),
		valueType:      identity.Type(t, "value"),
		placementType:  identity.Type(t, "placement"),
		scope:          identity.Scope(t, "allocationbirth"),
		candidate:      identity.Relation(t, "candidate"),
		facts:          identity.Relation(t, "facts"),
		output:         identity.Relation(t, "output"),
		seedCandidate:  identity.Operation(t, "seed-candidate"),
		seedFacts:      identity.Operation(t, "seed-facts"),
		operation:      identity.Operation(t, "allocationbirth"),
		expression:     identity.Expression(t, "allocationbirth"),
		dependency:     identity.Dependency(t, "allocationbirth"),
	}
	value.candidateAddress = identity.Column(t, value.candidate, "address")
	value.candidatePayload = identity.Column(t, value.candidate, "payload")
	value.factsAddress = identity.Column(t, value.facts, "address")
	value.factValue = identity.Column(t, value.facts, "value")
	value.outputAddress = identity.Column(t, value.output, "address")
	value.outputPayload = identity.Column(t, value.output, "payload")
	value.candidateKey = identity.Key(t, value.candidate, "address")
	value.factsKey = identity.Key(t, value.facts, "address")
	value.outputKey = identity.Key(t, value.output, "address")
	value.candidateRow = identity.RowFromContent(t, value.candidate, rowContent)
	value.factsRow = identity.RowFromContent(t, value.facts, rowContent)
	value.outputRow = identity.RowFromContent(t, value.output, rowContent)
	return value
}

type codecs struct {
	coordinate *relbindgen.Column[valuedomain.Coordinate]
	candidate  *relbindgen.Column[*valuedomain.AllocationResult]
	value      *relbindgen.Column[valuedomain.Value]
	placement  *relbindgen.Column[placementdomain.Fact]
}

func newCodecs(t targetfixture.Probe, ids fixtureIDs) codecs {
	t.Helper()
	coordinateStore, ok := relbindgen.NewStore[valuedomain.Coordinate](content(t, ids.identity, "codec/coordinate"), 2)
	if !ok {
		t.Fatal("allocationbirth coordinate store")
	}
	coordinate, ok := relbindgen.NewColumn(ids.coordinateType, coordinateStore)
	if !ok {
		t.Fatal("allocationbirth coordinate codec")
	}
	candidateStore, ok := relbindgen.NewStore[*valuedomain.AllocationResult](content(t, ids.identity, "codec/candidate"), 2)
	if !ok {
		t.Fatal("allocationbirth candidate store")
	}
	candidate, ok := relbindgen.NewColumn(ids.candidateType, candidateStore)
	if !ok {
		t.Fatal("allocationbirth candidate codec")
	}
	valueStore, ok := relbindgen.NewStore[valuedomain.Value](content(t, ids.identity, "codec/value"), 2)
	if !ok {
		t.Fatal("allocationbirth value store")
	}
	value, ok := relbindgen.NewColumn(ids.valueType, valueStore)
	if !ok {
		t.Fatal("allocationbirth value codec")
	}
	placementStore, ok := relbindgen.NewStore[placementdomain.Fact](content(t, ids.identity, "codec/placement"), 2)
	if !ok {
		t.Fatal("allocationbirth placement store")
	}
	placement, ok := relbindgen.NewColumn(ids.placementType, placementStore)
	if !ok {
		t.Fatal("allocationbirth placement codec")
	}
	return codecs{coordinate: coordinate, candidate: candidate, value: value, placement: placement}
}

func newDeclaration(t targetfixture.Probe, ids fixtureIDs) (signature.Signature, signature.Signature, signature.Signature, relcompile.Declaration, model.DenominatorRef, model.DenominatorRef, model.DenominatorRef) {
	t.Helper()
	candidates, ok := model.NewDenominatorRef(ids.candidate, ids.candidateKey)
	if !ok {
		t.Fatal("allocationbirth candidate denominator")
	}
	facts, ok := model.NewDenominatorRef(ids.facts, ids.factsKey)
	if !ok {
		t.Fatal("allocationbirth facts denominator")
	}
	output, ok := model.NewDenominatorRef(ids.output, ids.outputKey)
	if !ok {
		t.Fatal("allocationbirth output denominator")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("allocationbirth cardinality")
	}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	if !ok {
		t.Fatal("allocationbirth outcomes")
	}
	scalar, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("allocationbirth scalar delivery")
	}
	seedCandidate := seal(t, ids, ids.seedCandidate, nil, []signature.Output{
		{Relation: ids.candidate, Column: ids.candidateAddress, Type: ids.coordinateType, Presence: signature.ProduceOpaque, Denominator: candidates},
		{Relation: ids.candidate, Column: ids.candidatePayload, Type: ids.candidateType, Presence: signature.ProduceOpaque, Denominator: candidates},
	}, candidates, cardinality, outcomes)
	seedFacts := seal(t, ids, ids.seedFacts, nil, []signature.Output{
		{Relation: ids.facts, Column: ids.factsAddress, Type: ids.coordinateType, Presence: signature.ProduceOpaque, Denominator: facts},
		{Relation: ids.facts, Column: ids.factValue, Type: ids.valueType, Presence: signature.ProduceOpaque, Denominator: facts},
	}, facts, cardinality, outcomes)
	operation := seal(t, ids, ids.operation, []signature.Input{
		{Relation: ids.candidate, Column: ids.candidatePayload, Type: ids.candidateType, Presence: signature.RequireOpaque, Delivery: scalar, Denominator: candidates},
		{Relation: ids.facts, Column: ids.factValue, Type: ids.valueType, Presence: signature.RequireOpaque, Delivery: scalar, Denominator: facts},
	}, []signature.Output{{Relation: ids.output, Column: ids.outputPayload, Type: ids.placementType, Presence: signature.ProducePresent, Denominator: output}}, output, cardinality, outcomes)
	coordinateCapability, ok := model.NewEquatableCapability(ids.coordinateType)
	if !ok {
		t.Fatal("allocationbirth coordinate capability")
	}
	candidateCapability, ok := model.NewDecodeOnlyCapability(ids.candidateType)
	if !ok {
		t.Fatal("allocationbirth candidate capability")
	}
	valueCapability, ok := model.NewDecodeOnlyCapability(ids.valueType)
	if !ok {
		t.Fatal("allocationbirth value capability")
	}
	placementCapability, ok := model.NewAscendingCapability(ids.placementType)
	if !ok {
		t.Fatal("allocationbirth placement capability")
	}
	declaration := relcompile.Declaration{
		SchemaID: ids.schema,
		Relations: []model.RelationSchema{
			model.DefineRelationSchema(ids.candidate, []model.ColumnID{ids.candidateAddress, ids.candidatePayload}, []model.KeyID{ids.candidateKey}, ids.scope),
			model.DefineRelationSchema(ids.facts, []model.ColumnID{ids.factsAddress, ids.factValue}, []model.KeyID{ids.factsKey}, ids.scope),
			model.DefineRelationSchema(ids.output, []model.ColumnID{ids.outputAddress, ids.outputPayload}, []model.KeyID{ids.outputKey}, ids.scope),
		},
		Columns: []model.ColumnSchema{
			model.DefineColumnSchema(ids.candidateAddress, ids.coordinateType),
			model.DefineColumnSchema(ids.candidatePayload, ids.candidateType),
			model.DefineColumnSchema(ids.factsAddress, ids.coordinateType),
			model.DefineColumnSchema(ids.factValue, ids.valueType),
			model.DefineColumnSchema(ids.outputAddress, ids.coordinateType),
			model.DefineColumnSchema(ids.outputPayload, ids.placementType),
		},
		TypeCapabilities: []model.TypeCapability{coordinateCapability, candidateCapability, valueCapability, placementCapability},
		Keys: []model.KeySchema{
			model.DefineKeySchema(ids.candidateKey, []model.ColumnID{ids.candidateAddress}),
			model.DefineKeySchema(ids.factsKey, []model.ColumnID{ids.factsAddress}),
			model.DefineKeySchema(ids.outputKey, []model.ColumnID{ids.outputAddress}),
		},
		Scopes:     []model.ScopeSchema{model.DefineScopeSchema(ids.scope, nil, region.True())},
		Signatures: []signature.Signature{seedCandidate, seedFacts, operation},
		Rules: []relcompile.Rule{{
			ID:         ids.dependency,
			Expression: ids.expression,
			Candidate:  ids.candidate,
			Joins: []relcompile.JoinSpec{{
				Relation:     ids.facts,
				LeftColumns:  []model.ColumnID{ids.candidateAddress},
				RightColumns: []model.ColumnID{ids.factsAddress},
				Scope:        ids.scope,
			}},
			ApplySlots: []relcompile.ReadOccurrence{relcompile.CandidateOccurrence(), relcompile.JoinOccurrence(0)},
			Scope:      ids.scope,
			Apply:      operation.Identity(),
			Output:     algebra.ScalarSource(algebra.NewSlotSource(0, 1)),
			Publish:    &relcompile.Publication{Relation: ids.output, Key: ids.outputKey, Columns: []model.ColumnID{ids.outputPayload}},
		}},
	}
	assertOutputGeometry(t, operation, declaration.Rules[0], ids.output, ids.outputPayload, output, 0, 1)
	return seedCandidate, seedFacts, operation, declaration, candidates, facts, output
}

// assertOutputGeometry is the owner law for AllocationBirth's exact write:
// its one output is an ExactlyOne placement fact addressed by the candidate
// payload's retained row.  The compiler must transport this source, never
// recover it from the output relation or cardinality.
func assertOutputGeometry(t targetfixture.Probe, operation signature.Signature, rule relcompile.Rule, outputRelation model.RelationID, outputColumn model.ColumnID, outputDenominator model.DenominatorRef, inputIndex, cell uint32) {
	t.Helper()
	outputs := operation.Outputs()
	if len(outputs) != 1 || outputs[0].Relation != outputRelation || outputs[0].Column != outputColumn || outputs[0].Denominator != outputDenominator {
		t.Fatal("allocationbirth output relation geometry")
	}
	if !operation.Cardinality().Available() || operation.Cardinality().Kind() != model.ExactlyOne {
		t.Fatal("allocationbirth output cardinality geometry")
	}
	source, ok := rule.Output.Source()
	if !rule.Output.IsScalarSource() || !ok || source != algebra.NewSlotSource(0, cell) {
		t.Fatalf("allocationbirth output source=%v/%t, want scalar child 0 cell %d", source, ok, cell)
	}
	input, ok := operation.InputAt(int(inputIndex))
	if !ok || !input.Delivery.IsScalar() {
		t.Fatalf("allocationbirth output source input=%d is not scalar", inputIndex)
	}
}

func seal(t targetfixture.Probe, ids fixtureIDs, operation model.OperationID, inputs []signature.Input, outputs []signature.Output, authority model.DenominatorRef, cardinality model.Cardinality, outcomes outcome.Set) signature.Signature {
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
		t.Fatal("allocationbirth signature")
	}
	return value
}

type coordinateEquality struct{}

func (coordinateEquality) Equal(left, right valuedomain.Coordinate) bool { return left == right }

func content(t targetfixture.Probe, issuer targetfixture.Identity, label string) identity.ContentID {
	t.Helper()
	value, ok := issuer.Content(label)
	if !ok {
		t.Fatalf("allocationbirth content %q", label)
	}
	return value
}
