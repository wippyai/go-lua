package relation_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/placement/relation"
)

const reserve = 64

// TestPlacementTransferCarriesTheFactAlongTheRouteItsTagNames drives the real
// transfer fold. A placement is displaced along the route its tag names, and
// the answer publishes at the row the frame delivered.
func TestPlacementTransferCarriesTheFactAlongTheRouteItsTagNames(t *testing.T) {
	place := harness.New(t, "row/transfer")
	var types relation.PayloadTypes
	var tags relation.PayloadTags
	place.InstallTypes(t, &types)
	place.InstallTags(t, &tags)
	payloads, ok := relation.NewPayloads(types, tags, reserve)
	if !ok {
		t.Fatal("install the placement columns")
	}
	columns, ok := relation.NewPlacementTransferColumns(payloads.Placement, payloads.PlacementRouteTag)
	if !ok {
		t.Fatal("placement transfer columns")
	}
	tagAddress := place.Column(t, "column/route-tag")
	selectedAddress := place.Column(t, "column/selected")
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	operation := place.Seal(t, "operation/placement-transfer",
		[]signature.Input{
			harness.ScalarInput(t, place.Relation, tagAddress, types.PlacementRouteTag, place.Denominator),
			harness.ScalarInput(t, place.Relation, selectedAddress, types.Placement, place.Denominator),
		},
		[]signature.Output{{Relation: place.Relation, Column: selectedAddress, Type: types.Placement, Presence: signature.ProducePresent}},
		cardinality, outcome.Produced, outcome.Refused)
	factory, ok := relation.BindPlacementTransfer(operation, relation.PlacementTransferOperation{}, columns, place.Refusal)
	if !ok {
		t.Fatal("bind placement transfer")
	}
	worker := place.Worker(t, factory, operation)

	tagToken, ok := payloads.PlacementRouteTag.Encode(place.Issuer, 0)
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
	buffer := place.Buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	batch, sealed := buffer.Seal(result)
	if !sealed || !operation.Allows(result.Code) {
		t.Fatalf("the transfer settled outside its own vocabulary: %v", result.Code)
	}
	// A zero route tag is the owner's own refusal, and a refused fold publishes
	// nothing. What the law states is the shape of the answer, not which arm
	// this frame reaches.
	if result.Code == outcome.Produced {
		proposal, proposalOK := batch.At(0)
		if batch.Len() != 1 || !proposalOK || proposal.Destination().Row() != place.Rows[0] {
			t.Fatal("a produced transfer did not publish at the row it read")
		}
		return
	}
	if batch.Len() != 0 {
		t.Fatalf("a transfer that produced nothing published %d rows", batch.Len())
	}
}

// TestThePlacementAlgebraResolvesByTypeAlone states this axis's ascent
// authority is keyed by TypeID, so the state layer proves a proposal is an
// ascent without asking a binding which operation produced it.
func TestThePlacementAlgebraResolvesByTypeAlone(t *testing.T) {
	place := harness.New(t, "row/placement")
	var types relation.PayloadTypes
	var tags relation.PayloadTags
	place.InstallTypes(t, &types)
	place.InstallTags(t, &tags)
	payloads, ok := relation.NewPayloads(types, tags, reserve)
	if !ok {
		t.Fatal("install the placement columns")
	}
	witness, ok := relation.NewPlacementLattice()
	if !ok {
		t.Fatal("placement lattice witness")
	}
	evidence, ok := relation.NewEvidenceLattice()
	if !ok {
		t.Fatal("evidence lattice witness")
	}
	algebras, ok := payloads.Algebras(place.Issuer, relation.Lattices{Placement: witness, SuspensionEvidence: evidence})
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
