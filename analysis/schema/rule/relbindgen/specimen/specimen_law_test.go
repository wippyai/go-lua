package specimen_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/specimen"
	calldomain "github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/domain/value/moduleload"
)

func newColumn[T any](t testing.TB, typeID model.TypeID, label string, reserve int) *relbindgen.Column[T] {
	t.Helper()
	store, ok := relbindgen.NewStore[T](content(t, label), reserve)
	if !ok {
		t.Fatalf("store %q", label)
	}
	column, ok := relbindgen.NewColumn(typeID, store)
	if !ok {
		t.Fatalf("column %q", label)
	}
	return column
}

// TestOwnerLatticesReachOneGenericSurface is the TypeID-algebra specimen: two
// owners whose ascent APIs have nothing in common - correlated value states it
// as methods on a sealed *Schema, heap states it as package functions over
// values that carry their own owner - resolve through one binding.ValueAlgebra
// keyed by TypeID alone, with the payload never boxed.
func TestOwnerLatticesReachOneGenericSurface(t *testing.T) {
	fixture := newDomainFixture(t)
	place := newHarness(t, "row/one")

	valueType := place.typeID(t, "type/value")
	heapType := place.typeID(t, "type/heap")
	valueColumn := newColumn[valuedomain.Value](t, valueType, "store/value", 64)
	heapColumn := newColumn[heapdomain.Value](t, heapType, "store/heap", 64)

	valueWitness, ok := specimen.NewValueLattice(fixture.values)
	if !ok {
		t.Fatal("value lattice witness")
	}
	valueAlgebra, ok := relbindgen.NewAlgebra[valuedomain.Value, specimen.ValueLattice](valueColumn, place.issuer, valueWitness)
	if !ok {
		t.Fatal("value algebra")
	}
	heapAlgebra, ok := relbindgen.NewAlgebra[heapdomain.Value, specimen.HeapLattice](heapColumn, place.issuer, specimen.HeapLattice{})
	if !ok {
		t.Fatal("heap algebra")
	}

	registry := typedRegistry{algebras: []binding.ValueAlgebra{valueAlgebra, heapAlgebra}}
	if resolved, resolveOK := binding.ResolveAlgebra(registry, valueType); !resolveOK || resolved.Type() != valueType {
		t.Fatal("value TypeID did not resolve its own algebra")
	}
	if resolved, resolveOK := binding.ResolveAlgebra(registry, heapType); !resolveOK || resolved.Type() != heapType {
		t.Fatal("heap TypeID did not resolve its own algebra")
	}
	if _, resolveOK := binding.ResolveAlgebra(registry, place.typeID(t, "type/unowned")); resolveOK {
		t.Fatal("an unowned TypeID resolved an algebra")
	}

	valueBottom, ok := valueColumn.Encode(place.issuer, fixture.values.Bottom())
	if !ok {
		t.Fatal("encode value bottom")
	}
	valueTop, ok := valueColumn.Encode(place.issuer, fixture.values.Top())
	if !ok {
		t.Fatal("encode value top")
	}
	joined, ok := valueAlgebra.Join(valueBottom, valueTop)
	if !ok || !valueAlgebra.LessOrEqual(valueBottom, joined) || !valueAlgebra.LessOrEqual(valueTop, joined) {
		t.Fatal("correlated value join was not an upper bound of both operands")
	}
	if valueAlgebra.LessOrEqual(joined, valueBottom) {
		t.Fatal("correlated value order collapsed top into bottom")
	}

	heapBottom, ok := heapColumn.Encode(place.issuer, fixture.heap.Bottom())
	if !ok {
		t.Fatal("encode heap bottom")
	}
	heapTop, ok := heapColumn.Encode(place.issuer, fixture.heap.Top())
	if !ok {
		t.Fatal("encode heap top")
	}
	heapJoined, ok := heapAlgebra.Join(heapBottom, heapTop)
	if !ok || !heapAlgebra.LessOrEqual(heapBottom, heapJoined) || !heapAlgebra.LessOrEqual(heapTop, heapJoined) {
		t.Fatal("heap join was not an upper bound of both operands")
	}
	if heapAlgebra.LessOrEqual(heapJoined, heapBottom) {
		t.Fatal("heap order collapsed top into bottom")
	}

	// Owner payloads never cross as one another: a value token refuses at the
	// heap column and the reverse, even though both are the same generic
	// surface.
	if _, crossed := heapColumn.Decode(valueBottom); crossed {
		t.Fatal("a correlated value token decoded as a heap value")
	}
	if _, crossed := valueColumn.Decode(heapBottom); crossed {
		t.Fatal("a heap value token decoded as a correlated value")
	}
}

// TestHeterogeneousStorageBoundaryDoesNotAllocate holds the generic boundary
// to zero allocations for both owner payload types. Owner mathematics
// allocates whatever it allocates; the boundary that carries its values into
// generic storage adds nothing.
func TestHeterogeneousStorageBoundaryDoesNotAllocate(t *testing.T) {
	fixture := newDomainFixture(t)
	place := newHarness(t, "row/one")
	valueColumn := newColumn[valuedomain.Value](t, place.typeID(t, "type/value"), "store/value", 1<<20)
	heapColumn := newColumn[heapdomain.Value](t, place.typeID(t, "type/heap"), "store/heap", 1<<20)
	valueTop := fixture.values.Top()
	heapTop := fixture.heap.Top()

	if allocations := testing.AllocsPerRun(200, func() {
		token, ok := valueColumn.Encode(place.issuer, valueTop)
		if !ok {
			t.Fatal("encode correlated value")
		}
		if _, ok := valueColumn.Decode(token); !ok {
			t.Fatal("decode correlated value")
		}
	}); allocations != 0 {
		t.Fatalf("correlated value boundary allocated %.0f times", allocations)
	}

	if allocations := testing.AllocsPerRun(200, func() {
		token, ok := heapColumn.Encode(place.issuer, heapTop)
		if !ok {
			t.Fatal("encode heap value")
		}
		if _, ok := heapColumn.Decode(token); !ok {
			t.Fatal("decode heap value")
		}
	}); allocations != 0 {
		t.Fatalf("heap value boundary allocated %.0f times", allocations)
	}
}

// TestReceiverRouteExpansionPublishesTheRootsTheOwnerNames drives the real
// sealed topology. The receiver observes exactly the table root the program
// allocates, and the binding publishes at the row that root's own content
// identity names.
func TestReceiverRouteExpansionPublishesTheRootsTheOwnerNames(t *testing.T) {
	fixture := newDomainFixture(t)
	rootKey, ok := fixture.root.ContentID()
	if !ok {
		t.Fatal("fixture root content identity")
	}
	place := newHarnessKeyed(t, []identity.ContentID{rootKey})
	receiverType := place.typeID(t, "type/value")
	routeType := place.typeID(t, "type/route")
	columns := specimen.ReceiverRouteColumns{
		Receiver: newColumn[valuedomain.Value](t, receiverType, "store/value", 64),
		Route:    newColumn[specimen.RouteFact](t, routeType, "store/route", 64),
	}
	receiverColumn := place.column(t, "column/receiver")
	routeColumn := place.column(t, "column/route")
	many, ok := model.NewCardinality(model.BoundedMany, 4)
	if !ok {
		t.Fatal("cardinality")
	}
	operation := place.seal(t, "operation/receiver-routes",
		[]signature.Input{scalarInput(t, place.relation, receiverColumn, receiverType, place.denominator)},
		[]signature.Output{{Relation: place.relation, Column: routeColumn, Type: routeType, Presence: signature.ProducePresent}},
		many, outcome.Produced, outcome.NoCandidate, outcome.Refused)
	factory, ok := specimen.BindReceiverRoutes(operation, fixture.topology, columns, place.refusal)
	if !ok {
		t.Fatal("bind receiver routes")
	}
	worker := place.worker(t, factory, operation)

	token, ok := columns.Receiver.Encode(place.issuer, fixture.receiver)
	if !ok {
		t.Fatal("encode receiver")
	}
	frame := place.frame(t, scalarSlot(t, place.cell(t, receiverColumn, place.rows[0], receiverType, token)))
	buffer := place.buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	batch, sealed := buffer.Seal(result)
	if result.Code != outcome.Produced || !sealed || batch.Len() != 1 {
		t.Fatalf("route expansion outcome=%v sealed=%t rows=%d", result.Code, sealed, batch.Len())
	}
	proposal, ok := batch.At(0)
	if !ok || proposal.Destination().Row() != place.rows[0] {
		t.Fatal("route expansion did not publish at the row the owner named")
	}
	fact, ok := columns.Route.Decode(proposal.Value())
	if !ok || fact.Kind != indexdomain.RouteRoot {
		t.Fatalf("published route fact %v ok=%t", fact.Kind, ok)
	}

	// Bottom observes nothing, and that is a distinct outcome, never a
	// fabricated bottom row.
	bottomToken, ok := columns.Receiver.Encode(place.issuer, fixture.values.Bottom())
	if !ok {
		t.Fatal("encode bottom receiver")
	}
	if !buffer.Reset() {
		t.Fatal("buffer reset")
	}
	bottomFrame := place.frame(t, scalarSlot(t, place.cell(t, receiverColumn, place.rows[0], receiverType, bottomToken)))
	bottomResult := worker.Evaluate(bottomFrame, buffer)
	bottomBatch, bottomSealed := buffer.Seal(bottomResult)
	if bottomResult.Code != outcome.NoCandidate || !bottomSealed || bottomBatch.Len() != 0 {
		t.Fatalf("empty observation outcome=%v rows=%d", bottomResult.Code, bottomBatch.Len())
	}
}

// TestValueSummaryFoldsTheCompleteDeliveredGroup drives the real coordinatewise
// summary fold over a complete span, with an absent coordinate delivered as
// absent rather than as a stored domain default.
func TestValueSummaryFoldsTheCompleteDeliveredGroup(t *testing.T) {
	fixture := newDomainFixture(t)
	coordinates := fixture.values.CoordinateCount()
	if coordinates < 2 {
		t.Fatalf("fixture exposes %d coordinates", coordinates)
	}
	keys := make([]identity.ContentID, 0, coordinates)
	for index := 0; index < coordinates; index++ {
		keys = append(keys, content(t, fmt.Sprintf("row/coordinate-%d", index)))
	}
	place := newHarnessKeyed(t, keys)
	cellType := place.typeID(t, "type/value")
	observationType := place.typeID(t, "type/value-summary")
	columns := specimen.ValueSummaryColumns{
		Cell:        newColumn[valuedomain.Value](t, cellType, "store/value", 256),
		Observation: newColumn[valuedomain.ValueSummaryObservation](t, observationType, "store/value-summary", 16),
	}
	cellColumn := place.column(t, "column/cell")
	groupColumn := place.column(t, "column/group")
	observationColumn := place.column(t, "column/observation")
	exact, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	operation := place.seal(t, "operation/value-summary",
		[]signature.Input{
			completeInput(t, place.relation, cellColumn, cellType, place.denominator),
			scalarInput(t, place.relation, groupColumn, cellType, place.denominator),
		},
		[]signature.Output{{Relation: place.relation, Column: observationColumn, Type: observationType, Presence: signature.ProducePresent}},
		exact, outcome.Produced, outcome.NoSelection, outcome.Refused)
	factory, ok := specimen.BindValueSummary(operation, fixture.values, columns, place.refusal)
	if !ok {
		t.Fatal("bind value summary")
	}
	worker := place.worker(t, factory, operation)

	top := fixture.values.Top()
	cells := make([]binding.Cell, 0, coordinates)
	for index, row := range place.rows {
		if index == 1 {
			cells = append(cells, place.absentCell(t, cellColumn, row, cellType))
			continue
		}
		token, encodeOK := columns.Cell.Encode(place.issuer, top)
		if !encodeOK {
			t.Fatal("encode coordinate")
		}
		cells = append(cells, place.cell(t, cellColumn, row, cellType, token))
	}
	groupToken, ok := columns.Cell.Encode(place.issuer, top)
	if !ok {
		t.Fatal("encode group address")
	}
	frame := place.frame(t, spanSlot(t, cells), scalarSlot(t, place.cell(t, groupColumn, place.rows[0], cellType, groupToken)))
	buffer := place.buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	batch, sealed := buffer.Seal(result)
	if result.Code != outcome.Produced || !sealed || batch.Len() != 1 {
		t.Fatalf("summary fold outcome=%v sealed=%t rows=%d", result.Code, sealed, batch.Len())
	}
	proposal, ok := batch.At(0)
	if !ok || proposal.Destination().Row() != place.rows[0] {
		t.Fatal("summary fold did not publish at the group row it was given")
	}
	observation, ok := columns.Observation.Decode(proposal.Value())
	if !ok || !observation.Valid || observation.Rows != 1 {
		t.Fatalf("published observation valid=%t rows=%d ok=%t", observation.Valid, observation.Rows, ok)
	}
	if !fixture.values.OwnsSummaryObservation(observation) {
		t.Fatal("published observation was not owned by the sealed schema")
	}
	if observation.Present[1] {
		t.Fatal("an absent coordinate was folded as a stored domain default")
	}
	if !observation.Present[0] {
		t.Fatal("a present coordinate was dropped by the fold")
	}
}

// TestHeapAscentProposesAnAscentOfTheCellItRead drives the real heap lattice.
func TestHeapAscentProposesAnAscentOfTheCellItRead(t *testing.T) {
	fixture := newDomainFixture(t)
	place := newHarness(t, "row/cell")
	heapType := place.typeID(t, "type/heap")
	columns := specimen.HeapAscentColumns{
		Cell:     newColumn[heapdomain.Value](t, heapType, "store/heap", 64),
		Proposed: nil,
	}
	columns.Proposed = columns.Cell
	cellColumn := place.column(t, "column/heap-cell")
	proposedColumn := place.column(t, "column/heap-proposed")
	exact, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	operation := place.seal(t, "operation/heap-ascent",
		[]signature.Input{
			scalarInput(t, place.relation, cellColumn, heapType, place.denominator),
			scalarInput(t, place.relation, proposedColumn, heapType, place.denominator),
		},
		[]signature.Output{{Relation: place.relation, Column: cellColumn, Type: heapType, Presence: signature.ProducePresent}},
		exact, outcome.Produced, outcome.Refused)
	factory, ok := specimen.BindHeapAscent(operation, columns, place.refusal)
	if !ok {
		t.Fatal("bind heap ascent")
	}
	worker := place.worker(t, factory, operation)

	current := fixture.heap.Bottom()
	proposed := fixture.heap.Top()
	currentToken, ok := columns.Cell.Encode(place.issuer, current)
	if !ok {
		t.Fatal("encode current heap cell")
	}
	proposedToken, ok := columns.Proposed.Encode(place.issuer, proposed)
	if !ok {
		t.Fatal("encode proposed heap fact")
	}
	frame := place.frame(t,
		scalarSlot(t, place.cell(t, cellColumn, place.rows[0], heapType, currentToken)),
		scalarSlot(t, place.cell(t, proposedColumn, place.rows[0], heapType, proposedToken)),
	)
	buffer := place.buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	batch, sealed := buffer.Seal(result)
	if result.Code != outcome.Produced || !sealed || batch.Len() != 1 {
		t.Fatalf("heap ascent outcome=%v sealed=%t rows=%d", result.Code, sealed, batch.Len())
	}
	proposal, ok := batch.At(0)
	if !ok || proposal.Destination().Row() != place.rows[0] || proposal.Destination().Column() != cellColumn {
		t.Fatal("heap ascent did not propose at the cell it read")
	}
	ascended, ok := columns.Cell.Decode(proposal.Value())
	if !ok || !heapdomain.LessOrEq(current, ascended) || !heapdomain.LessOrEq(proposed, ascended) {
		t.Fatal("heap update proposal was not an ascent of the cell it read")
	}
}

// TestModuleLoadJudgmentRefusesAnUnownedCandidate drives the real module-load
// judgment. The fixture program declares no module load, so the reachable arm
// is the judgment's own refusal; the binding carries it as a closed refusal
// with the owner's reason and publishes nothing.
func TestModuleLoadJudgmentRefusesAnUnownedCandidate(t *testing.T) {
	fixture := newDomainFixture(t)
	judgment, ok := moduleload.Derive(fixture.values)
	if !ok || !judgment.Valid() {
		t.Fatal("derive module-load judgment")
	}
	place := newHarness(t, "row/candidate")
	candidateType := place.typeID(t, "type/module-load-call")
	valueType := place.typeID(t, "type/value")
	callType := place.typeID(t, "type/call")
	columns := specimen.ModuleLoadColumns{
		Candidate:  newColumn[valuedomain.ModuleLoadCall](t, candidateType, "store/module-load-call", 16),
		Argument:   newColumn[valuedomain.Value](t, valueType, "store/value", 16),
		Dispatched: newColumn[calldomain.Value](t, callType, "store/call", 16),
		Result:     nil,
	}
	columns.Result = columns.Argument
	candidateColumn := place.column(t, "column/candidate")
	argumentColumn := place.column(t, "column/argument")
	dispatchedColumn := place.column(t, "column/dispatched")
	resultColumn := place.column(t, "column/result")
	exact, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	operation := place.seal(t, "operation/module-load",
		[]signature.Input{
			scalarInput(t, place.relation, candidateColumn, candidateType, place.denominator),
			scalarInput(t, place.relation, argumentColumn, valueType, place.denominator),
			scalarInput(t, place.relation, dispatchedColumn, callType, place.denominator),
		},
		[]signature.Output{{Relation: place.relation, Column: resultColumn, Type: valueType, Presence: signature.ProducePresent}},
		exact, outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	factory, ok := specimen.BindModuleLoad(operation, judgment, columns, place.refusal)
	if !ok {
		t.Fatal("bind module load")
	}
	worker := place.worker(t, factory, operation)

	candidateToken, ok := columns.Candidate.Encode(place.issuer, valuedomain.ModuleLoadCall{})
	if !ok {
		t.Fatal("encode candidate")
	}
	argumentToken, ok := columns.Argument.Encode(place.issuer, fixture.values.Bottom())
	if !ok {
		t.Fatal("encode argument")
	}
	dispatchedToken, ok := columns.Dispatched.Encode(place.issuer, calldomain.Value{})
	if !ok {
		t.Fatal("encode dispatched call")
	}
	frame := place.frame(t,
		scalarSlot(t, place.cell(t, candidateColumn, place.rows[0], candidateType, candidateToken)),
		scalarSlot(t, place.cell(t, argumentColumn, place.rows[0], valueType, argumentToken)),
		scalarSlot(t, place.cell(t, dispatchedColumn, place.rows[0], callType, dispatchedToken)),
	)
	buffer := place.buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	if result.Code != outcome.Refused || result.RefusalID != place.refusal {
		t.Fatalf("unowned candidate settled as %v", result.Code)
	}
	batch, sealed := buffer.Seal(result)
	if !sealed || batch.Len() != 0 {
		t.Fatal("a refused judgment published a row")
	}
}

type typedRegistry struct {
	algebras []binding.ValueAlgebra
}

func (registry typedRegistry) Resolve(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	for _, algebra := range registry.algebras {
		if algebra.Type() == typeID {
			return algebra, true
		}
	}
	return nil, false
}
