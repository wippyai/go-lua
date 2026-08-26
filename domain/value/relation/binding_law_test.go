package relation_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/relationfixture"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/domain/value/relation"
)

const reserve = 256

func exactlyOne(t testing.TB) model.Cardinality {
	t.Helper()
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	return cardinality
}

// TestModuleLoadAnswersWithTheOwnersOwnDisposition drives the scalar-judgment
// arm against the real module-load judgment. The fixture program declares no
// module load, so the reachable arm is the judgment's own refusal: the binding
// carries it as a closed refusal with the owner's reason and publishes nothing.
func TestModuleLoadAnswersWithTheOwnersOwnDisposition(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/candidate")
	valueType := place.TypeID(t, "type/value")
	callType := place.TypeID(t, "type/call")
	candidateType := place.TypeID(t, "type/module-load-call")
	valueColumn := harness.NewColumn[valuedomain.Value](t, valueType, "store/value", reserve)
	callColumn := harness.NewColumn[calldomain.Value](t, callType, "store/call", reserve)
	candidateColumn := harness.NewColumn[valuedomain.ModuleLoadCall](t, candidateType, "store/module-load-call", reserve)
	columns, ok := relation.NewValueModuleLoadColumns(valueColumn, callColumn, candidateColumn)
	if !ok {
		t.Fatal("module load columns")
	}
	candidateAddress := place.Column(t, "column/candidate")
	argumentAddress := place.Column(t, "column/argument")
	dispatchedAddress := place.Column(t, "column/dispatched")
	resultAddress := place.Column(t, "column/result")
	operation := place.Seal(t, "operation/module-load",
		[]signature.Input{
			harness.ScalarInput(t, place.Relation, candidateAddress, candidateType, place.Denominator),
			harness.ScalarInput(t, place.Relation, argumentAddress, valueType, place.Denominator),
			harness.ScalarInput(t, place.Relation, dispatchedAddress, callType, place.Denominator),
		},
		[]signature.Output{{Relation: place.Relation, Column: resultAddress, Type: valueType, Presence: signature.ProducePresent}},
		exactlyOne(t), outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	judgment, ok := relation.NewValueModuleLoadOperation(fixture.Values)
	if !ok {
		t.Fatal("module load judgment")
	}
	factory, ok := relation.BindValueModuleLoad(operation, judgment, columns, place.Refusal)
	if !ok {
		t.Fatal("bind module load")
	}
	worker := place.Worker(t, factory, operation)

	candidateToken, ok := candidateColumn.Encode(place.Issuer, valuedomain.ModuleLoadCall{})
	if !ok {
		t.Fatal("encode candidate")
	}
	argumentToken, ok := valueColumn.Encode(place.Issuer, fixture.Values.Bottom())
	if !ok {
		t.Fatal("encode argument")
	}
	dispatchedToken, ok := callColumn.Encode(place.Issuer, calldomain.Value{})
	if !ok {
		t.Fatal("encode dispatched call")
	}
	frame := place.Frame(t,
		harness.ScalarSlot(t, place.Cell(t, candidateAddress, place.Rows[0], candidateType, candidateToken)),
		harness.ScalarSlot(t, place.Cell(t, argumentAddress, place.Rows[0], valueType, argumentToken)),
		harness.ScalarSlot(t, place.Cell(t, dispatchedAddress, place.Rows[0], callType, dispatchedToken)),
	)
	buffer := place.Buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	if result.Code != outcome.Refused || result.RefusalID != place.Refusal {
		t.Fatalf("an unowned candidate settled as %v", result.Code)
	}
	batch, sealed := buffer.Seal(result)
	if !sealed || batch.Len() != 0 {
		t.Fatal("a refused judgment published a row")
	}
}

// TestValueSummaryFoldsTheCompleteDeliveredGroup drives the grouped-reduction
// arm against the real coordinatewise fold. The span is delivered complete, an
// absent coordinate arrives absent rather than as a stored domain default, and
// the one folded row publishes at the group row the frame addressed.
func TestValueSummaryFoldsTheCompleteDeliveredGroup(t *testing.T) {
	fixture := relationfixture.New(t)
	coordinates := fixture.Values.CoordinateCount()
	if coordinates < 2 {
		t.Fatalf("the fixture exposes %d coordinates", coordinates)
	}
	keys := make([]identity.ContentID, 0, coordinates)
	for index := 0; index < coordinates; index++ {
		keys = append(keys, harness.Content(t, fmt.Sprintf("row/coordinate-%d", index)))
	}
	place := harness.NewKeyed(t, keys)
	valueType := place.TypeID(t, "type/value")
	summaryType := place.TypeID(t, "type/value-summary")
	valueColumn := harness.NewColumn[valuedomain.Value](t, valueType, "store/value", reserve)
	summaryColumn := harness.NewColumn[valuedomain.ValueSummaryObservation](t, summaryType, "store/value-summary", reserve)
	columns, ok := relation.NewValueSummaryColumns(valueColumn, summaryColumn)
	if !ok {
		t.Fatal("value summary columns")
	}
	cellAddress := place.Column(t, "column/cell")
	groupAddress := place.Column(t, "column/group")
	observationAddress := place.Column(t, "column/observation")
	operation := place.Seal(t, "operation/value-summary",
		[]signature.Input{
			harness.CompleteInput(t, place.Relation, cellAddress, valueType, place.Denominator),
			harness.ScalarInput(t, place.Relation, groupAddress, valueType, place.Denominator),
		},
		[]signature.Output{{Relation: place.Relation, Column: observationAddress, Type: summaryType, Presence: signature.ProducePresent}},
		exactlyOne(t), outcome.Produced, outcome.NoSelection, outcome.Refused)
	judgment, ok := relation.NewValueSummaryOperation(fixture.Values)
	if !ok {
		t.Fatal("value summary judgment")
	}
	factory, ok := relation.BindValueSummary(operation, judgment, columns, place.Refusal)
	if !ok {
		t.Fatal("bind value summary")
	}
	worker := place.Worker(t, factory, operation)

	top := fixture.Values.Top()
	cells := make([]binding.Cell, 0, coordinates)
	for index, row := range place.Rows {
		if index == 1 {
			cells = append(cells, place.AbsentCell(t, cellAddress, row, valueType))
			continue
		}
		token, encodeOK := valueColumn.Encode(place.Issuer, top)
		if !encodeOK {
			t.Fatal("encode coordinate")
		}
		cells = append(cells, place.Cell(t, cellAddress, row, valueType, token))
	}
	groupToken, ok := valueColumn.Encode(place.Issuer, top)
	if !ok {
		t.Fatal("encode group address")
	}
	frame := place.Frame(t, harness.SpanSlot(t, cells), harness.ScalarSlot(t, place.Cell(t, groupAddress, place.Rows[0], valueType, groupToken)))
	buffer := place.Buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	batch, sealed := buffer.Seal(result)
	if result.Code != outcome.Produced || !sealed || batch.Len() != 1 {
		t.Fatalf("summary fold outcome=%v sealed=%t rows=%d", result.Code, sealed, batch.Len())
	}
	proposal, ok := batch.At(0)
	if !ok || proposal.Destination().Row() != place.Rows[0] {
		t.Fatal("the fold did not publish at the group row it was given")
	}
	observation, ok := summaryColumn.Decode(proposal.Value())
	if !ok || !observation.Valid || observation.Rows != 1 {
		t.Fatalf("published observation valid=%t rows=%d ok=%t", observation.Valid, observation.Rows, ok)
	}
	if !fixture.Values.OwnsSummaryObservation(observation) {
		t.Fatal("the published observation was not owned by the sealed schema")
	}
	if observation.Present[1] {
		t.Fatal("an absent coordinate was folded as a stored domain default")
	}
	if !observation.Present[0] {
		t.Fatal("a present coordinate was dropped by the fold")
	}
}

// TestValueTransferCarriesTheFactItRead drives the smallest scalar judgment in
// the axis and states where a carried fact lands: the row the frame delivered.
func TestValueTransferCarriesTheFactItRead(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/transfer")
	valueType := place.TypeID(t, "type/value")
	valueColumn := harness.NewColumn[valuedomain.Value](t, valueType, "store/value", reserve)
	columns, ok := relation.NewValueTransferColumns(valueColumn)
	if !ok {
		t.Fatal("value transfer columns")
	}
	sourceAddress := place.Column(t, "column/source")
	storedAddress := place.Column(t, "column/stored")
	operation := place.Seal(t, "operation/value-transfer",
		[]signature.Input{harness.ScalarInput(t, place.Relation, sourceAddress, valueType, place.Denominator)},
		[]signature.Output{{Relation: place.Relation, Column: storedAddress, Type: valueType, Presence: signature.ProducePresent}},
		exactlyOne(t), outcome.Produced, outcome.Refused)
	factory, ok := relation.BindValueTransfer(operation, relation.ValueTransferOperation{}, columns, place.Refusal)
	if !ok {
		t.Fatal("bind value transfer")
	}
	worker := place.Worker(t, factory, operation)

	top := fixture.Values.Top()
	token, ok := valueColumn.Encode(place.Issuer, top)
	if !ok {
		t.Fatal("encode source")
	}
	frame := place.Frame(t, harness.ScalarSlot(t, place.Cell(t, sourceAddress, place.Rows[0], valueType, token)))
	buffer := place.Buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	batch, sealed := buffer.Seal(result)
	if result.Code != outcome.Produced || !sealed || batch.Len() != 1 {
		t.Fatalf("transfer outcome=%v sealed=%t rows=%d", result.Code, sealed, batch.Len())
	}
	proposal, ok := batch.At(0)
	if !ok || proposal.Destination().Row() != place.Rows[0] || proposal.Destination().Column() != storedAddress {
		t.Fatal("the transfer did not publish at the declared destination of the row it read")
	}
	carried, ok := valueColumn.Decode(proposal.Value())
	if !ok || !fixture.Values.LessOrEq(top, carried) || !fixture.Values.LessOrEq(carried, top) {
		t.Fatal("the transfer did not carry the fact it read")
	}
}

// TestTheValueBoundaryDoesNotAllocate holds the generic boundary to zero
// allocations for the axis's own payload.
func TestTheValueBoundaryDoesNotAllocate(t *testing.T) {
	fixture := relationfixture.New(t)
	place := harness.New(t, "row/value")
	column := harness.NewColumn[valuedomain.Value](t, place.TypeID(t, "type/value"), "store/value", 1<<20)
	top := fixture.Values.Top()
	if allocations := testing.AllocsPerRun(200, func() {
		token, ok := column.Encode(place.Issuer, top)
		if !ok {
			t.Fatal("encode correlated value")
		}
		if _, ok := column.Decode(token); !ok {
			t.Fatal("decode correlated value")
		}
	}); allocations != 0 {
		t.Fatalf("the value boundary allocated %.0f times", allocations)
	}
}

// TestAValueTokenIsNotAnotherOwnersToken states the boundary is typed all the
// way down: two owners share one generic surface and neither can read the
// other's payload, even when both are behind the same token shape.
func TestAValueTokenIsNotAnotherOwnersToken(t *testing.T) {
	place := harness.New(t, "row/value")
	valueColumn := harness.NewColumn[valuedomain.Value](t, place.TypeID(t, "type/value"), "store/value", reserve)
	callColumn := harness.NewColumn[calldomain.Value](t, place.TypeID(t, "type/call"), "store/call", reserve)
	valueToken, ok := valueColumn.Encode(place.Issuer, valuedomain.Value{})
	if !ok {
		t.Fatal("encode correlated value")
	}
	callToken, ok := callColumn.Encode(place.Issuer, calldomain.Value{})
	if !ok {
		t.Fatal("encode call value")
	}
	if _, crossed := callColumn.Decode(valueToken); crossed {
		t.Fatal("a correlated value token decoded as a call value")
	}
	if _, crossed := valueColumn.Decode(callToken); crossed {
		t.Fatal("a call value token decoded as a correlated value")
	}
}
