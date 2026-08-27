package relation_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
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
		[]signature.Output{{Relation: place.Relation, Column: resultAddress, Type: valueType, Presence: signature.ProducePresent, Denominator: place.Denominator}},
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
		[]signature.Output{{Relation: place.Relation, Column: observationAddress, Type: summaryType, Presence: signature.ProducePresent, Denominator: place.Denominator}},
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
		[]signature.Output{{Relation: place.Relation, Column: storedAddress, Type: valueType, Presence: signature.ProducePresent, Denominator: place.Denominator}},
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

// TestADeliveredSpanSaysWhichRowEachPositionCarries is the span identity law.
// A span arrives in the mounted order of its declared key, and an owner fold
// that looks its rows up by the owner's own identity has to be able to say
// which row each delivered position is. Without that a binding can read the
// values of a group and still not answer a lookup its owner keys by identity,
// which is the whole of what a grouped owner judgment asks for.
func TestADeliveredSpanSaysWhichRowEachPositionCarries(t *testing.T) {
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
		[]signature.Output{{Relation: place.Relation, Column: observationAddress, Type: summaryType, Presence: signature.ProducePresent, Denominator: place.Denominator}},
		exactlyOne(t), outcome.Produced, outcome.NoSelection, outcome.Refused)

	top := fixture.Values.Top()
	cells := make([]binding.Cell, 0, coordinates)
	for _, row := range place.Rows {
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

	// The decoder is the only thing that can borrow the span, so the law asks
	// the question through one: a judgment that reads every delivered row's
	// identity and answers with what it read.
	observed := make([]identity.ContentID, 0, coordinates)
	judgment := spanIdentityJudgment{observed: &observed}
	factory, ok := relation.BindValueSummary(operation, judgment, columns, place.Refusal)
	if !ok {
		t.Fatal("bind value summary")
	}
	worker := place.Worker(t, factory, operation)
	buffer := place.Buffer(t, operation)
	if result := worker.Evaluate(frame, buffer); result.Code != outcome.Produced {
		t.Fatalf("the span identity judgment settled as %v", result.Code)
	}
	if len(observed) != coordinates {
		t.Fatalf("the judgment read %d row identities from a span of %d rows", len(observed), coordinates)
	}
	for index, row := range place.Rows {
		if observed[index] != row.Content() {
			t.Fatalf("delivered row %d reports the identity of another row", index)
		}
	}
}

// spanIdentityJudgment reads every delivered row's owner identity and records
// it. It is the smallest owner judgment that needs the span to say which row
// each position carries.
type spanIdentityJudgment struct {
	observed *[]identity.ContentID
}

func (spanIdentityJudgment) Available() bool { return true }

func (judgment spanIdentityJudgment) Evaluate(argument relation.ValueSummaryArgument, emitter *relbindgen.Emitter[valuedomain.ValueSummaryObservation]) outcome.Code {
	for index := 0; index < argument.Cells.Len(); index++ {
		key, ok := argument.Cells.RowKeyAt(index)
		if !ok {
			return outcome.Refused
		}
		*judgment.observed = append(*judgment.observed, key)
	}
	if !emitter.Put(valuedomain.ValueSummaryObservation{}) {
		return outcome.Refused
	}
	return outcome.Produced
}

// TestMaterializingADeliveredSpanIsAllocationFreeWhenWarm is the measurement
// the seam decision rests on.
//
// A span holds value tokens, so handing an owner fold the operand vocabulary
// it reads decodes rather than views, and the question is not whether that
// costs a copy - it does - but whether it costs an allocation per invocation.
// Sized once at its width and refilled, it costs none.
func TestMaterializingADeliveredSpanIsAllocationFreeWhenWarm(t *testing.T) {
	fixture := relationfixture.New(t)
	coordinates := fixture.Values.CoordinateCount()
	keys := make([]identity.ContentID, 0, coordinates)
	for index := 0; index < coordinates; index++ {
		keys = append(keys, harness.Content(t, fmt.Sprintf("row/coordinate-%d", index)))
	}
	place := harness.NewKeyed(t, keys)
	valueType := place.TypeID(t, "type/value")
	valueColumn := harness.NewColumn[valuedomain.Value](t, valueType, "store/value", 1<<20)
	cellAddress := place.Column(t, "column/cell")

	top := fixture.Values.Top()
	cells := make([]binding.Cell, 0, coordinates)
	for _, row := range place.Rows {
		token, encodeOK := valueColumn.Encode(place.Issuer, top)
		if !encodeOK {
			t.Fatal("encode coordinate")
		}
		cells = append(cells, place.Cell(t, cellAddress, row, valueType, token))
	}
	frame := place.Frame(t, harness.SpanSlot(t, cells))
	span, ok := relbindgen.SpanAtFrame(frame, 0, valueColumn)
	if !ok {
		t.Fatal("borrow span")
	}

	members, ok := relbindgen.NewMembers[valuedomain.Value](coordinates)
	if !ok {
		t.Fatal("reserve member storage")
	}
	if vector, filled := members.Fill(span); !filled || vector.Count() != coordinates {
		t.Fatalf("the vector materialized %t at width %d", filled, vector.Count())
	}
	if allocations := testing.AllocsPerRun(200, func() {
		if _, filled := members.Fill(span); !filled {
			t.Fatal("materialize the vector")
		}
	}); allocations != 0 {
		t.Fatalf("materializing a summary vector allocated %.0f times per invocation", allocations)
	}

	rows, ok := relbindgen.NewCells[valuedomain.Value](coordinates)
	if !ok {
		t.Fatal("reserve cell storage")
	}
	ordinals := map[identity.ContentID]uint64{}
	for index, row := range place.Rows {
		ordinals[row.Content()] = uint64(index + 1)
	}
	tag := func(row identity.ContentID) (uint64, bool) {
		owned, ok := ordinals[row]
		return owned, ok
	}
	selected, filled := rows.Fill(span, tag)
	if !filled || len(selected) != coordinates {
		t.Fatalf("the selection materialized %t at width %d", filled, len(selected))
	}
	for index, cell := range selected {
		if cell.Tag != uint64(index+1) {
			t.Fatalf("cell %d carries tag %d, and the row it was delivered at names %d", index, cell.Tag, index+1)
		}
	}
	if allocations := testing.AllocsPerRun(200, func() {
		if _, filled := rows.Fill(span, tag); !filled {
			t.Fatal("materialize the selection")
		}
	}); allocations != 0 {
		t.Fatalf("materializing selected cells allocated %.0f times per invocation", allocations)
	}
}
