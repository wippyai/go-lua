// Package containment supplies the complete family-owned target-runtime
// specimen for Placement Containment. The family owns the complete-vector
// geometry, owner-derived route row, typed values, and expected Fact.
package containment

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
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementcontainment "github.com/wippyai/go-lua/domain/placement/containment"
	"github.com/wippyai/go-lua/domain/relationfixture"
)

const fixtureDomain = "analysis/engine/relation/runtime/testdata/targetfixture/placement/containment/v1"

// Probe is the frozen generic fixture failure surface, re-exposed directly
// for consumers of this family package.
type Probe = targetfixture.Probe

// Fixture carries Containment's typed Fact query contract over one generic
// target world. It does not retain a legacy relation runtime or query path.
type Fixture struct {
	world     targetfixture.World
	output    model.ColumnID
	placement *relbindgen.Column[placementdomain.Fact]
	expected  placementdomain.Fact
}

// New constructs the one Containment schema and geometry authority. The
// owner relation fixture supplies authentic Heap values only. In particular,
// the repeated fold is sourced from J2's route row, never reopened from the
// complete placement or heap summary vectors.
func New(t Probe) Fixture {
	t.Helper()
	ownerWorld := relationfixture.New(t)
	ids := newIDs(t)
	codecs := newCodecs(t, ids)
	parentFact := placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven}
	currentFact := placementdomain.DefaultFact()
	expected, reduction := placementcontainment.ContainmentFold(currentFact, parentFact)
	if reduction != structure.Concrete {
		t.Fatalf("containment expected reduction = %v", reduction)
	}
	pointSeed, placementSeed, heapSeed, routeSeed, operation, declaration := newDeclaration(t, ids)
	columns, ok := placementrelation.NewPlacementContainmentColumns(codecs.placement)
	if !ok {
		t.Fatal("containment typed columns")
	}
	factory, ok := placementrelation.BindPlacementContainment(
		operation,
		placementrelation.PlacementContainmentOperation{},
		columns,
		ids.identity.Refusal(t, "containment"),
	)
	if !ok {
		t.Fatal("containment typed binding")
	}

	world := targetfixture.Build(t, targetfixture.Spec{
		Identity:    ids.identity,
		Declaration: declaration,
		Bindings:    []binding.Factory{factory},
		Populations: []targetfixture.Population{
			{Denominator: ids.pointDenominator, Rows: []model.RowID{ids.pointRow}},
			{Denominator: ids.placementDenominator, Rows: []model.RowID{ids.placementRow}},
			{Denominator: ids.heapDenominator, Rows: []model.RowID{ids.heapRow}},
			{Denominator: ids.routeDenominator, Rows: []model.RowID{ids.routeRow}},
			{Denominator: ids.outputDenominator, Rows: []model.RowID{ids.outputRow}},
		},
		Scopes: []targetfixture.Scope{{ID: ids.scope, Region: "containment"}},
		Initials: []targetfixture.Initial{
			{
				Operation: pointSeed,
				Scope:     ids.scope,
				Cells: func(issuer binding.Issuer) ([]targetfixture.Cell, bool) {
					link, ok := codecs.link.Encode(issuer, uint64(1))
					if !ok {
						return nil, false
					}
					return []targetfixture.Cell{opaque(t, ids.pointDenominator, ids.pointRow, ids.pointLink, link)}, true
				},
			},
			{
				Operation: placementSeed,
				Scope:     ids.scope,
				Cells: func(issuer binding.Issuer) ([]targetfixture.Cell, bool) {
					link, linkOK := codecs.link.Encode(issuer, uint64(1))
					fact, factOK := codecs.placement.Encode(issuer, parentFact)
					if !linkOK || !factOK {
						return nil, false
					}
					return []targetfixture.Cell{
						opaque(t, ids.placementDenominator, ids.placementRow, ids.placementLink, link),
						opaque(t, ids.placementDenominator, ids.placementRow, ids.placementFact, fact),
					}, true
				},
			},
			{
				Operation: heapSeed,
				Scope:     ids.scope,
				Cells: func(issuer binding.Issuer) ([]targetfixture.Cell, bool) {
					link, linkOK := codecs.link.Encode(issuer, uint64(1))
					value, valueOK := codecs.heapValue.Encode(issuer, ownerWorld.Heap.Top())
					if !linkOK || !valueOK {
						return nil, false
					}
					return []targetfixture.Cell{
						opaque(t, ids.heapDenominator, ids.heapRow, ids.heapLink, link),
						opaque(t, ids.heapDenominator, ids.heapRow, ids.heapValue, value),
					}, true
				},
			},
			{
				Operation: routeSeed,
				Scope:     ids.scope,
				Cells: func(issuer binding.Issuer) ([]targetfixture.Cell, bool) {
					link, linkOK := codecs.link.Encode(issuer, uint64(1))
					parentKey, parentKeyOK := codecs.heapKey.Encode(issuer, ownerWorld.Root)
					destinationKey, destinationKeyOK := codecs.heapKey.Encode(issuer, ownerWorld.Root)
					tag, tagOK := codecs.routeTag.Encode(issuer, uint64(1))
					current, currentOK := codecs.placement.Encode(issuer, currentFact)
					parent, parentOK := codecs.placement.Encode(issuer, parentFact)
					if !linkOK || !parentKeyOK || !destinationKeyOK || !tagOK || !currentOK || !parentOK {
						return nil, false
					}
					return []targetfixture.Cell{
						opaque(t, ids.routeDenominator, ids.routeRow, ids.routeLink, link),
						opaque(t, ids.routeDenominator, ids.routeRow, ids.parentKey, parentKey),
						opaque(t, ids.routeDenominator, ids.routeRow, ids.destinationKey, destinationKey),
						opaque(t, ids.routeDenominator, ids.routeRow, ids.routeTag, tag),
						opaque(t, ids.routeDenominator, ids.routeRow, ids.currentFact, current),
						opaque(t, ids.routeDenominator, ids.routeRow, ids.parentFact, parent),
					}, true
				},
			},
		},
		Authorities: func(issuer binding.Issuer) (targetfixture.Registry, bool) {
			lattice, latticeOK := placementrelation.NewPlacementLattice()
			algebra, algebraOK := relbindgen.NewAlgebra[placementdomain.Fact, placementrelation.PlacementLattice](codecs.placement, issuer, lattice)
			equality, equalityOK := relbindgen.NewEquality[uint64](codecs.link, uint64Equality{})
			if !latticeOK || !algebraOK || !equalityOK {
				return targetfixture.Registry{}, false
			}
			return targetfixture.Registry{Algebras: []binding.ValueAlgebra{algebra}, Equalities: []binding.ValueEquality{equality}}, true
		},
		MountByte: 0xC5,
	})
	return Fixture{world: world, output: ids.outputFact, placement: codecs.placement, expected: expected}
}

// Solve executes Containment through the production target relation runtime.
func (value Fixture) Solve() (terminal.Result, bool) { return value.world.Solve() }

// Facts redeems Containment's sole output through the canonical typed
// Placement query over the generic target snapshot.
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

// Expected returns Containment's family-owned fold oracle.
func (value Fixture) Expected() placementdomain.Fact { return value.expected }

type ids struct {
	identity targetfixture.Identity
	schema   model.SchemaID

	linkType      model.TypeID
	heapKeyType   model.TypeID
	routeTagType  model.TypeID
	heapValueType model.TypeID
	placementType model.TypeID
	scope         model.ScopeID

	point     model.RelationID
	placement model.RelationID
	heap      model.RelationID
	routes    model.RelationID
	output    model.RelationID

	pointLink      model.ColumnID
	placementLink  model.ColumnID
	placementFact  model.ColumnID
	heapLink       model.ColumnID
	heapValue      model.ColumnID
	routeLink      model.ColumnID
	parentKey      model.ColumnID
	destinationKey model.ColumnID
	routeTag       model.ColumnID
	currentFact    model.ColumnID
	parentFact     model.ColumnID
	outputLink     model.ColumnID
	outputFact     model.ColumnID
	pointKey       model.KeyID
	placementKey   model.KeyID
	heapKey        model.KeyID
	routeKey       model.KeyID
	outputKey      model.KeyID

	pointDenominator     model.DenominatorRef
	placementDenominator model.DenominatorRef
	heapDenominator      model.DenominatorRef
	routeDenominator     model.DenominatorRef
	outputDenominator    model.DenominatorRef

	pointRow     model.RowID
	placementRow model.RowID
	heapRow      model.RowID
	routeRow     model.RowID
	outputRow    model.RowID

	pointSeed     model.OperationID
	placementSeed model.OperationID
	heapSeed      model.OperationID
	routeSeed     model.OperationID
	operation     model.OperationID
	expression    model.ExpressionID
	dependency    model.DependencyID
}

func newIDs(t Probe) ids {
	t.Helper()
	identity := targetfixture.NewIdentity(t, fixtureDomain)
	value := ids{
		identity:      identity,
		schema:        identity.Schema(t, "containment"),
		linkType:      identity.Type(t, "point-link"),
		heapKeyType:   identity.Type(t, "heap-key"),
		routeTagType:  identity.Type(t, "route-tag"),
		heapValueType: identity.Type(t, "heap-value"),
		placementType: identity.Type(t, "placement"),
		scope:         identity.Scope(t, "containment"),
		point:         identity.Relation(t, "point"),
		placement:     identity.Relation(t, "placement-summary"),
		heap:          identity.Relation(t, "heap-summary"),
		routes:        identity.Relation(t, "containment-routes"),
		output:        identity.Relation(t, "output"),
		pointSeed:     identity.Operation(t, "seed/point"),
		placementSeed: identity.Operation(t, "seed/placement-summary"),
		heapSeed:      identity.Operation(t, "seed/heap-summary"),
		routeSeed:     identity.Operation(t, "seed/route"),
		operation:     identity.Operation(t, "containment"),
		expression:    identity.Expression(t, "containment"),
		dependency:    identity.Dependency(t, "containment"),
	}
	value.pointLink = identity.Column(t, value.point, "link")
	value.placementLink = identity.Column(t, value.placement, "link")
	value.placementFact = identity.Column(t, value.placement, "fact")
	value.heapLink = identity.Column(t, value.heap, "link")
	value.heapValue = identity.Column(t, value.heap, "value")
	value.routeLink = identity.Column(t, value.routes, "link")
	value.parentKey = identity.Column(t, value.routes, "parent-key")
	value.destinationKey = identity.Column(t, value.routes, "destination-key")
	value.routeTag = identity.Column(t, value.routes, "tag")
	value.currentFact = identity.Column(t, value.routes, "current")
	value.parentFact = identity.Column(t, value.routes, "parent")
	value.outputLink = identity.Column(t, value.output, "link")
	value.outputFact = identity.Column(t, value.output, "fact")
	value.pointKey = identity.Key(t, value.point, "link")
	value.placementKey = identity.Key(t, value.placement, "link")
	value.heapKey = identity.Key(t, value.heap, "link")
	value.routeKey = identity.Key(t, value.routes, "link")
	value.outputKey = identity.Key(t, value.output, "link")
	value.pointDenominator = denominator(t, value.point, value.pointKey)
	value.placementDenominator = denominator(t, value.placement, value.placementKey)
	value.heapDenominator = denominator(t, value.heap, value.heapKey)
	value.routeDenominator = denominator(t, value.routes, value.routeKey)
	value.outputDenominator = denominator(t, value.output, value.outputKey)
	value.pointRow = identity.Row(t, value.point, "mounted-point")
	value.placementRow = identity.Row(t, value.placement, "placement-vector")
	value.heapRow = identity.Row(t, value.heap, "heap-vector")
	value.routeRow = identity.Row(t, value.routes, "parent-to-child")
	value.outputRow = identity.RowFromContent(t, value.output, value.routeRow.Content())
	return value
}

type codecs struct {
	link      *relbindgen.Column[uint64]
	heapKey   *relbindgen.Column[heapdomain.Key]
	routeTag  *relbindgen.Column[uint64]
	heapValue *relbindgen.Column[heapdomain.Value]
	placement *relbindgen.Column[placementdomain.Fact]
}

func newCodecs(t Probe, ids ids) codecs {
	t.Helper()
	return codecs{
		link:      newCodec[uint64](t, ids, ids.linkType, "point-link"),
		heapKey:   newCodec[heapdomain.Key](t, ids, ids.heapKeyType, "heap-key"),
		routeTag:  newCodec[uint64](t, ids, ids.routeTagType, "route-tag"),
		heapValue: newCodec[heapdomain.Value](t, ids, ids.heapValueType, "heap-value"),
		placement: newCodec[placementdomain.Fact](t, ids, ids.placementType, "placement"),
	}
}

func newCodec[T any](t Probe, ids ids, typeID model.TypeID, label string) *relbindgen.Column[T] {
	t.Helper()
	store, ok := relbindgen.NewStore[T](content(t, ids.identity, "codec/"+label), 4)
	if !ok {
		t.Fatalf("containment %s store", label)
	}
	column, ok := relbindgen.NewColumn(typeID, store)
	if !ok {
		t.Fatalf("containment %s column", label)
	}
	return column
}

func newDeclaration(t Probe, ids ids) (signature.Signature, signature.Signature, signature.Signature, signature.Signature, signature.Signature, relcompile.Declaration) {
	t.Helper()
	pointSeed := seal(t, ids, ids.pointSeed, nil, []signature.Output{{Relation: ids.point, Column: ids.pointLink, Type: ids.linkType, Presence: signature.ProduceOpaque, Denominator: ids.pointDenominator}}, outcome.Produced)
	placementSeed := seal(t, ids, ids.placementSeed, nil, []signature.Output{
		{Relation: ids.placement, Column: ids.placementLink, Type: ids.linkType, Presence: signature.ProduceOpaque, Denominator: ids.placementDenominator},
		{Relation: ids.placement, Column: ids.placementFact, Type: ids.placementType, Presence: signature.ProduceOpaque, Denominator: ids.placementDenominator},
	}, outcome.Produced)
	heapSeed := seal(t, ids, ids.heapSeed, nil, []signature.Output{
		{Relation: ids.heap, Column: ids.heapLink, Type: ids.linkType, Presence: signature.ProduceOpaque, Denominator: ids.heapDenominator},
		{Relation: ids.heap, Column: ids.heapValue, Type: ids.heapValueType, Presence: signature.ProduceOpaque, Denominator: ids.heapDenominator},
	}, outcome.Produced)
	routeSeed := seal(t, ids, ids.routeSeed, nil, []signature.Output{
		{Relation: ids.routes, Column: ids.routeLink, Type: ids.linkType, Presence: signature.ProduceOpaque, Denominator: ids.routeDenominator},
		{Relation: ids.routes, Column: ids.parentKey, Type: ids.heapKeyType, Presence: signature.ProduceOpaque, Denominator: ids.routeDenominator},
		{Relation: ids.routes, Column: ids.destinationKey, Type: ids.heapKeyType, Presence: signature.ProduceOpaque, Denominator: ids.routeDenominator},
		{Relation: ids.routes, Column: ids.routeTag, Type: ids.routeTagType, Presence: signature.ProduceOpaque, Denominator: ids.routeDenominator},
		{Relation: ids.routes, Column: ids.currentFact, Type: ids.placementType, Presence: signature.ProduceOpaque, Denominator: ids.routeDenominator},
		{Relation: ids.routes, Column: ids.parentFact, Type: ids.placementType, Presence: signature.ProduceOpaque, Denominator: ids.routeDenominator},
	}, outcome.Produced)
	delivery := scalarDelivery(t)
	operation := seal(t, ids, ids.operation, []signature.Input{
		{Relation: ids.routes, Column: ids.currentFact, Type: ids.placementType, Presence: signature.RequireOpaque, Delivery: delivery, Denominator: ids.routeDenominator},
		{Relation: ids.routes, Column: ids.parentFact, Type: ids.placementType, Presence: signature.RequireOpaque, Delivery: delivery, Denominator: ids.routeDenominator},
	}, []signature.Output{{Relation: ids.output, Column: ids.outputFact, Type: ids.placementType, Presence: signature.ProducePresent, Denominator: ids.outputDenominator}}, outcome.Produced, outcome.Refused)

	placementComplete := ids.placementDenominator
	heapComplete := ids.heapDenominator
	declaration := relcompile.Declaration{
		SchemaID: ids.schema,
		Relations: []model.RelationSchema{
			model.DefineRelationSchema(ids.point, []model.ColumnID{ids.pointLink}, []model.KeyID{ids.pointKey}, ids.scope),
			model.DefineRelationSchema(ids.placement, []model.ColumnID{ids.placementLink, ids.placementFact}, []model.KeyID{ids.placementKey}, ids.scope),
			model.DefineRelationSchema(ids.heap, []model.ColumnID{ids.heapLink, ids.heapValue}, []model.KeyID{ids.heapKey}, ids.scope),
			model.DefineRelationSchema(ids.routes, []model.ColumnID{ids.routeLink, ids.parentKey, ids.destinationKey, ids.routeTag, ids.currentFact, ids.parentFact}, []model.KeyID{ids.routeKey}, ids.scope),
			model.DefineRelationSchema(ids.output, []model.ColumnID{ids.outputLink, ids.outputFact}, []model.KeyID{ids.outputKey}, ids.scope),
		},
		Columns: []model.ColumnSchema{
			model.DefineColumnSchema(ids.pointLink, ids.linkType),
			model.DefineColumnSchema(ids.placementLink, ids.linkType),
			model.DefineColumnSchema(ids.placementFact, ids.placementType),
			model.DefineColumnSchema(ids.heapLink, ids.linkType),
			model.DefineColumnSchema(ids.heapValue, ids.heapValueType),
			model.DefineColumnSchema(ids.routeLink, ids.linkType),
			model.DefineColumnSchema(ids.parentKey, ids.heapKeyType),
			model.DefineColumnSchema(ids.destinationKey, ids.heapKeyType),
			model.DefineColumnSchema(ids.routeTag, ids.routeTagType),
			model.DefineColumnSchema(ids.currentFact, ids.placementType),
			model.DefineColumnSchema(ids.parentFact, ids.placementType),
			model.DefineColumnSchema(ids.outputLink, ids.linkType),
			model.DefineColumnSchema(ids.outputFact, ids.placementType),
		},
		TypeCapabilities: []model.TypeCapability{
			capability(t, ids.linkType, model.Equatable),
			capability(t, ids.heapKeyType, model.DecodeOnly),
			capability(t, ids.routeTagType, model.DecodeOnly),
			capability(t, ids.heapValueType, model.DecodeOnly),
			capability(t, ids.placementType, model.Ascending),
		},
		Keys: []model.KeySchema{
			model.DefineKeySchema(ids.pointKey, []model.ColumnID{ids.pointLink}),
			model.DefineKeySchema(ids.placementKey, []model.ColumnID{ids.placementLink}),
			model.DefineKeySchema(ids.heapKey, []model.ColumnID{ids.heapLink}),
			model.DefineKeySchema(ids.routeKey, []model.ColumnID{ids.routeLink}),
			model.DefineKeySchema(ids.outputKey, []model.ColumnID{ids.outputLink}),
		},
		Scopes:     []model.ScopeSchema{model.DefineScopeSchema(ids.scope, nil, region.True())},
		Signatures: []signature.Signature{pointSeed, placementSeed, heapSeed, routeSeed, operation},
		Rules: []relcompile.Rule{{
			ID:         ids.dependency,
			Expression: ids.expression,
			Candidate:  ids.point,
			Joins: []relcompile.JoinSpec{
				{Relation: ids.placement, LeftColumns: []model.ColumnID{ids.pointLink}, RightColumns: []model.ColumnID{ids.placementLink}, Scope: ids.scope, Complete: &placementComplete},
				{Relation: ids.heap, LeftColumns: []model.ColumnID{ids.pointLink}, RightColumns: []model.ColumnID{ids.heapLink}, Scope: ids.scope, Complete: &heapComplete},
				{Relation: ids.routes, LeftColumns: []model.ColumnID{ids.pointLink}, RightColumns: []model.ColumnID{ids.routeLink}, Scope: ids.scope},
			},
			ApplySlots: []relcompile.ReadOccurrence{
				relcompile.JoinOccurrence(2),
				relcompile.JoinOccurrence(2),
			},
			Scope:   ids.scope,
			Apply:   operation.Identity(),
			Output:  algebra.ScalarSource(algebra.NewSlotSource(0, 10)),
			Publish: &relcompile.Publication{Relation: ids.output, Key: ids.outputKey, Columns: []model.ColumnID{ids.outputFact}},
		}},
	}
	assertGeometry(t, declaration.Rules[0], operation, ids)
	return pointSeed, placementSeed, heapSeed, routeSeed, operation, declaration
}

func assertGeometry(t Probe, rule relcompile.Rule, operation signature.Signature, ids ids) {
	t.Helper()
	if len(rule.Joins) != 3 || rule.Joins[0].Complete == nil || rule.Joins[1].Complete == nil || rule.Joins[2].Complete != nil {
		t.Fatal("containment geometry must retain complete J0/J1 and selected J2")
	}
	if len(rule.ApplySlots) != 2 {
		t.Fatalf("containment fold slots=%d, want repeated J2", len(rule.ApplySlots))
	}
	for index, slot := range rule.ApplySlots {
		join, ok := slot.Join()
		if !ok || join != 2 {
			t.Fatalf("containment fold slot %d=%v/%t, want J2", index, join, ok)
		}
	}
	outputs := operation.Outputs()
	if len(outputs) != 1 || outputs[0].Relation != ids.output || outputs[0].Column != ids.outputFact || outputs[0].Denominator != ids.outputDenominator {
		t.Fatal("containment output relation geometry")
	}
	if !operation.Cardinality().Available() || operation.Cardinality().Kind() != model.ExactlyOne {
		t.Fatal("containment output cardinality geometry")
	}
	source, ok := rule.Output.Source()
	if !rule.Output.IsScalarSource() || !ok || source != algebra.NewSlotSource(0, 10) {
		t.Fatalf("containment output source=%v/%t, want scalar child 0 cell 10", source, ok)
	}
	if input, inputOK := operation.InputAt(1); !inputOK || !input.Delivery.IsScalar() {
		t.Fatal("containment output source input is not scalar")
	}
}

func seal(t Probe, ids ids, operation model.OperationID, inputs []signature.Input, outputs []signature.Output, codes ...outcome.Code) signature.Signature {
	t.Helper()
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("containment cardinality")
	}
	accepted, ok := outcome.NewSet(codes...)
	if !ok {
		t.Fatal("containment outcomes")
	}
	value, ok := signature.Seal(signature.Spec{
		Identity:    signature.Identity{Operation: operation, Version: 1},
		Fence:       signature.Fence{Owner: ids.identity.Owner(), Schema: ids.schema},
		Inputs:      inputs,
		Outputs:     outputs,
		Cardinality: cardinality,
		Outcomes:    accepted,
	})
	if !ok {
		t.Fatalf("containment signature %v", operation)
	}
	return value
}

func scalarDelivery(t Probe) signature.Delivery {
	t.Helper()
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("containment scalar delivery")
	}
	return delivery
}

func denominator(t Probe, relation model.RelationID, key model.KeyID) model.DenominatorRef {
	t.Helper()
	value, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("containment denominator")
	}
	return value
}

func capability(t Probe, typeID model.TypeID, kind model.TypeCapabilityKind) model.TypeCapability {
	t.Helper()
	value, ok := model.NewTypeCapability(typeID, kind)
	if !ok {
		t.Fatal("containment type capability")
	}
	return value
}

func content(t Probe, owner targetfixture.Identity, label string) identity.ContentID {
	t.Helper()
	value, ok := owner.Content(label)
	if !ok {
		t.Fatalf("containment content %q", label)
	}
	return value
}

func opaque(t Probe, denominator model.DenominatorRef, row model.RowID, column model.ColumnID, value binding.ValueToken) targetfixture.Cell {
	t.Helper()
	cell, ok := targetfixture.Opaque(denominator, row, column, value)
	if !ok {
		t.Fatal("containment opaque seed cell")
	}
	return cell
}

type uint64Equality struct{}

func (uint64Equality) Equal(left, right uint64) bool { return left == right }
