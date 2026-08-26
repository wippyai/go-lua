package relation_test

import (
	"testing"

	placementrelation "github.com/wippyai/go-lua/analysis/domain/placement/relation"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/relbind"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementstore "github.com/wippyai/go-lua/domain/placement/store"
	"github.com/wippyai/go-lua/domain/relationfixture"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// opaqueStorageCandidate delivers Value's sealed candidate through the
// AuthenticatedOpaque carrier. Store may decode the typed receipt but receives
// no lattice authority for it.
func opaqueStorageCandidate(t testing.TB, place harness.Place, column model.ColumnID, row model.RowID, typeID model.TypeID, value binding.ValueToken) binding.Slot {
	t.Helper()
	address, ok := place.Issuer.IssueCell(place.Witness, place.Scope, column, row)
	if !ok {
		t.Fatal("issue opaque storage candidate cell")
	}
	presence, ok := model.NewPresence(model.AuthenticatedOpaque)
	if !ok {
		t.Fatal("storage candidate opaque presence")
	}
	cell, ok := binding.NewCell(address, typeID, value, presence)
	if !ok {
		t.Fatal("construct opaque storage candidate cell")
	}
	return harness.ScalarSlot(t, cell)
}

func issuedStorageTransfer(t testing.TB, values *valuedomain.Schema) valuedomain.StorageTransfer {
	t.Helper()
	for index := 0; index < values.StorageTransferCount(); index++ {
		candidate, ok := values.StorageTransferAt(index)
		if ok && candidate.Persistent() {
			return candidate
		}
	}
	t.Fatal("real Value fixture has no persistent StorageTransfer candidate")
	return valuedomain.StorageTransfer{}
}

// TestPlacementStorageCandidateIsAnOpaqueCodecOnly keeps the Value receipt on
// its owner-issued codec path. Store consumes it as authenticated evidence;
// a candidate column is not a Placement or Value lattice cell.
func TestPlacementStorageCandidateIsAnOpaqueCodecOnly(t *testing.T) {
	corpus := relbind.Declared()
	candidate, ok := corpus.Payload("storage-transfer-candidate")
	if !ok || candidate.Axis != "value" || candidate.Field != "StorageTransferCandidate" || candidate.Ascends() || candidate.Lattice != "" {
		t.Fatalf("storage candidate payload = %+v, want a Value-owned codec-only receipt", candidate)
	}
	for _, family := range corpus.Families {
		if family.Stem != "PlacementStorage" {
			continue
		}
		if !family.Emitted() || len(family.Inputs) != 4 || family.Inputs[0].Payload != "storage-transfer-candidate" {
			t.Fatalf("storage binding declaration = %+v, want its four sealed inputs", family)
		}
		return
	}
	t.Fatal("storage family is absent from the binding corpus")
}

// TestPlacementStorageBindsTheSealedFourInputStoreFold proves the first
// multi-input Placement binding. The candidate and source come from Value's
// real sealed schema, while the selected fact's row is the only row the
// generated binding may address. The route relation already selected that
// fact; this bridge does not construct or rediscover a route.
func TestPlacementStorageBindsTheSealedFourInputStoreFold(t *testing.T) {
	world := relationfixture.New(t)
	candidate := issuedStorageTransfer(t, world.Values)
	source := world.Receiver
	const routeTag uint64 = 1
	selected := placementdomain.DefaultFact()
	want, reduction := placementstore.StorageFold(candidate, source, routeTag, selected)
	if reduction != structure.Concrete {
		t.Fatalf("real Store fold reduction = %v, want concrete", reduction)
	}

	place := harness.New(t, "row/storage-selected")
	candidateType := place.TypeID(t, "type/storage-transfer-candidate")
	valueType := place.TypeID(t, "type/value")
	routeType := place.TypeID(t, "type/placement-route-tag")
	placementType := place.TypeID(t, "type/placement")
	candidateColumn := harness.NewColumn[valuedomain.StorageTransfer](t, candidateType, "store/storage-transfer-candidate", reserve)
	valueColumn := harness.NewColumn[valuedomain.Value](t, valueType, "store/value", reserve)
	routeColumn := harness.NewColumn[uint64](t, routeType, "store/route-tag", reserve)
	placementColumn := harness.NewColumn[placementdomain.Fact](t, placementType, "store/placement", reserve)
	columns, ok := placementrelation.NewPlacementStorageColumns(valueColumn, placementColumn, candidateColumn, routeColumn)
	if !ok {
		t.Fatal("storage owner columns")
	}

	candidateAddress := place.Column(t, "column/storage-transfer-candidate")
	sourceAddress := place.Column(t, "column/value-source")
	routeAddress := place.Column(t, "column/route-tag")
	selectedAddress := place.Column(t, "column/selected-placement")
	outputAddress := place.Column(t, "column/placement-output")
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("storage cardinality")
	}
	candidateDelivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("storage candidate delivery")
	}
	operation := place.Seal(t, "operation/placement-storage",
		[]signature.Input{
			{Relation: place.Relation, Column: candidateAddress, Type: candidateType, Presence: signature.RequireOpaque, Delivery: candidateDelivery, Denominator: place.Denominator},
			harness.ScalarInput(t, place.Relation, sourceAddress, valueType, place.Denominator),
			harness.ScalarInput(t, place.Relation, routeAddress, routeType, place.Denominator),
			harness.ScalarInput(t, place.Relation, selectedAddress, placementType, place.Denominator),
		},
		[]signature.Output{{Relation: place.Relation, Column: outputAddress, Type: placementType, Presence: signature.ProducePresent, Denominator: place.Denominator}},
		cardinality, outcome.Produced, outcome.Refused,
	)
	factory, ok := placementrelation.BindPlacementStorage(operation, placementrelation.PlacementStorageOperation{}, columns, place.Refusal)
	if !ok {
		t.Fatal("bind placement storage")
	}
	worker := place.Worker(t, factory, operation)

	evaluate := func(candidate valuedomain.StorageTransfer, tag uint64, fact placementdomain.Fact) (outcome.Result, binding.ProposalBatch, bool) {
		t.Helper()
		candidateToken, ok := candidateColumn.Encode(place.Issuer, candidate)
		if !ok {
			t.Fatal("encode storage candidate")
		}
		sourceToken, ok := valueColumn.Encode(place.Issuer, source)
		if !ok {
			t.Fatal("encode storage source")
		}
		routeToken, ok := routeColumn.Encode(place.Issuer, tag)
		if !ok {
			t.Fatal("encode storage route tag")
		}
		selectedToken, ok := placementColumn.Encode(place.Issuer, fact)
		if !ok {
			t.Fatal("encode selected placement")
		}
		frame := place.Frame(t,
			opaqueStorageCandidate(t, place, candidateAddress, place.Rows[0], candidateType, candidateToken),
			harness.ScalarSlot(t, place.Cell(t, sourceAddress, place.Rows[0], valueType, sourceToken)),
			harness.ScalarSlot(t, place.Cell(t, routeAddress, place.Rows[0], routeType, routeToken)),
			harness.ScalarSlot(t, place.Cell(t, selectedAddress, place.Rows[0], placementType, selectedToken)),
		)
		buffer := place.BufferAt(t, operation, place.Rows[0])
		result := worker.Evaluate(frame, buffer)
		batch, sealed := buffer.Seal(result)
		return result, batch, sealed
	}

	result, batch, sealed := evaluate(candidate, routeTag, selected)
	if !sealed || result.Code != outcome.Produced || batch.Len() != 1 {
		t.Fatalf("storage binding = %v sealed=%t rows=%d, want one produced row", result.Code, sealed, batch.Len())
	}
	proposal, ok := batch.At(0)
	if !ok || proposal.Destination().Row() != place.Rows[0] || proposal.Destination().Column() != outputAddress {
		t.Fatal("storage binding did not publish at the selected row and declared Placement output")
	}
	published, ok := placementColumn.Decode(proposal.Value())
	if !ok || !placementdomain.EqualFact(published, want) {
		t.Fatalf("storage binding fact = %#v, want StoreFold %#v", published, want)
	}

	// The nearest malformed route witness is refused by StoreFold rather than
	// being interpreted as an empty route or widened to Unknown.
	result, batch, sealed = evaluate(candidate, 0, selected)
	if !sealed || result.Code != outcome.Refused || batch.Len() != 0 {
		t.Fatalf("zero storage route = %v sealed=%t rows=%d, want refused without rows", result.Code, sealed, batch.Len())
	}

	// A typed codec can carry an invalid receipt, but it cannot mint an
	// owner-issued candidate or acquire an ascent algebra. The owner fold
	// therefore refuses it without publishing a fallback fact.
	result, batch, sealed = evaluate(valuedomain.StorageTransfer{}, routeTag, selected)
	if !sealed || result.Code != outcome.Refused || batch.Len() != 0 {
		t.Fatalf("forged storage candidate = %v sealed=%t rows=%d, want refused without rows", result.Code, sealed, batch.Len())
	}
}
