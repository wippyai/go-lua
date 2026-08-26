package relation_test

import (
	"testing"

	placementrelation "github.com/wippyai/go-lua/analysis/domain/placement/relation"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementcapture "github.com/wippyai/go-lua/domain/placement/capture"
	"github.com/wippyai/go-lua/domain/relationfixture"
)

type placementCaptureFixture struct {
	place         harness.Place
	operation     signature.Signature
	worker        binding.Worker
	placementType model.TypeID
	heapType      model.TypeID
	routeTagType  model.TypeID
	parent        model.ColumnID
	route         model.ColumnID
	routeTag      model.ColumnID
	current       model.ColumnID
	output        model.ColumnID
	placement     interface {
		Encode(binding.Issuer, placementdomain.Fact) (binding.ValueToken, bool)
		Decode(binding.ValueToken) (placementdomain.Fact, bool)
	}
	heap interface {
		Encode(binding.Issuer, heapdomain.Key) (binding.ValueToken, bool)
	}
	routeTags interface {
		Encode(binding.Issuer, uint64) (binding.ValueToken, bool)
	}
	root heapdomain.Key
}

func newPlacementCaptureFixture(t *testing.T) placementCaptureFixture {
	t.Helper()
	world := relationfixture.New(t)
	place := harness.New(t, "row/capture-parent", "row/capture-route", "row/capture-tag", "row/capture-current")
	placementType := place.TypeID(t, "type/placement")
	heapType := place.TypeID(t, "type/heap-candidate")
	routeTagType := place.TypeID(t, "type/route-tag")
	placementColumn := harness.NewColumn[placementdomain.Fact](t, placementType, "store/capture-placement", reserve)
	heapColumn := harness.NewColumn[heapdomain.Key](t, heapType, "store/capture-heap", reserve)
	routeTagColumn := harness.NewColumn[uint64](t, routeTagType, "store/capture-route-tag", reserve)
	columns, ok := placementrelation.NewPlacementCaptureColumns(placementColumn, heapColumn, routeTagColumn)
	if !ok {
		t.Fatal("capture owner columns")
	}
	parentAddress := place.Column(t, "column/capture-parent")
	routeAddress := place.Column(t, "column/capture-route")
	routeTagAddress := place.Column(t, "column/capture-route-tag")
	currentAddress := place.Column(t, "column/capture-current")
	outputAddress := place.Column(t, "column/capture-output")
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("capture cardinality")
	}
	operation := place.Seal(t, "operation/placement-capture",
		[]signature.Input{
			harness.ScalarInput(t, place.Relation, parentAddress, placementType, place.Denominator),
			harness.ScalarInput(t, place.Relation, routeAddress, heapType, place.Denominator),
			harness.ScalarInput(t, place.Relation, routeTagAddress, routeTagType, place.Denominator),
			harness.ScalarInput(t, place.Relation, currentAddress, placementType, place.Denominator),
		},
		[]signature.Output{{Relation: place.Relation, Column: outputAddress, Type: placementType, Presence: signature.ProducePresent, Denominator: place.Denominator}},
		cardinality, outcome.Produced, outcome.Refused,
	)
	factory, ok := placementrelation.BindPlacementCapture(operation, placementrelation.PlacementCaptureOperation{}, columns, place.Refusal)
	if !ok {
		t.Fatal("bind placement capture")
	}
	return placementCaptureFixture{
		place: place, operation: operation, worker: place.Worker(t, factory, operation),
		placementType: placementType, heapType: heapType, routeTagType: routeTagType,
		parent: parentAddress, route: routeAddress, routeTag: routeTagAddress,
		current: currentAddress, output: outputAddress,
		placement: placementColumn, heap: heapColumn, routeTags: routeTagColumn, root: world.Root,
	}
}

func (fixture placementCaptureFixture) evaluate(t *testing.T, slots ...binding.Slot) (outcome.Result, binding.ProposalBatch, bool) {
	t.Helper()
	frame := fixture.place.Frame(t, slots...)
	buffer := fixture.place.BufferAt(t, fixture.operation, fixture.place.Rows[3])
	result := fixture.worker.Evaluate(frame, buffer)
	batch, sealed := buffer.Seal(result)
	return result, batch, sealed
}

func (fixture placementCaptureFixture) presentSlots(t *testing.T, parent placementdomain.Fact, route heapdomain.Key, routeTag uint64, current placementdomain.Fact) (binding.Slot, binding.Slot, binding.Slot, binding.Slot) {
	t.Helper()
	parentToken, ok := fixture.placement.Encode(fixture.place.Issuer, parent)
	if !ok {
		t.Fatal("encode capture parent")
	}
	routeToken, ok := fixture.heap.Encode(fixture.place.Issuer, route)
	if !ok {
		t.Fatal("encode capture route")
	}
	routeTagToken, ok := fixture.routeTags.Encode(fixture.place.Issuer, routeTag)
	if !ok {
		t.Fatal("encode capture route tag")
	}
	currentToken, ok := fixture.placement.Encode(fixture.place.Issuer, current)
	if !ok {
		t.Fatal("encode capture current")
	}
	return harness.ScalarSlot(t, fixture.place.Cell(t, fixture.parent, fixture.place.Rows[0], fixture.placementType, parentToken)),
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.route, fixture.place.Rows[1], fixture.heapType, routeToken)),
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.routeTag, fixture.place.Rows[2], fixture.routeTagType, routeTagToken)),
		harness.ScalarSlot(t, fixture.place.Cell(t, fixture.current, fixture.place.Rows[3], fixture.placementType, currentToken))
}

func TestPlacementCaptureBindsRealFoldAtDeclaredRouteRow(t *testing.T) {
	fixture := newPlacementCaptureFixture(t)
	parent := placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven}
	current := placementdomain.DefaultFact()
	want, reduction := placementcapture.CaptureFold(parent, 1, current)
	if reduction != structure.Concrete {
		t.Fatalf("real CaptureFold reduction = %v, want Concrete", reduction)
	}
	parentSlot, routeSlot, routeTagSlot, currentSlot := fixture.presentSlots(t, parent, fixture.root, 1, current)
	result, batch, sealed := fixture.evaluate(t, parentSlot, routeSlot, routeTagSlot, currentSlot)
	if !sealed || result.Code != outcome.Produced || batch.Len() != 1 {
		t.Fatalf("capture binding = %v sealed=%t rows=%d, want one produced row", result.Code, sealed, batch.Len())
	}
	proposal, ok := batch.At(0)
	if !ok || proposal.Destination().Row() != fixture.place.Rows[3] || proposal.Destination().Column() != fixture.output {
		t.Fatal("capture binding did not publish at declared current/route row")
	}
	published, ok := fixture.placement.Decode(proposal.Value())
	if !ok || !placementdomain.EqualFact(published, want) {
		t.Fatalf("capture fact = %#v, want CaptureFold %#v", published, want)
	}
}

func TestPlacementCaptureRefusesAbsentForeignAndWrongRouteEvidence(t *testing.T) {
	fixture := newPlacementCaptureFixture(t)
	parent := placementdomain.Fact{Class: placementdomain.OwnedHeap, RetainEscape: placementdomain.EvidenceProven}
	current := placementdomain.DefaultFact()
	checkRefused := func(name string, slots ...binding.Slot) {
		t.Helper()
		result, batch, sealed := fixture.evaluate(t, slots...)
		if !sealed || result.Code != outcome.Refused || batch.Len() != 0 {
			t.Fatalf("%s = %v sealed=%t rows=%d, want refused without rows", name, result.Code, sealed, batch.Len())
		}
	}

	// Missing parent evidence must not become a fabricated Bottom/Unknown path.
	routeParent, route, routeTag, currentSlot := fixture.presentSlots(t, parent, fixture.root, 1, current)
	absentParent := harness.ScalarSlot(t, fixture.place.AbsentCell(t, fixture.parent, fixture.place.Rows[0], fixture.placementType))
	checkRefused("absent parent", absentParent, route, routeTag, currentSlot)

	// A token issued by another Heap store is foreign even when its TypeID is
	// identical; the owner codec must reject it before CaptureFold runs.
	foreignHeap := harness.NewColumn[heapdomain.Key](t, fixture.heapType, "store/capture-foreign-heap", reserve)
	foreignToken, ok := foreignHeap.Encode(fixture.place.Issuer, fixture.root)
	if !ok {
		t.Fatal("encode foreign capture route")
	}
	foreignRoute := harness.ScalarSlot(t, fixture.place.Cell(t, fixture.route, fixture.place.Rows[1], fixture.heapType, foreignToken))
	checkRefused("foreign route", routeParent, foreignRoute, routeTag, currentSlot)

	// A typed but unissued Heap key is also malformed route evidence; the
	// operation must not let the fold widen it into a valid capture.
	invalidRouteToken, ok := fixture.heap.Encode(fixture.place.Issuer, heapdomain.Key{})
	if !ok {
		t.Fatal("encode invalid capture route")
	}
	invalidRoute := harness.ScalarSlot(t, fixture.place.Cell(t, fixture.route, fixture.place.Rows[1], fixture.heapType, invalidRouteToken))
	checkRefused("invalid route key", routeParent, invalidRoute, routeTag, currentSlot)

	// Zero is a typed route tag but is not an authored route member.
	zeroTag, ok := fixture.routeTags.Encode(fixture.place.Issuer, 0)
	if !ok {
		t.Fatal("encode zero capture route tag")
	}
	wrongRouteTag := harness.ScalarSlot(t, fixture.place.Cell(t, fixture.routeTag, fixture.place.Rows[2], fixture.routeTagType, zeroTag))
	checkRefused("wrong route tag", routeParent, route, wrongRouteTag, currentSlot)
}
