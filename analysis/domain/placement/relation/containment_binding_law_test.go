package relation_test

import (
	"testing"

	placementrelation "github.com/wippyai/go-lua/analysis/domain/placement/relation"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/relbind"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementcontainment "github.com/wippyai/go-lua/domain/placement/containment"
)

type containmentBindingFixture struct {
	place         harness.Place
	operation     signature.Signature
	worker        binding.Worker
	placementType model.TypeID
	current       model.ColumnID
	parent        model.ColumnID
	output        model.ColumnID
	placement     *relbindgen.Column[placementdomain.Fact]
}

// TestPlacementContainmentCorpusKeepsCompleteVectorsOutOfTheFoldFrame
// records the ownership boundary: the route derivation consumes complete
// Placement and Heap deliveries, while this typed fold sees precisely the two
// scalar facts carried by the selected route tuple.
func TestPlacementContainmentCorpusKeepsCompleteVectorsOutOfTheFoldFrame(t *testing.T) {
	var containmentFound bool
	for _, family := range relbind.Declared().Families {
		if family.Census != "placement/containment" {
			continue
		}
		containmentFound = true
		if !family.Emitted() || family.Result != "placement" || len(family.Outputs) != 1 {
			t.Fatalf("containment family=%+v, want one emitted scalar placement binding", family)
		}
		if len(family.Inputs) != 2 {
			t.Fatalf("containment fold slots=%d, want only current and retained parent", len(family.Inputs))
		}
		wantFields := []string{"Current", "Parent"}
		for index, input := range family.Inputs {
			if input.Field != wantFields[index] || input.Payload != "placement" || input.Delivery != signature.ScalarDelivery {
				t.Fatalf("containment fold slot %d=%+v, want scalar %s Placement", index, input, wantFields[index])
			}
		}
	}
	if !containmentFound {
		t.Fatal("relbind corpus lacks placement/containment")
	}
}

func newContainmentBindingFixture(t *testing.T) containmentBindingFixture {
	t.Helper()
	place := harness.New(t, "row/containment-route")
	placementType := place.TypeID(t, "type/placement")
	placementColumn := harness.NewColumn[placementdomain.Fact](t, placementType, "store/containment-placement", reserve)
	columns, ok := placementrelation.NewPlacementContainmentColumns(placementColumn)
	if !ok {
		t.Fatal("containment owner column")
	}
	current := place.Column(t, "column/containment-route-current")
	parent := place.Column(t, "column/containment-route-parent")
	output := place.Column(t, "column/containment-output")
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("containment cardinality")
	}
	operation := place.Seal(t, "operation/placement-containment",
		[]signature.Input{
			harness.ScalarInput(t, place.Relation, current, placementType, place.Denominator),
			harness.ScalarInput(t, place.Relation, parent, placementType, place.Denominator),
		},
		[]signature.Output{{Relation: place.Relation, Column: output, Type: placementType, Presence: signature.ProducePresent, Denominator: place.Denominator}},
		cardinality, outcome.Produced, outcome.Refused,
	)
	factory, ok := placementrelation.BindPlacementContainment(operation, placementrelation.PlacementContainmentOperation{}, columns, place.Refusal)
	if !ok {
		t.Fatal("bind placement containment")
	}
	return containmentBindingFixture{
		place: place, operation: operation, worker: place.Worker(t, factory, operation),
		placementType: placementType, current: current, parent: parent, output: output, placement: placementColumn,
	}
}

func (fixture containmentBindingFixture) slots(t *testing.T, current, parent placementdomain.Fact) (binding.Slot, binding.Slot) {
	t.Helper()
	currentToken, ok := fixture.placement.Encode(fixture.place.Issuer, current)
	if !ok {
		t.Fatal("encode containment current")
	}
	parentToken, ok := fixture.placement.Encode(fixture.place.Issuer, parent)
	if !ok {
		t.Fatal("encode containment parent")
	}
	// J2 is repeated in the fold declaration: both scalar columns belong to
	// this one selected route row, rather than to separately rebuilt vectors.
	routeRow := fixture.place.Rows[0]
	return harness.ScalarSlot(t, fixture.place.Cell(t, fixture.current, routeRow, fixture.placementType, currentToken)),
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.parent, routeRow, fixture.placementType, parentToken))
}

func (fixture containmentBindingFixture) evaluate(t *testing.T, slots ...binding.Slot) (outcome.Result, binding.ProposalBatch, bool) {
	t.Helper()
	buffer := fixture.place.BufferAt(t, fixture.operation, fixture.place.Rows[0])
	result := fixture.worker.Evaluate(fixture.place.Frame(t, slots...), buffer)
	batch, sealed := buffer.Seal(result)
	return result, batch, sealed
}

// TestPlacementContainmentBindsRealFoldAtOneSelectedRouteRow proves the
// scalar bridge consumes the child and the parent retained by
// ContainmentRoutes, in ContainmentFold's declared order. The complete
// Placement and Heap inputs were consumed before this row existed.
func TestPlacementContainmentBindsRealFoldAtOneSelectedRouteRow(t *testing.T) {
	fixture := newContainmentBindingFixture(t)
	current := placementdomain.DefaultFact()
	parent := placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven}
	want, reduction := placementcontainment.ContainmentFold(current, parent)
	if reduction != structure.Concrete {
		t.Fatalf("real ContainmentFold reduction = %v, want Concrete", reduction)
	}
	currentSlot, parentSlot := fixture.slots(t, current, parent)
	result, batch, sealed := fixture.evaluate(t, currentSlot, parentSlot)
	if !sealed || result.Code != outcome.Produced || batch.Len() != 1 {
		t.Fatalf("containment binding = %v sealed=%t rows=%d, want one produced row", result.Code, sealed, batch.Len())
	}
	proposal, ok := batch.At(0)
	if !ok || proposal.Destination().Row() != fixture.place.Rows[0] || proposal.Destination().Column() != fixture.output {
		t.Fatal("containment binding did not publish at the selected child route row")
	}
	published, ok := fixture.placement.Decode(proposal.Value())
	if !ok || !placementdomain.EqualFact(published, want) {
		t.Fatalf("containment fact = %#v, want ContainmentFold %#v", published, want)
	}
}

// TestPlacementContainmentRefusesMissingForeignAndInvalidRouteFacts keeps the
// route row authoritative: neither a missing parent nor a compatible-looking
// token from another Placement store can be widened into a containment fact.
func TestPlacementContainmentRefusesMissingForeignAndInvalidRouteFacts(t *testing.T) {
	fixture := newContainmentBindingFixture(t)
	current := placementdomain.DefaultFact()
	parent := placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven}
	checkRefused := func(name string, slots ...binding.Slot) {
		t.Helper()
		result, batch, sealed := fixture.evaluate(t, slots...)
		if !sealed || result.Code != outcome.Refused || batch.Len() != 0 {
			t.Fatalf("%s = %v sealed=%t rows=%d, want refused without rows", name, result.Code, sealed, batch.Len())
		}
	}

	currentSlot, _ := fixture.slots(t, current, parent)
	absentParent := harness.ScalarSlot(t, fixture.place.AbsentCell(t, fixture.parent, fixture.place.Rows[0], fixture.placementType))
	checkRefused("absent retained parent", currentSlot, absentParent)

	foreign := harness.NewColumn[placementdomain.Fact](t, fixture.placementType, "store/containment-foreign-placement", reserve)
	foreignToken, ok := foreign.Encode(fixture.place.Issuer, parent)
	if !ok {
		t.Fatal("encode foreign containment parent")
	}
	foreignParent := harness.ScalarSlot(t, fixture.place.Cell(t, fixture.parent, fixture.place.Rows[0], fixture.placementType, foreignToken))
	checkRefused("foreign retained parent", currentSlot, foreignParent)

	bottomToken, ok := fixture.placement.Encode(fixture.place.Issuer, placementdomain.BottomFact())
	if !ok {
		t.Fatal("encode invalid containment current")
	}
	invalidCurrent := harness.ScalarSlot(t, fixture.place.Cell(t, fixture.current, fixture.place.Rows[0], fixture.placementType, bottomToken))
	_, parentSlot := fixture.slots(t, current, parent)
	checkRefused("invalid current", invalidCurrent, parentSlot)
}
