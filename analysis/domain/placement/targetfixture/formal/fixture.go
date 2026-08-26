// Package formal is the first Placement family specimen using targetfixture.
// It supplies only Formal's declared schema, binding, typed seed, and oracle
// value; generic mounting and solving remain in the parent test fixture kit.
package formal

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

const fixtureDomain = "analysis/engine/relation/runtime/testdata/targetfixture/placement/formal/v1"

// Fixture carries Formal's typed output declaration and codec over the
// generic target runtime world.
type Fixture struct {
	world     targetfixture.World
	output    model.ColumnID
	placement *relbindgen.Column[placementdomain.Fact]
	expected  placementdomain.Fact
}

// New mounts a scalar Formal route. The input fact and route tag are family
// data; targetfixture neither chooses nor interprets them.
func New(t targetfixture.Probe) Fixture {
	t.Helper()
	ids := newIDs(t)
	codecs := newCodecs(t, ids)
	seed, operation, declaration, source, output := newDeclaration(t, ids)
	columns, ok := placementrelation.NewPlacementFormalColumns(codecs.placement, codecs.route)
	if !ok {
		t.Fatal("formal typed binding columns")
	}
	factory, ok := placementrelation.BindPlacementFormal(
		operation,
		placementrelation.PlacementFormalOperation{},
		columns,
		ids.identity.Refusal(t, "formal"),
	)
	if !ok {
		t.Fatal("formal typed binding")
	}
	world := targetfixture.Build(t, targetfixture.Spec{
		Identity:    ids.identity,
		Declaration: declaration,
		Bindings:    []binding.Factory{factory},
		Populations: []targetfixture.Population{
			{Denominator: source, Rows: []model.RowID{ids.sourceRow}},
			{Denominator: output, Rows: []model.RowID{ids.outputRow}},
		},
		Scopes: []targetfixture.Scope{{ID: ids.scope, Region: "formal"}},
		Initials: []targetfixture.Initial{{
			Operation: seed,
			Scope:     ids.scope,
			Cells: func(issuer binding.Issuer) ([]targetfixture.Cell, bool) {
				route, ok := codecs.route.Encode(issuer, uint64(0x1f))
				if !ok {
					return nil, false
				}
				selected, ok := codecs.placement.Encode(issuer, placementdomain.DefaultFact())
				if !ok {
					return nil, false
				}
				routeCell, ok := targetfixture.Present(source, ids.sourceRow, ids.routeTag, route)
				if !ok {
					return nil, false
				}
				selectedCell, ok := targetfixture.Present(source, ids.sourceRow, ids.selected, selected)
				if !ok {
					return nil, false
				}
				return []targetfixture.Cell{routeCell, selectedCell}, true
			},
		}},
		Authorities: func(issuer binding.Issuer) (targetfixture.Registry, bool) {
			lattice, ok := placementrelation.NewPlacementLattice()
			if !ok {
				return targetfixture.Registry{}, false
			}
			placementAlgebra, ok := relbindgen.NewAlgebra[placementdomain.Fact, placementrelation.PlacementLattice](codecs.placement, issuer, lattice)
			if !ok {
				return targetfixture.Registry{}, false
			}
			routeEquality, ok := relbindgen.NewEquality(codecs.route, uint64Equality{})
			if !ok {
				return targetfixture.Registry{}, false
			}
			routeAlgebra, ok := relbindgen.NewAlgebra[uint64, uint64Lattice](codecs.route, issuer, uint64Lattice{})
			if !ok {
				return targetfixture.Registry{}, false
			}
			return targetfixture.Registry{Algebras: []binding.ValueAlgebra{placementAlgebra, routeAlgebra}, Equalities: []binding.ValueEquality{routeEquality}}, true
		},
		MountByte: 0xF2,
	})
	return Fixture{world: world, output: ids.outputFact, placement: codecs.placement, expected: placementdomain.UnknownFact()}
}

// Solve runs Formal through the target serial runtime.
func (value Fixture) Solve() (terminal.Result, bool) { return value.world.Solve() }

// Facts projects Solve's target root and redeems the actual owner Fact codec.
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

	placementType model.TypeID
	routeType     model.TypeID
	scope         model.ScopeID

	source model.RelationID
	output model.RelationID

	routeTag      model.ColumnID
	selected      model.ColumnID
	outputAddress model.ColumnID
	outputFact    model.ColumnID

	sourceKey model.KeyID
	outputKey model.KeyID

	sourceRow model.RowID
	outputRow model.RowID

	seed       model.OperationID
	operation  model.OperationID
	expression model.ExpressionID
	dependency model.DependencyID
}

func newIDs(t targetfixture.Probe) ids {
	t.Helper()
	identity := targetfixture.NewIdentity(t, fixtureDomain)
	value := ids{
		identity:      identity,
		schema:        identity.Schema(t, "formal"),
		placementType: identity.Type(t, "placement-fact"),
		routeType:     identity.Type(t, "route-tag"),
		scope:         identity.Scope(t, "formal"),
		source:        identity.Relation(t, "source"),
		output:        identity.Relation(t, "output"),
		seed:          identity.Operation(t, "seed"),
		operation:     identity.Operation(t, "formal"),
		expression:    identity.Expression(t, "formal"),
		dependency:    identity.Dependency(t, "formal"),
	}
	value.routeTag = identity.Column(t, value.source, "route-tag")
	value.selected = identity.Column(t, value.source, "selected")
	value.outputAddress = identity.Column(t, value.output, "address")
	value.outputFact = identity.Column(t, value.output, "fact")
	value.sourceKey = identity.Key(t, value.source, "route-tag")
	value.outputKey = identity.Key(t, value.output, "address")
	rowContent := content(t, identity, "row/formal")
	value.sourceRow = identity.RowFromContent(t, value.source, rowContent)
	value.outputRow = identity.RowFromContent(t, value.output, rowContent)
	return value
}

type codecs struct {
	placement *relbindgen.Column[placementdomain.Fact]
	route     *relbindgen.Column[uint64]
}

func newCodecs(t targetfixture.Probe, ids ids) codecs {
	t.Helper()
	placementStore, ok := relbindgen.NewStore[placementdomain.Fact](content(t, ids.identity, "codec/placement"), 2)
	if !ok {
		t.Fatal("formal placement store")
	}
	placement, ok := relbindgen.NewColumn(ids.placementType, placementStore)
	if !ok {
		t.Fatal("formal placement codec")
	}
	routeStore, ok := relbindgen.NewStore[uint64](content(t, ids.identity, "codec/route"), 2)
	if !ok {
		t.Fatal("formal route store")
	}
	route, ok := relbindgen.NewColumn(ids.routeType, routeStore)
	if !ok {
		t.Fatal("formal route codec")
	}
	return codecs{placement: placement, route: route}
}

func newDeclaration(t targetfixture.Probe, ids ids) (signature.Signature, signature.Signature, relcompile.Declaration, model.DenominatorRef, model.DenominatorRef) {
	t.Helper()
	source, ok := model.NewDenominatorRef(ids.source, ids.sourceKey)
	if !ok {
		t.Fatal("formal source denominator")
	}
	output, ok := model.NewDenominatorRef(ids.output, ids.outputKey)
	if !ok {
		t.Fatal("formal output denominator")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("formal cardinality")
	}
	allOutcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	if !ok {
		t.Fatal("formal outcomes")
	}
	scalar, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("formal scalar delivery")
	}
	seed := seal(t, ids, ids.seed, nil, []signature.Output{
		{Relation: ids.source, Column: ids.routeTag, Type: ids.routeType, Presence: signature.ProducePresent, Denominator: source},
		{Relation: ids.source, Column: ids.selected, Type: ids.placementType, Presence: signature.ProducePresent, Denominator: source},
	}, source, cardinality, allOutcomes)
	formal := seal(t, ids, ids.operation, []signature.Input{
		{Relation: ids.source, Column: ids.routeTag, Type: ids.routeType, Presence: signature.RequirePresent, Delivery: scalar, Denominator: source},
		{Relation: ids.source, Column: ids.selected, Type: ids.placementType, Presence: signature.RequirePresent, Delivery: scalar, Denominator: source},
	}, []signature.Output{{Relation: ids.output, Column: ids.outputFact, Type: ids.placementType, Presence: signature.ProducePresent, Denominator: output}}, output, cardinality, allOutcomes)
	routeCapability, ok := model.NewAscendingCapability(ids.routeType)
	if !ok {
		t.Fatal("formal route capability")
	}
	placementCapability, ok := model.NewAscendingCapability(ids.placementType)
	if !ok {
		t.Fatal("formal placement capability")
	}
	declaration := relcompile.Declaration{
		SchemaID: ids.schema,
		Relations: []model.RelationSchema{
			model.DefineRelationSchema(ids.source, []model.ColumnID{ids.routeTag, ids.selected}, []model.KeyID{ids.sourceKey}, ids.scope),
			model.DefineRelationSchema(ids.output, []model.ColumnID{ids.outputAddress, ids.outputFact}, []model.KeyID{ids.outputKey}, ids.scope),
		},
		Columns: []model.ColumnSchema{
			model.DefineColumnSchema(ids.routeTag, ids.routeType),
			model.DefineColumnSchema(ids.selected, ids.placementType),
			model.DefineColumnSchema(ids.outputAddress, ids.routeType),
			model.DefineColumnSchema(ids.outputFact, ids.placementType),
		},
		TypeCapabilities: []model.TypeCapability{routeCapability, placementCapability},
		Keys: []model.KeySchema{
			model.DefineKeySchema(ids.sourceKey, []model.ColumnID{ids.routeTag}),
			model.DefineKeySchema(ids.outputKey, []model.ColumnID{ids.outputAddress}),
		},
		Scopes:     []model.ScopeSchema{model.DefineScopeSchema(ids.scope, nil, region.True())},
		Signatures: []signature.Signature{seed, formal},
		Rules: []relcompile.Rule{{
			ID:         ids.dependency,
			Expression: ids.expression,
			Candidate:  ids.source,
			ApplySlots: []relcompile.ReadOccurrence{relcompile.CandidateOccurrence(), relcompile.CandidateOccurrence()},
			Scope:      ids.scope,
			Apply:      formal.Identity(),
			Output:     algebra.ScalarSource(algebra.NewSlotSource(0, 1)),
			Publish:    &relcompile.Publication{Relation: ids.output, Key: ids.outputKey, Columns: []model.ColumnID{ids.outputFact}},
		}},
	}
	assertOutputGeometry(t, formal, declaration.Rules[0], ids, output)
	return seed, formal, declaration, source, output
}

// assertOutputGeometry is Formal's owner law: the exact output fact is
// addressed by the retained selected source cell under its sealed scalar
// delivery and ExactlyOne cardinality.
func assertOutputGeometry(t targetfixture.Probe, operation signature.Signature, rule relcompile.Rule, ids ids, output model.DenominatorRef) {
	t.Helper()
	outputs := operation.Outputs()
	if len(outputs) != 1 || outputs[0].Relation != ids.output || outputs[0].Column != ids.outputFact || outputs[0].Denominator != output {
		t.Fatal("formal output relation geometry")
	}
	if !operation.Cardinality().Available() || operation.Cardinality().Kind() != model.ExactlyOne {
		t.Fatal("formal output cardinality geometry")
	}
	source, ok := rule.Output.Source()
	if !rule.Output.IsScalarSource() || !ok || source != algebra.NewSlotSource(0, 1) {
		t.Fatalf("formal output source=%v/%t, want scalar child 0 cell 1", source, ok)
	}
	if input, inputOK := operation.InputAt(1); !inputOK || !input.Delivery.IsScalar() {
		t.Fatal("formal output source input is not scalar")
	}
}

func seal(t targetfixture.Probe, ids ids, operation model.OperationID, inputs []signature.Input, outputs []signature.Output, authority model.DenominatorRef, cardinality model.Cardinality, outcomes outcome.Set) signature.Signature {
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
		t.Fatal("formal signature")
	}
	return value
}

type uint64Equality struct{}

func (uint64Equality) Equal(left, right uint64) bool { return left == right }

// uint64Lattice is Formal's exact route-tag transport authority: one route
// identity may be republished unchanged, while competing tags have no
// family-defined join. The generic kit never manufactures this algebra.
type uint64Lattice struct{}

func (uint64Lattice) Join(left, right uint64) (uint64, bool) {
	return left, left == right
}

func (uint64Lattice) Widen(previous, next uint64) (uint64, bool) {
	return previous, previous == next
}

func (uint64Lattice) LessOrEq(left, right uint64) bool { return left == right }

func content(t targetfixture.Probe, issuer targetfixture.Identity, label string) identity.ContentID {
	t.Helper()
	value, ok := issuer.Content(label)
	if !ok {
		t.Fatalf("formal content %q", label)
	}
	return value
}
