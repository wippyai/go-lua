package relation_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	"github.com/wippyai/go-lua/domain/heap/relation"
	"github.com/wippyai/go-lua/domain/relationfixture"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const reserve = 64

func exactlyOne(t testing.TB) model.Cardinality {
	t.Helper()
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	return cardinality
}

// TestHeapAscentProposesAnAscentOfTheCellItRead drives the cell-update arm
// against the real heap lattice. An update is publication to an existing
// authenticated row, and the row is the one the frame delivered.
func TestHeapAscentProposesAnAscentOfTheCellItRead(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/cell")
	heapType := place.TypeID(t, "type/heap")
	column := harness.NewColumn[heapdomain.Value](t, heapType, "store/heap", reserve)
	cellColumn := place.Column(t, "column/heap-cell")
	proposedColumn := place.Column(t, "column/heap-proposed")
	columns, ok := relation.NewHeapAscentColumns(column)
	if !ok {
		t.Fatal("heap ascent columns")
	}
	operation := place.Seal(t, "operation/heap-ascent",
		[]signature.Input{
			harness.ScalarInput(t, place.Relation, cellColumn, heapType, place.Denominator),
			harness.ScalarInput(t, place.Relation, proposedColumn, heapType, place.Denominator),
		},
		[]signature.Output{{Relation: place.Relation, Column: cellColumn, Type: heapType, Presence: signature.ProducePresent}},
		exactlyOne(t), outcome.Produced, outcome.Refused)
	factory, ok := relation.BindHeapAscent(operation, relation.HeapAscentOperation{}, columns, place.Refusal)
	if !ok {
		t.Fatal("bind heap ascent")
	}
	worker := place.Worker(t, factory, operation)

	current := fixture.Heap.Bottom()
	proposed := fixture.Heap.Top()
	currentToken, ok := column.Encode(place.Issuer, current)
	if !ok {
		t.Fatal("encode current heap cell")
	}
	proposedToken, ok := column.Encode(place.Issuer, proposed)
	if !ok {
		t.Fatal("encode proposed heap fact")
	}
	frame := place.Frame(t,
		harness.ScalarSlot(t, place.Cell(t, cellColumn, place.Rows[0], heapType, currentToken)),
		harness.ScalarSlot(t, place.Cell(t, proposedColumn, place.Rows[0], heapType, proposedToken)),
	)
	buffer := place.Buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	batch, sealed := buffer.Seal(result)
	if result.Code != outcome.Produced || !sealed || batch.Len() != 1 {
		t.Fatalf("heap ascent outcome=%v sealed=%t rows=%d", result.Code, sealed, batch.Len())
	}
	proposal, ok := batch.At(0)
	if !ok || proposal.Destination().Row() != place.Rows[0] || proposal.Destination().Column() != cellColumn {
		t.Fatal("heap ascent did not propose at the cell it read")
	}
	ascended, ok := column.Decode(proposal.Value())
	if !ok || !heapdomain.LessOrEq(current, ascended) || !heapdomain.LessOrEq(proposed, ascended) {
		t.Fatal("the update proposal was not an ascent of the cell it read")
	}
}

// TestReceiverRoutesPublishTheRootsTheOwnerNames drives the finite-expansion
// arm against the real sealed topology. The receiver observes exactly the
// table root the program allocates, and the binding publishes at the row that
// root's own content identity names.
func TestReceiverRoutesPublishTheRootsTheOwnerNames(t *testing.T) {
	fixture := relationfixture.New(t)
	rootKey, ok := fixture.Root.ContentID()
	if !ok {
		t.Fatal("fixture root content identity")
	}
	place := harness.NewKeyed(t, []identity.ContentID{rootKey})
	receiverType := place.TypeID(t, "type/value")
	routeType := place.TypeID(t, "type/route")
	receiverColumn := harness.NewColumn[valuedomain.Value](t, receiverType, "store/value", reserve)
	routeStore := harness.NewColumn[relation.HeapRouteFact](t, routeType, "store/route", reserve)
	columns, ok := relation.NewHeapReceiverRoutesColumns(receiverColumn, routeStore)
	if !ok {
		t.Fatal("receiver route columns")
	}
	receiverAddress := place.Column(t, "column/receiver")
	routeAddress := place.Column(t, "column/route")
	many, ok := model.NewCardinality(model.BoundedMany, 64)
	if !ok {
		t.Fatal("cardinality")
	}
	operation := place.Seal(t, "operation/receiver-routes",
		[]signature.Input{harness.ScalarInput(t, place.Relation, receiverAddress, receiverType, place.Denominator)},
		[]signature.Output{{Relation: place.Relation, Column: routeAddress, Type: routeType, Presence: signature.ProducePresent}},
		many, outcome.Produced, outcome.NoCandidate, outcome.Refused)
	judgment, ok := relation.NewHeapReceiverRoutesOperation(fixture.Topology)
	if !ok {
		t.Fatal("receiver route judgment")
	}
	factory, ok := relation.BindHeapReceiverRoutes(operation, judgment, columns, place.Refusal)
	if !ok {
		t.Fatal("bind receiver routes")
	}
	worker := place.Worker(t, factory, operation)

	token, ok := receiverColumn.Encode(place.Issuer, fixture.Receiver)
	if !ok {
		t.Fatal("encode receiver")
	}
	frame := place.Frame(t, harness.ScalarSlot(t, place.Cell(t, receiverAddress, place.Rows[0], receiverType, token)))
	buffer := place.Buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	batch, sealed := buffer.Seal(result)
	if result.Code != outcome.Produced || !sealed || batch.Len() != 1 {
		t.Fatalf("route expansion outcome=%v sealed=%t rows=%d", result.Code, sealed, batch.Len())
	}
	proposal, ok := batch.At(0)
	if !ok || proposal.Destination().Row() != place.Rows[0] {
		t.Fatal("the expansion did not publish at the row the owner named")
	}
	fact, ok := routeStore.Decode(proposal.Value())
	if !ok || fact.Kind != indexdomain.RouteRoot {
		t.Fatalf("published route fact %v ok=%t", fact.Kind, ok)
	}

	// Bottom observes nothing, and that is a distinct outcome rather than a
	// fabricated bottom row.
	bottomToken, ok := receiverColumn.Encode(place.Issuer, fixture.Values.Bottom())
	if !ok {
		t.Fatal("encode bottom receiver")
	}
	if !buffer.Reset() {
		t.Fatal("buffer reset")
	}
	bottomFrame := place.Frame(t, harness.ScalarSlot(t, place.Cell(t, receiverAddress, place.Rows[0], receiverType, bottomToken)))
	bottomResult := worker.Evaluate(bottomFrame, buffer)
	bottomBatch, bottomSealed := buffer.Seal(bottomResult)
	if bottomResult.Code != outcome.NoCandidate || !bottomSealed || bottomBatch.Len() != 0 {
		t.Fatalf("empty observation outcome=%v rows=%d", bottomResult.Code, bottomBatch.Len())
	}
}

// TestHeapSeedsAnswerTheKeyTheyWereGiven drives the two heap seeds, which are
// the scalar-judgment arm at its smallest: one candidate in, one fact at the
// candidate's own row, and the owner's own disposition when there is none.
func TestHeapSeedsAnswerTheKeyTheyWereGiven(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/key")
	heapType := place.TypeID(t, "type/heap")
	keyType := place.TypeID(t, "type/heap-key")
	factColumn := harness.NewColumn[heapdomain.Value](t, heapType, "store/heap", reserve)
	keyColumn := harness.NewColumn[heapdomain.Key](t, keyType, "store/heap-key", reserve)
	columns, ok := relation.NewHeapIngressColumns(factColumn, keyColumn)
	if !ok {
		t.Fatal("heap ingress columns")
	}
	keyAddress := place.Column(t, "column/key")
	factAddress := place.Column(t, "column/fact")
	operation := place.Seal(t, "operation/heap-ingress",
		[]signature.Input{harness.ScalarInput(t, place.Relation, keyAddress, keyType, place.Denominator)},
		[]signature.Output{{Relation: place.Relation, Column: factAddress, Type: heapType, Presence: signature.ProducePresent}},
		exactlyOne(t), outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	factory, ok := relation.BindHeapIngress(operation, relation.HeapIngressOperation{}, columns, place.Refusal)
	if !ok {
		t.Fatal("bind heap ingress")
	}
	worker := place.Worker(t, factory, operation)

	token, ok := keyColumn.Encode(place.Issuer, fixture.Root)
	if !ok {
		t.Fatal("encode heap key")
	}
	frame := place.Frame(t, harness.ScalarSlot(t, place.Cell(t, keyAddress, place.Rows[0], keyType, token)))
	buffer := place.Buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	batch, sealed := buffer.Seal(result)
	if !sealed || !operation.Allows(result.Code) {
		t.Fatalf("ingress seed settled outside its own vocabulary: %v", result.Code)
	}
	if result.Code == outcome.Produced {
		proposal, proposalOK := batch.At(0)
		if batch.Len() != 1 || !proposalOK || proposal.Destination().Row() != place.Rows[0] {
			t.Fatal("a produced seed did not publish at the candidate's own row")
		}
		return
	}
	if batch.Len() != 0 {
		t.Fatalf("a seed that produced nothing published %d rows", batch.Len())
	}
}

// TestABindingRefusesAForeignSignature states the admission fence. A factory
// answers for exactly the contract it was constructed with, so a drifted or
// foreign signature is refused without any inspection of what the family does.
func TestABindingRefusesAForeignSignature(t *testing.T) {
	place := harness.New(t, "row/cell")
	heapType := place.TypeID(t, "type/heap")
	column := harness.NewColumn[heapdomain.Value](t, heapType, "store/heap", reserve)
	columns, ok := relation.NewHeapAscentColumns(column)
	if !ok {
		t.Fatal("heap ascent columns")
	}
	cellColumn := place.Column(t, "column/heap-cell")
	inputs := []signature.Input{
		harness.ScalarInput(t, place.Relation, cellColumn, heapType, place.Denominator),
		harness.ScalarInput(t, place.Relation, place.Column(t, "column/heap-proposed"), heapType, place.Denominator),
	}
	outputs := []signature.Output{{Relation: place.Relation, Column: cellColumn, Type: heapType, Presence: signature.ProducePresent}}
	operation := place.Seal(t, "operation/heap-ascent", inputs, outputs, exactlyOne(t), outcome.Produced, outcome.Refused)
	foreign := place.Seal(t, "operation/foreign", inputs, outputs, exactlyOne(t), outcome.Produced, outcome.Refused)
	factory, ok := relation.BindHeapAscent(operation, relation.HeapAscentOperation{}, columns, place.Refusal)
	if !ok {
		t.Fatal("bind heap ascent")
	}
	if _, admitted := binding.Admit(factory, foreign); admitted {
		t.Fatal("a factory admitted a signature it was not constructed with")
	}
	if _, admitted := binding.Admit(factory, operation); !admitted {
		t.Fatal("a factory refused its own signature")
	}
}

// TestTheHeapBoundaryDoesNotAllocate holds the generic boundary to zero
// allocations. Owner mathematics allocates whatever it allocates; the boundary
// that carries its values into generic storage adds nothing.
func TestTheHeapBoundaryDoesNotAllocate(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/cell")
	column := harness.NewColumn[heapdomain.Value](t, place.TypeID(t, "type/heap"), "store/heap", 1<<20)
	top := fixture.Heap.Top()
	if allocations := testing.AllocsPerRun(200, func() {
		token, ok := column.Encode(place.Issuer, top)
		if !ok {
			t.Fatal("encode heap value")
		}
		if _, ok := column.Decode(token); !ok {
			t.Fatal("decode heap value")
		}
	}); allocations != 0 {
		t.Fatalf("the heap boundary allocated %.0f times", allocations)
	}
}

// TestTheHeapAlgebraResolvesByTypeAlone states that ascent authority is keyed
// by TypeID and not by the operation that produced a value, which is what lets
// the state layer prove a proposal is an ascent without asking a binding.
func TestTheHeapAlgebraResolvesByTypeAlone(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/cell")
	var types relation.PayloadTypes
	var tags relation.PayloadTags
	place.InstallTypes(t, &types)
	place.InstallTags(t, &tags)
	heapType := types.Heap
	payloads, ok := relation.NewPayloads(types, tags, reserve)
	if !ok {
		t.Fatal("install the heap columns")
	}
	witness, ok := relation.NewHeapLattice()
	if !ok {
		t.Fatal("heap lattice witness")
	}
	algebras, ok := payloads.Algebras(place.Issuer, relation.Lattices{Heap: witness})
	if !ok || len(algebras) != 1 || algebras[0].Type() != heapType {
		t.Fatal("the heap axis did not state one ascent authority for its own TypeID")
	}
	bottom, ok := payloads.Heap.Encode(place.Issuer, fixture.Heap.Bottom())
	if !ok {
		t.Fatal("encode heap bottom")
	}
	top, ok := payloads.Heap.Encode(place.Issuer, fixture.Heap.Top())
	if !ok {
		t.Fatal("encode heap top")
	}
	joined, ok := algebras[0].Join(bottom, top)
	if !ok || !algebras[0].LessOrEqual(bottom, joined) || !algebras[0].LessOrEqual(top, joined) {
		t.Fatal("the heap join was not an upper bound of both operands")
	}
	if algebras[0].LessOrEqual(joined, bottom) {
		t.Fatal("the heap order collapsed top into bottom")
	}
}
