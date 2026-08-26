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
	"github.com/wippyai/go-lua/domain/relationfixture"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const reserve = 64

// TestPlacementAllocationBirthUsesTheValueIssuedReceiptAndFreshFact drives
// the smallest real cross-axis family. Value issues the allocation receipt
// and exact fresh fact; Placement owns the initial Fact it publishes.
func TestPlacementAllocationBirthUsesTheValueIssuedReceiptAndFreshFact(t *testing.T) {
	world := relationfixture.New(t)
	candidate, ok := world.Values.AllocationResultAt(0)
	if !ok {
		t.Fatal("the fixture has no issued allocation receipt")
	}
	allocated, ok := candidate.Fresh()
	if !ok {
		t.Fatal("the issued allocation receipt has no fresh fact")
	}

	place := harness.New(t, "row/allocation-birth")
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
	candidateType := place.TypeID(t, "type/allocation-candidate")
	valueType := place.TypeID(t, "type/value")
	candidateColumn := harness.NewColumn[*valuedomain.AllocationResult](t, candidateType, "store/allocation-candidate", reserve)
	valueColumn := harness.NewColumn[valuedomain.Value](t, valueType, "store/value", reserve)
	columns, ok := relation.NewPlacementAllocationBirthColumns(valueColumn, payloads.Placement, candidateColumn)
	if !ok {
		t.Fatal("placement allocation-birth columns")
	}
	candidateAddress := place.Column(t, "column/allocation-candidate")
	allocatedAddress := place.Column(t, "column/allocated")
	placementAddress := place.Column(t, "column/placement")
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	operation := place.Seal(t, "operation/placement-allocation-birth",
		[]signature.Input{
			harness.ScalarInput(t, place.Relation, candidateAddress, candidateType, place.Denominator),
			harness.ScalarInput(t, place.Relation, allocatedAddress, valueType, place.Denominator),
		},
		[]signature.Output{{Relation: place.Relation, Column: placementAddress, Type: types.Placement, Presence: signature.ProducePresent, Denominator: place.Denominator}},
		cardinality, outcome.Produced, outcome.Refused)
	factory, ok := relation.BindPlacementAllocationBirth(operation, relation.PlacementAllocationBirthOperation{}, columns, place.Refusal)
	if !ok {
		t.Fatal("bind placement allocation birth")
	}
	worker := place.Worker(t, factory, operation)

	candidateToken, ok := candidateColumn.Encode(place.Issuer, candidate)
	if !ok {
		t.Fatal("encode allocation candidate")
	}
	allocatedToken, ok := valueColumn.Encode(place.Issuer, allocated)
	if !ok {
		t.Fatal("encode fresh value")
	}
	frame := place.Frame(t,
		harness.ScalarSlot(t, place.Cell(t, candidateAddress, place.Rows[0], candidateType, candidateToken)),
		harness.ScalarSlot(t, place.Cell(t, allocatedAddress, place.Rows[0], valueType, allocatedToken)),
	)
	buffer := place.BufferAt(t, operation, place.Rows[0])
	result := worker.Evaluate(frame, buffer)
	batch, sealed := buffer.Seal(result)
	if !sealed || !operation.Allows(result.Code) {
		t.Fatalf("the allocation-birth judgment settled outside its own vocabulary: %v", result.Code)
	}
	if result.Code != outcome.Produced || batch.Len() != 1 {
		t.Fatalf("allocation birth = %v with %d rows, want one produced row", result.Code, batch.Len())
	}
	proposal, proposalOK := batch.At(0)
	if !proposalOK || proposal.Destination().Row() != place.Rows[0] {
		t.Fatal("allocation birth did not publish at the candidate row")
	}
	published, ok := payloads.Placement.Decode(proposal.Value())
	if !ok || !placementdomain.EqualFact(published, placementdomain.DefaultFact()) {
		t.Fatalf("allocation birth fact = %#v, want %#v", published, placementdomain.DefaultFact())
	}
}

// TestPlacementFormalCarriesItsSelectedFactAtTheDeclaredRow drives the
// first scalar joined Placement family. The formal route's authenticated
// unknown tag widens the selected fact through the owner fold and publishes
// at the selected-fact row, never at a relation chosen by the bridge.
func TestPlacementFormalCarriesItsSelectedFactAtTheDeclaredRow(t *testing.T) {
	place := harness.New(t, "row/formal")
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
	columns, ok := relation.NewPlacementFormalColumns(payloads.Placement, payloads.PlacementRouteTag)
	if !ok {
		t.Fatal("placement formal columns")
	}
	tagAddress := place.Column(t, "column/route-tag")
	selectedAddress := place.Column(t, "column/selected")
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	operation := place.Seal(t, "operation/placement-formal",
		[]signature.Input{
			harness.ScalarInput(t, place.Relation, tagAddress, types.PlacementRouteTag, place.Denominator),
			harness.ScalarInput(t, place.Relation, selectedAddress, types.Placement, place.Denominator),
		},
		[]signature.Output{{Relation: place.Relation, Column: selectedAddress, Type: types.Placement, Presence: signature.ProducePresent, Denominator: place.Denominator}},
		cardinality, outcome.Produced, outcome.Refused)
	factory, ok := relation.BindPlacementFormal(operation, relation.PlacementFormalOperation{}, columns, place.Refusal)
	if !ok {
		t.Fatal("bind placement formal")
	}
	worker := place.Worker(t, factory, operation)

	// Formal's one-bit coordinate plus its authenticated unknown code is a
	// valid widening route. The code is owner mathematics; this bridge merely
	// carries the typed scalar it was delivered.
	tagToken, ok := payloads.PlacementRouteTag.Encode(place.Issuer, 0x1f)
	if !ok {
		t.Fatal("encode route tag")
	}
	selectedToken, ok := payloads.Placement.Encode(place.Issuer, placementdomain.DefaultFact())
	if !ok {
		t.Fatal("encode selected placement")
	}
	frame := place.Frame(t,
		harness.ScalarSlot(t, place.Cell(t, tagAddress, place.Rows[0], types.PlacementRouteTag, tagToken)),
		harness.ScalarSlot(t, place.Cell(t, selectedAddress, place.Rows[0], types.Placement, selectedToken)),
	)
	buffer := place.BufferAt(t, operation, place.Rows[0])
	result := worker.Evaluate(frame, buffer)
	batch, sealed := buffer.Seal(result)
	if !sealed || result.Code != outcome.Produced || batch.Len() != 1 {
		t.Fatalf("formal = %v with %d rows, want one produced row", result.Code, batch.Len())
	}
	proposal, proposalOK := batch.At(0)
	if !proposalOK || proposal.Destination().Row() != place.Rows[0] {
		t.Fatal("formal did not publish at the selected-fact row")
	}
	published, ok := payloads.Placement.Decode(proposal.Value())
	if !ok || !placementdomain.EqualFact(published, placementdomain.UnknownFact()) {
		t.Fatalf("formal fact = %#v, want %#v", published, placementdomain.UnknownFact())
	}
}

// TestThePlacementAlgebraResolvesByTypeAlone states this axis's ascent
// authority is keyed by TypeID, so the state layer proves a proposal is an
// ascent without asking a binding which operation produced it.
func TestThePlacementAlgebraResolvesByTypeAlone(t *testing.T) {
	place := harness.New(t, "row/placement")
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
	witness, ok := relation.NewPlacementLattice()
	if !ok {
		t.Fatal("placement lattice witness")
	}
	algebras, ok := payloads.Algebras(place.Issuer, relation.Lattices{Placement: witness})
	if !ok {
		t.Fatal("the placement axis stated no ascent authority")
	}
	// The law asks that the axis's own fact resolves its authority, not how
	// many columns the axis happens to declare an order for.
	var resolved binding.ValueAlgebra
	for _, algebra := range algebras {
		if algebra.Type() == types.Placement {
			resolved = algebra
		}
	}
	if resolved == nil {
		t.Fatal("the placement fact TypeID resolved no ascent authority")
	}
	bottom, ok := payloads.Placement.Encode(place.Issuer, placementdomain.BottomFact())
	if !ok {
		t.Fatal("encode placement bottom")
	}
	top, ok := payloads.Placement.Encode(place.Issuer, placementdomain.UnknownFact())
	if !ok {
		t.Fatal("encode placement top")
	}
	joined, ok := resolved.Join(bottom, top)
	if !ok || !resolved.LessOrEqual(bottom, joined) || !resolved.LessOrEqual(top, joined) {
		t.Fatal("the placement join was not an upper bound of both operands")
	}
	if resolved.LessOrEqual(joined, bottom) {
		t.Fatal("the placement order collapsed top into bottom")
	}
}

// TestThePlacementBoundaryDoesNotAllocate holds the generic boundary to zero
// allocations for this axis's own payload.
func TestThePlacementBoundaryDoesNotAllocate(t *testing.T) {
	place := harness.New(t, "row/placement")
	column := harness.NewColumn[placementdomain.Fact](t, place.TypeID(t, "type/placement"), "store/placement", 1<<20)
	top := placementdomain.UnknownFact()
	if allocations := testing.AllocsPerRun(200, func() {
		token, ok := column.Encode(place.Issuer, top)
		if !ok {
			t.Fatal("encode placement fact")
		}
		if _, ok := column.Decode(token); !ok {
			t.Fatal("decode placement fact")
		}
	}); allocations != 0 {
		t.Fatalf("the placement boundary allocated %.0f times", allocations)
	}
}
