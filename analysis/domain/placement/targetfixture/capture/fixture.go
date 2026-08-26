// Package capture supplies the complete family-owned target-runtime specimen
// for Placement Capture. It owns Capture's schema, routed input geometry,
// typed seed values, and expected Fact; targetfixture owns all runtime work.
package capture

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
	placementcapture "github.com/wippyai/go-lua/domain/placement/capture"
	"github.com/wippyai/go-lua/domain/relationfixture"
)

const fixtureDomain = "analysis/engine/relation/runtime/testdata/targetfixture/placement/capture/v1"

// Probe is the fixture family failure surface. It is intentionally identical
// to the frozen generic kit contract, so callers need no adapter.
type Probe = targetfixture.Probe

// Fixture is Capture's typed Fact query contract over one generic target
// world. It retains no legacy evaluator, result store, or schema duplicate.
type Fixture struct {
	world     targetfixture.World
	output    model.ColumnID
	placement *relbindgen.Column[placementdomain.Fact]
	expected  placementdomain.Fact
}

// New builds Capture's single declaration authority. The domain relation
// fixture supplies the real authenticated Heap root used as the candidate;
// it is never used as a target runtime or target query implementation.
func New(t Probe) Fixture {
	t.Helper()
	ownerWorld := relationfixture.New(t)
	ids := newIDs(t)
	codecs := newCodecs(t, ids)
	parentFact := placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven}
	currentFact := placementdomain.DefaultFact()
	expected, reduction := placementcapture.CaptureFold(parentFact, 1, currentFact)
	if reduction != structure.Concrete {
		t.Fatalf("capture expected reduction = %v", reduction)
	}
	parentSeed, candidateSeed, routeSeed, currentSeed, operation, declaration := newDeclaration(t, ids)
	columns, ok := placementrelation.NewPlacementCaptureColumns(codecs.placement, codecs.heap, codecs.routeTag)
	if !ok {
		t.Fatal("capture typed columns")
	}
	factory, ok := placementrelation.BindPlacementCapture(
		operation,
		placementrelation.PlacementCaptureOperation{},
		columns,
		ids.identity.Refusal(t, "capture"),
	)
	if !ok {
		t.Fatal("capture typed binding")
	}

	world := targetfixture.Build(t, targetfixture.Spec{
		Identity:    ids.identity,
		Declaration: declaration,
		Bindings:    []binding.Factory{factory},
		Populations: []targetfixture.Population{
			{Denominator: ids.parentDenominator, Rows: []model.RowID{ids.parentRow}},
			{Denominator: ids.candidateDenominator, Rows: []model.RowID{ids.candidateRow}},
			{Denominator: ids.routeDenominator, Rows: []model.RowID{ids.routeRow}},
			{Denominator: ids.currentDenominator, Rows: []model.RowID{ids.currentRow}},
			{Denominator: ids.outputDenominator, Rows: []model.RowID{ids.outputRow}},
		},
		Scopes: []targetfixture.Scope{{ID: ids.scope, Region: "capture"}},
		Initials: []targetfixture.Initial{
			{
				Operation: parentSeed,
				Scope:     ids.scope,
				Cells: func(issuer binding.Issuer) ([]targetfixture.Cell, bool) {
					link, linkOK := codecs.link.Encode(issuer, uint64(1))
					fact, factOK := codecs.placement.Encode(issuer, parentFact)
					if !linkOK || !factOK {
						return nil, false
					}
					return []targetfixture.Cell{
						opaque(t, ids.parentDenominator, ids.parentRow, ids.parentLink, link),
						opaque(t, ids.parentDenominator, ids.parentRow, ids.parentFact, fact),
					}, true
				},
			},
			{
				Operation: candidateSeed,
				Scope:     ids.scope,
				Cells: func(issuer binding.Issuer) ([]targetfixture.Cell, bool) {
					link, linkOK := codecs.link.Encode(issuer, uint64(1))
					candidate, candidateOK := codecs.heap.Encode(issuer, ownerWorld.Root)
					if !linkOK || !candidateOK {
						return nil, false
					}
					return []targetfixture.Cell{
						opaque(t, ids.candidateDenominator, ids.candidateRow, ids.candidateLink, link),
						opaque(t, ids.candidateDenominator, ids.candidateRow, ids.candidateKey, candidate),
					}, true
				},
			},
			{
				Operation: routeSeed,
				Scope:     ids.scope,
				Cells: func(issuer binding.Issuer) ([]targetfixture.Cell, bool) {
					link, linkOK := codecs.link.Encode(issuer, uint64(1))
					tag, tagOK := codecs.routeTag.Encode(issuer, uint64(1))
					if !linkOK || !tagOK {
						return nil, false
					}
					return []targetfixture.Cell{
						opaque(t, ids.routeDenominator, ids.routeRow, ids.routeLink, link),
						opaque(t, ids.routeDenominator, ids.routeRow, ids.routeTag, tag),
					}, true
				},
			},
			{
				Operation: currentSeed,
				Scope:     ids.scope,
				Cells: func(issuer binding.Issuer) ([]targetfixture.Cell, bool) {
					link, linkOK := codecs.link.Encode(issuer, uint64(1))
					fact, factOK := codecs.placement.Encode(issuer, currentFact)
					if !linkOK || !factOK {
						return nil, false
					}
					return []targetfixture.Cell{
						opaque(t, ids.currentDenominator, ids.currentRow, ids.currentLink, link),
						opaque(t, ids.currentDenominator, ids.currentRow, ids.currentFact, fact),
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
		MountByte: 0xC4,
	})
	return Fixture{world: world, output: ids.outputFact, placement: codecs.placement, expected: expected}
}

// Solve executes Capture through the production target relation runtime.
func (value Fixture) Solve() (terminal.Result, bool) { return value.world.Solve() }

// Facts redeems Capture's sole output through the canonical typed Placement
// query over the generic target snapshot.
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

// Expected returns Capture's family-owned fold oracle.
func (value Fixture) Expected() placementdomain.Fact { return value.expected }

type ids struct {
	identity targetfixture.Identity
	schema   model.SchemaID

	linkType      model.TypeID
	heapType      model.TypeID
	routeTagType  model.TypeID
	placementType model.TypeID
	scope         model.ScopeID

	parent    model.RelationID
	candidate model.RelationID
	route     model.RelationID
	current   model.RelationID
	output    model.RelationID

	parentLink     model.ColumnID
	parentFact     model.ColumnID
	candidateLink  model.ColumnID
	candidateKey   model.ColumnID
	routeLink      model.ColumnID
	routeTag       model.ColumnID
	currentLink    model.ColumnID
	currentFact    model.ColumnID
	outputLink     model.ColumnID
	outputFact     model.ColumnID
	parentKey      model.KeyID
	candidateKeyID model.KeyID
	routeKey       model.KeyID
	currentKey     model.KeyID
	outputKey      model.KeyID

	parentDenominator    model.DenominatorRef
	candidateDenominator model.DenominatorRef
	routeDenominator     model.DenominatorRef
	currentDenominator   model.DenominatorRef
	outputDenominator    model.DenominatorRef

	parentRow    model.RowID
	candidateRow model.RowID
	routeRow     model.RowID
	currentRow   model.RowID
	outputRow    model.RowID

	parentSeed    model.OperationID
	candidateSeed model.OperationID
	routeSeed     model.OperationID
	currentSeed   model.OperationID
	operation     model.OperationID
	expression    model.ExpressionID
	dependency    model.DependencyID
}

func newIDs(t Probe) ids {
	t.Helper()
	identity := targetfixture.NewIdentity(t, fixtureDomain)
	value := ids{
		identity:      identity,
		schema:        identity.Schema(t, "capture"),
		linkType:      identity.Type(t, "route-link"),
		heapType:      identity.Type(t, "heap-candidate"),
		routeTagType:  identity.Type(t, "route-tag"),
		placementType: identity.Type(t, "placement"),
		scope:         identity.Scope(t, "capture"),
		parent:        identity.Relation(t, "parent"),
		candidate:     identity.Relation(t, "heap-candidate"),
		route:         identity.Relation(t, "route-tag"),
		current:       identity.Relation(t, "current"),
		output:        identity.Relation(t, "output"),
		parentSeed:    identity.Operation(t, "seed/parent"),
		candidateSeed: identity.Operation(t, "seed/candidate"),
		routeSeed:     identity.Operation(t, "seed/route-tag"),
		currentSeed:   identity.Operation(t, "seed/current"),
		operation:     identity.Operation(t, "capture"),
		expression:    identity.Expression(t, "capture"),
		dependency:    identity.Dependency(t, "capture"),
	}
	value.parentLink = identity.Column(t, value.parent, "link")
	value.parentFact = identity.Column(t, value.parent, "fact")
	value.candidateLink = identity.Column(t, value.candidate, "link")
	value.candidateKey = identity.Column(t, value.candidate, "key")
	value.routeLink = identity.Column(t, value.route, "link")
	value.routeTag = identity.Column(t, value.route, "tag")
	value.currentLink = identity.Column(t, value.current, "link")
	value.currentFact = identity.Column(t, value.current, "fact")
	value.outputLink = identity.Column(t, value.output, "link")
	value.outputFact = identity.Column(t, value.output, "fact")
	value.parentKey = identity.Key(t, value.parent, "link")
	value.candidateKeyID = identity.Key(t, value.candidate, "link")
	value.routeKey = identity.Key(t, value.route, "link")
	value.currentKey = identity.Key(t, value.current, "link")
	value.outputKey = identity.Key(t, value.output, "link")
	value.parentDenominator = denominator(t, value.parent, value.parentKey)
	value.candidateDenominator = denominator(t, value.candidate, value.candidateKeyID)
	value.routeDenominator = denominator(t, value.route, value.routeKey)
	value.currentDenominator = denominator(t, value.current, value.currentKey)
	value.outputDenominator = denominator(t, value.output, value.outputKey)
	value.parentRow = identity.Row(t, value.parent, "parent")
	value.candidateRow = identity.Row(t, value.candidate, "candidate")
	value.routeRow = identity.Row(t, value.route, "route-tag")
	value.currentRow = identity.Row(t, value.current, "current")
	value.outputRow = identity.RowFromContent(t, value.output, value.currentRow.Content())
	return value
}

type codecs struct {
	link      *relbindgen.Column[uint64]
	heap      *relbindgen.Column[heapdomain.Key]
	routeTag  *relbindgen.Column[uint64]
	placement *relbindgen.Column[placementdomain.Fact]
}

func newCodecs(t Probe, ids ids) codecs {
	t.Helper()
	return codecs{
		link:      newCodec[uint64](t, ids, ids.linkType, "route-link"),
		heap:      newCodec[heapdomain.Key](t, ids, ids.heapType, "heap-candidate"),
		routeTag:  newCodec[uint64](t, ids, ids.routeTagType, "route-tag"),
		placement: newCodec[placementdomain.Fact](t, ids, ids.placementType, "placement"),
	}
}

func newCodec[T any](t Probe, ids ids, typeID model.TypeID, label string) *relbindgen.Column[T] {
	t.Helper()
	store, ok := relbindgen.NewStore[T](content(t, ids.identity, "codec/"+label), 4)
	if !ok {
		t.Fatalf("capture %s store", label)
	}
	column, ok := relbindgen.NewColumn(typeID, store)
	if !ok {
		t.Fatalf("capture %s column", label)
	}
	return column
}

func newDeclaration(t Probe, ids ids) (signature.Signature, signature.Signature, signature.Signature, signature.Signature, signature.Signature, relcompile.Declaration) {
	t.Helper()
	parentSeed := seal(t, ids, ids.parentSeed, nil, []signature.Output{
		{Relation: ids.parent, Column: ids.parentLink, Type: ids.linkType, Presence: signature.ProduceOpaque, Denominator: ids.parentDenominator},
		{Relation: ids.parent, Column: ids.parentFact, Type: ids.placementType, Presence: signature.ProduceOpaque, Denominator: ids.parentDenominator},
	}, outcome.Produced)
	candidateSeed := seal(t, ids, ids.candidateSeed, nil, []signature.Output{
		{Relation: ids.candidate, Column: ids.candidateLink, Type: ids.linkType, Presence: signature.ProduceOpaque, Denominator: ids.candidateDenominator},
		{Relation: ids.candidate, Column: ids.candidateKey, Type: ids.heapType, Presence: signature.ProduceOpaque, Denominator: ids.candidateDenominator},
	}, outcome.Produced)
	routeSeed := seal(t, ids, ids.routeSeed, nil, []signature.Output{
		{Relation: ids.route, Column: ids.routeLink, Type: ids.linkType, Presence: signature.ProduceOpaque, Denominator: ids.routeDenominator},
		{Relation: ids.route, Column: ids.routeTag, Type: ids.routeTagType, Presence: signature.ProduceOpaque, Denominator: ids.routeDenominator},
	}, outcome.Produced)
	currentSeed := seal(t, ids, ids.currentSeed, nil, []signature.Output{
		{Relation: ids.current, Column: ids.currentLink, Type: ids.linkType, Presence: signature.ProduceOpaque, Denominator: ids.currentDenominator},
		{Relation: ids.current, Column: ids.currentFact, Type: ids.placementType, Presence: signature.ProduceOpaque, Denominator: ids.currentDenominator},
	}, outcome.Produced)
	delivery := scalarDelivery(t)
	operation := seal(t, ids, ids.operation, []signature.Input{
		{Relation: ids.parent, Column: ids.parentFact, Type: ids.placementType, Presence: signature.RequireOpaque, Delivery: delivery, Denominator: ids.parentDenominator},
		{Relation: ids.candidate, Column: ids.candidateKey, Type: ids.heapType, Presence: signature.RequireOpaque, Delivery: delivery, Denominator: ids.candidateDenominator},
		{Relation: ids.route, Column: ids.routeTag, Type: ids.routeTagType, Presence: signature.RequireOpaque, Delivery: delivery, Denominator: ids.routeDenominator},
		{Relation: ids.current, Column: ids.currentFact, Type: ids.placementType, Presence: signature.RequireOpaque, Delivery: delivery, Denominator: ids.currentDenominator},
	}, []signature.Output{{Relation: ids.output, Column: ids.outputFact, Type: ids.placementType, Presence: signature.ProducePresent, Denominator: ids.outputDenominator}}, outcome.Produced, outcome.Refused)

	declaration := relcompile.Declaration{
		SchemaID: ids.schema,
		Relations: []model.RelationSchema{
			model.DefineRelationSchema(ids.parent, []model.ColumnID{ids.parentLink, ids.parentFact}, []model.KeyID{ids.parentKey}, ids.scope),
			model.DefineRelationSchema(ids.candidate, []model.ColumnID{ids.candidateLink, ids.candidateKey}, []model.KeyID{ids.candidateKeyID}, ids.scope),
			model.DefineRelationSchema(ids.route, []model.ColumnID{ids.routeLink, ids.routeTag}, []model.KeyID{ids.routeKey}, ids.scope),
			model.DefineRelationSchema(ids.current, []model.ColumnID{ids.currentLink, ids.currentFact}, []model.KeyID{ids.currentKey}, ids.scope),
			model.DefineRelationSchema(ids.output, []model.ColumnID{ids.outputLink, ids.outputFact}, []model.KeyID{ids.outputKey}, ids.scope),
		},
		Columns: []model.ColumnSchema{
			model.DefineColumnSchema(ids.parentLink, ids.linkType),
			model.DefineColumnSchema(ids.parentFact, ids.placementType),
			model.DefineColumnSchema(ids.candidateLink, ids.linkType),
			model.DefineColumnSchema(ids.candidateKey, ids.heapType),
			model.DefineColumnSchema(ids.routeLink, ids.linkType),
			model.DefineColumnSchema(ids.routeTag, ids.routeTagType),
			model.DefineColumnSchema(ids.currentLink, ids.linkType),
			model.DefineColumnSchema(ids.currentFact, ids.placementType),
			model.DefineColumnSchema(ids.outputLink, ids.linkType),
			model.DefineColumnSchema(ids.outputFact, ids.placementType),
		},
		TypeCapabilities: []model.TypeCapability{
			capability(t, ids.linkType, model.Equatable),
			capability(t, ids.heapType, model.DecodeOnly),
			capability(t, ids.routeTagType, model.DecodeOnly),
			capability(t, ids.placementType, model.Ascending),
		},
		Keys: []model.KeySchema{
			model.DefineKeySchema(ids.parentKey, []model.ColumnID{ids.parentLink}),
			model.DefineKeySchema(ids.candidateKeyID, []model.ColumnID{ids.candidateLink}),
			model.DefineKeySchema(ids.routeKey, []model.ColumnID{ids.routeLink}),
			model.DefineKeySchema(ids.currentKey, []model.ColumnID{ids.currentLink}),
			model.DefineKeySchema(ids.outputKey, []model.ColumnID{ids.outputLink}),
		},
		Scopes:     []model.ScopeSchema{model.DefineScopeSchema(ids.scope, nil, region.True())},
		Signatures: []signature.Signature{parentSeed, candidateSeed, routeSeed, currentSeed, operation},
		Rules: []relcompile.Rule{{
			ID:         ids.dependency,
			Expression: ids.expression,
			Candidate:  ids.parent,
			Joins: []relcompile.JoinSpec{
				{Relation: ids.candidate, LeftColumns: []model.ColumnID{ids.parentLink}, RightColumns: []model.ColumnID{ids.candidateLink}, Scope: ids.scope},
				{Relation: ids.route, LeftColumns: []model.ColumnID{ids.parentLink}, RightColumns: []model.ColumnID{ids.routeLink}, Scope: ids.scope},
				{Relation: ids.current, LeftColumns: []model.ColumnID{ids.parentLink}, RightColumns: []model.ColumnID{ids.currentLink}, Scope: ids.scope},
			},
			ApplySlots: []relcompile.ReadOccurrence{
				relcompile.CandidateOccurrence(),
				relcompile.JoinOccurrence(0),
				relcompile.JoinOccurrence(1),
				relcompile.JoinOccurrence(2),
			},
			Scope:   ids.scope,
			Apply:   operation.Identity(),
			Output:  algebra.ScalarSource(algebra.NewSlotSource(0, 7)),
			Publish: &relcompile.Publication{Relation: ids.output, Key: ids.outputKey, Columns: []model.ColumnID{ids.outputFact}},
		}},
	}
	assertGeometry(t, declaration.Rules[0], operation, ids)
	return parentSeed, candidateSeed, routeSeed, currentSeed, operation, declaration
}

func assertGeometry(t Probe, rule relcompile.Rule, operation signature.Signature, ids ids) {
	t.Helper()
	if len(rule.Joins) != 3 {
		t.Fatalf("capture joins=%d, want heap candidate, route tag, current placement", len(rule.Joins))
	}
	if len(rule.ApplySlots) != 4 || !rule.ApplySlots[0].Candidate() {
		t.Fatal("capture input geometry did not retain parent candidate")
	}
	for index, want := range []uint32{0, 1, 2} {
		join, ok := rule.ApplySlots[index+1].Join()
		if !ok || join != want {
			t.Fatalf("capture input slot %d=%v/%t, want J%d", index+1, join, ok, want)
		}
	}
	outputs := operation.Outputs()
	if len(outputs) != 1 || outputs[0].Relation != ids.output || outputs[0].Column != ids.outputFact || outputs[0].Denominator != ids.outputDenominator {
		t.Fatal("capture output relation geometry")
	}
	if !operation.Cardinality().Available() || operation.Cardinality().Kind() != model.ExactlyOne {
		t.Fatal("capture output cardinality geometry")
	}
	source, ok := rule.Output.Source()
	if !rule.Output.IsScalarSource() || !ok || source != algebra.NewSlotSource(0, 7) {
		t.Fatalf("capture output source=%v/%t, want scalar child 0 cell 7", source, ok)
	}
	if input, inputOK := operation.InputAt(3); !inputOK || !input.Delivery.IsScalar() {
		t.Fatal("capture output source input is not scalar")
	}
}

func seal(t Probe, ids ids, operation model.OperationID, inputs []signature.Input, outputs []signature.Output, codes ...outcome.Code) signature.Signature {
	t.Helper()
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("capture cardinality")
	}
	accepted, ok := outcome.NewSet(codes...)
	if !ok {
		t.Fatal("capture outcomes")
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
		t.Fatalf("capture signature %v", operation)
	}
	return value
}

func scalarDelivery(t Probe) signature.Delivery {
	t.Helper()
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("capture scalar delivery")
	}
	return delivery
}

func denominator(t Probe, relation model.RelationID, key model.KeyID) model.DenominatorRef {
	t.Helper()
	value, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("capture denominator")
	}
	return value
}

func capability(t Probe, typeID model.TypeID, kind model.TypeCapabilityKind) model.TypeCapability {
	t.Helper()
	value, ok := model.NewTypeCapability(typeID, kind)
	if !ok {
		t.Fatal("capture type capability")
	}
	return value
}

func content(t Probe, owner targetfixture.Identity, label string) identity.ContentID {
	t.Helper()
	value, ok := owner.Content(label)
	if !ok {
		t.Fatalf("capture content %q", label)
	}
	return value
}

func opaque(t Probe, denominator model.DenominatorRef, row model.RowID, column model.ColumnID, value binding.ValueToken) targetfixture.Cell {
	t.Helper()
	cell, ok := targetfixture.Opaque(denominator, row, column, value)
	if !ok {
		t.Fatal("capture opaque seed cell")
	}
	return cell
}

type uint64Equality struct{}

func (uint64Equality) Equal(left, right uint64) bool { return left == right }
