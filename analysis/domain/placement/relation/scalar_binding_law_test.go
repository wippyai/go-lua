package relation_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/placement/relation"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

type placementScalarFixture struct {
	place     harness.Place
	types     relation.PayloadTypes
	payloads  relation.Payloads
	operation signature.Signature
	factory   binding.Factory
	current   model.ColumnID
	routeTag  model.ColumnID
	require   model.ColumnID
}

func newPlacementScalarWorld(t *testing.T, label string) (harness.Place, relation.PayloadTypes, relation.Payloads) {
	t.Helper()
	place := harness.New(t, label)
	types := relation.PayloadTypes{
		Placement:         place.TypeID(t, "type/placement"),
		Requirement:       place.TypeID(t, "type/requirement"),
		PlacementRouteTag: place.TypeID(t, "type/route-tag"),
	}
	tags := relation.PayloadTags{
		Placement:         harness.Content(t, "store/placement"),
		Requirement:       harness.Content(t, "store/requirement"),
		PlacementRouteTag: harness.Content(t, "store/route-tag"),
	}
	payloads, ok := relation.NewPayloads(types, tags, reserve)
	if !ok {
		t.Fatal("install the placement columns")
	}
	return place, types, payloads
}

func (fixture placementScalarFixture) evaluate(t *testing.T, slots ...binding.Slot) (outcome.Result, binding.ProposalBatch, bool) {
	t.Helper()
	worker := fixture.place.Worker(t, fixture.factory, fixture.operation)
	buffer := fixture.place.BufferAt(t, fixture.operation, fixture.place.Rows[0])
	result := worker.Evaluate(fixture.place.Frame(t, slots...), buffer)
	batch, sealed := buffer.Seal(result)
	return result, batch, sealed
}

func requirePlacementProposal(t *testing.T, fixture placementScalarFixture, batch binding.ProposalBatch, want placementdomain.Fact) {
	t.Helper()
	if batch.Len() != 1 {
		t.Fatalf("published %d rows, want one", batch.Len())
	}
	proposal, ok := batch.At(0)
	if !ok || proposal.Destination().Row() != fixture.place.Rows[0] {
		t.Fatal("binding did not publish at the delivered row")
	}
	published, ok := fixture.payloads.Placement.Decode(proposal.Value())
	if !ok || !placementdomain.EqualFact(published, want) {
		t.Fatalf("published fact = %#v, want %#v", published, want)
	}
}

func newPlacementPublicationEscapeFixture(t *testing.T) placementScalarFixture {
	place, types, payloads := newPlacementScalarWorld(t, "row/publication-escape")
	requireAddress := place.Column(t, "column/requirement")
	currentAddress := place.Column(t, "column/current")
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	operation := place.Seal(t, "operation/placement-publication-escape",
		[]signature.Input{
			harness.ScalarInput(t, place.Relation, requireAddress, types.Requirement, place.Denominator),
			harness.ScalarInput(t, place.Relation, currentAddress, types.Placement, place.Denominator),
		},
		[]signature.Output{{Relation: place.Relation, Column: currentAddress, Type: types.Placement, Presence: signature.ProducePresent, Denominator: place.Denominator}},
		cardinality, outcome.Produced, outcome.Refused)
	columns, ok := relation.NewPlacementPublicationEscapeColumns(payloads.Placement, payloads.Requirement)
	if !ok {
		t.Fatal("placement publication-escape columns")
	}
	factory, ok := relation.BindPlacementPublicationEscape(operation, relation.PlacementPublicationEscapeOperation{}, columns, place.Refusal)
	if !ok {
		t.Fatal("bind placement publication escape")
	}
	return placementScalarFixture{
		place: place, types: types, payloads: payloads, operation: operation, factory: factory,
		current: currentAddress, require: requireAddress,
	}
}

func newPlacementReturnEscapeFixture(t *testing.T) placementScalarFixture {
	place, types, payloads := newPlacementScalarWorld(t, "row/return-escape")
	routeAddress := place.Column(t, "column/route-tag")
	currentAddress := place.Column(t, "column/current")
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	operation := place.Seal(t, "operation/placement-return-escape",
		[]signature.Input{
			harness.ScalarInput(t, place.Relation, routeAddress, types.PlacementRouteTag, place.Denominator),
			harness.ScalarInput(t, place.Relation, currentAddress, types.Placement, place.Denominator),
		},
		[]signature.Output{{Relation: place.Relation, Column: currentAddress, Type: types.Placement, Presence: signature.ProducePresent, Denominator: place.Denominator}},
		cardinality, outcome.Produced, outcome.Refused)
	columns, ok := relation.NewPlacementReturnEscapeColumns(payloads.Placement, payloads.PlacementRouteTag)
	if !ok {
		t.Fatal("placement return-escape columns")
	}
	factory, ok := relation.BindPlacementReturnEscape(operation, relation.PlacementReturnEscapeOperation{}, columns, place.Refusal)
	if !ok {
		t.Fatal("bind placement return escape")
	}
	return placementScalarFixture{
		place: place, types: types, payloads: payloads, operation: operation, factory: factory,
		current: currentAddress, routeTag: routeAddress,
	}
}

func newPlacementTransferFixture(t *testing.T) placementScalarFixture {
	place, types, payloads := newPlacementScalarWorld(t, "row/transfer")
	routeAddress := place.Column(t, "column/route-tag")
	selectedAddress := place.Column(t, "column/selected")
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	operation := place.Seal(t, "operation/placement-transfer",
		[]signature.Input{
			harness.ScalarInput(t, place.Relation, routeAddress, types.PlacementRouteTag, place.Denominator),
			harness.ScalarInput(t, place.Relation, selectedAddress, types.Placement, place.Denominator),
		},
		[]signature.Output{{Relation: place.Relation, Column: selectedAddress, Type: types.Placement, Presence: signature.ProducePresent, Denominator: place.Denominator}},
		cardinality, outcome.Produced, outcome.Refused)
	columns, ok := relation.NewPlacementTransferColumns(payloads.Placement, payloads.PlacementRouteTag)
	if !ok {
		t.Fatal("placement transfer columns")
	}
	factory, ok := relation.BindPlacementTransfer(operation, relation.PlacementTransferOperation{}, columns, place.Refusal)
	if !ok {
		t.Fatal("bind placement transfer")
	}
	return placementScalarFixture{
		place: place, types: types, payloads: payloads, operation: operation, factory: factory,
		current: selectedAddress, routeTag: routeAddress,
	}
}

// TestPlacementPublicationEscapeBindsTypedRequirementAndFact proves that the
// bridge delivers only its declared Requirement and Fact scalars to the owner
// fold, and publishes that fold's authenticated displacement at the same row.
func TestPlacementPublicationEscapeBindsTypedRequirementAndFact(t *testing.T) {
	fixture := newPlacementPublicationEscapeFixture(t)
	requirement, ok := fixture.payloads.Requirement.Encode(fixture.place.Issuer, placementdomain.OwnedHeap)
	if !ok {
		t.Fatal("encode publication requirement")
	}
	current, ok := fixture.payloads.Placement.Encode(fixture.place.Issuer, placementdomain.DefaultFact())
	if !ok {
		t.Fatal("encode current placement")
	}
	result, batch, sealed := fixture.evaluate(t,
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.require, fixture.place.Rows[0], fixture.types.Requirement, requirement)),
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.current, fixture.place.Rows[0], fixture.types.Placement, current)),
	)
	if !sealed || result.Code != outcome.Produced {
		t.Fatalf("publication escape = %v, want produced", result.Code)
	}
	requirePlacementProposal(t, fixture, batch, placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven})
}

// TestPlacementPublicationEscapeRefusesNearestInvalidRequirement keeps an
// authored Stack requirement from becoming an Unknown fallback at the bridge.
func TestPlacementPublicationEscapeRefusesNearestInvalidRequirement(t *testing.T) {
	fixture := newPlacementPublicationEscapeFixture(t)
	requirement, ok := fixture.payloads.Requirement.Encode(fixture.place.Issuer, placementdomain.Stack)
	if !ok {
		t.Fatal("encode invalid publication requirement")
	}
	current, ok := fixture.payloads.Placement.Encode(fixture.place.Issuer, placementdomain.DefaultFact())
	if !ok {
		t.Fatal("encode current placement")
	}
	result, batch, sealed := fixture.evaluate(t,
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.require, fixture.place.Rows[0], fixture.types.Requirement, requirement)),
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.current, fixture.place.Rows[0], fixture.types.Placement, current)),
	)
	if !sealed || result.Code != outcome.Refused || batch.Len() != 0 {
		t.Fatalf("invalid publication requirement = %v with %d rows, want refused with no rows", result.Code, batch.Len())
	}
}

// TestPlacementReturnEscapeBindsTypedRouteAndFact proves that a declared
// nonzero route tag is carried to ReturnEscapeFold without route inference.
func TestPlacementReturnEscapeBindsTypedRouteAndFact(t *testing.T) {
	fixture := newPlacementReturnEscapeFixture(t)
	route, ok := fixture.payloads.PlacementRouteTag.Encode(fixture.place.Issuer, 1)
	if !ok {
		t.Fatal("encode return route tag")
	}
	current, ok := fixture.payloads.Placement.Encode(fixture.place.Issuer, placementdomain.DefaultFact())
	if !ok {
		t.Fatal("encode current placement")
	}
	result, batch, sealed := fixture.evaluate(t,
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.routeTag, fixture.place.Rows[0], fixture.types.PlacementRouteTag, route)),
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.current, fixture.place.Rows[0], fixture.types.Placement, current)),
	)
	if !sealed || result.Code != outcome.Produced {
		t.Fatalf("return escape = %v, want produced", result.Code)
	}
	requirePlacementProposal(t, fixture, batch, placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven})
}

// TestPlacementReturnEscapeRefusesMissingRoute keeps an absent route witness
// from fabricating a return displacement or an Unknown fact.
func TestPlacementReturnEscapeRefusesMissingRoute(t *testing.T) {
	fixture := newPlacementReturnEscapeFixture(t)
	route, ok := fixture.payloads.PlacementRouteTag.Encode(fixture.place.Issuer, 0)
	if !ok {
		t.Fatal("encode missing return route tag")
	}
	current, ok := fixture.payloads.Placement.Encode(fixture.place.Issuer, placementdomain.DefaultFact())
	if !ok {
		t.Fatal("encode current placement")
	}
	result, batch, sealed := fixture.evaluate(t,
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.routeTag, fixture.place.Rows[0], fixture.types.PlacementRouteTag, route)),
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.current, fixture.place.Rows[0], fixture.types.Placement, current)),
	)
	if !sealed || result.Code != outcome.Refused || batch.Len() != 0 {
		t.Fatalf("missing return route = %v with %d rows, want refused with no rows", result.Code, batch.Len())
	}
}

// TestPlacementTransferBindsTypedRouteAndFact proves that TransferFold owns
// the Send displacement while the bridge transports only RouteTag and Fact.
func TestPlacementTransferBindsTypedRouteAndFact(t *testing.T) {
	fixture := newPlacementTransferFixture(t)
	routeTag := uint64(1)<<4 | uint64(placementdomain.Send+1)
	route, ok := fixture.payloads.PlacementRouteTag.Encode(fixture.place.Issuer, routeTag)
	if !ok {
		t.Fatal("encode transfer route tag")
	}
	selected, ok := fixture.payloads.Placement.Encode(fixture.place.Issuer, placementdomain.DefaultFact())
	if !ok {
		t.Fatal("encode selected placement")
	}
	result, batch, sealed := fixture.evaluate(t,
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.routeTag, fixture.place.Rows[0], fixture.types.PlacementRouteTag, route)),
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.current, fixture.place.Rows[0], fixture.types.Placement, selected)),
	)
	if !sealed || result.Code != outcome.Produced {
		t.Fatalf("transfer = %v, want produced", result.Code)
	}
	requirePlacementProposal(t, fixture, batch, placementdomain.Fact{Class: placementdomain.SharedHeap, RetainEscape: placementdomain.EvidenceProven})
}

// TestPlacementTransferRefusesNearestWrongEscapeRoute keeps a typed route for
// another escape from being treated as a Transfer route by fallback logic.
func TestPlacementTransferRefusesNearestWrongEscapeRoute(t *testing.T) {
	fixture := newPlacementTransferFixture(t)
	routeTag := uint64(1)<<4 | uint64(placementdomain.Retain+1)
	route, ok := fixture.payloads.PlacementRouteTag.Encode(fixture.place.Issuer, routeTag)
	if !ok {
		t.Fatal("encode wrong transfer route tag")
	}
	selected, ok := fixture.payloads.Placement.Encode(fixture.place.Issuer, placementdomain.DefaultFact())
	if !ok {
		t.Fatal("encode selected placement")
	}
	result, batch, sealed := fixture.evaluate(t,
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.routeTag, fixture.place.Rows[0], fixture.types.PlacementRouteTag, route)),
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.current, fixture.place.Rows[0], fixture.types.Placement, selected)),
	)
	if !sealed || result.Code != outcome.Refused || batch.Len() != 0 {
		t.Fatalf("wrong transfer route = %v with %d rows, want refused with no rows", result.Code, batch.Len())
	}
}
