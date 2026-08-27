package relbindgen_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
)

type opaqueArgument struct{ key identity.ContentID }

type opaqueDecoder struct{}

func (opaqueDecoder) Decode(inputs relbindgen.Inputs) (opaqueArgument, bool) {
	key, ok := inputs.RowKeyAt(0)
	return opaqueArgument{key: key}, ok
}

type opaqueEncoder struct{ column *relbindgen.Column[int] }

func (encoder opaqueEncoder) Encode(outputs relbindgen.Outputs, value int) bool {
	return relbindgen.PutColumn(outputs, 0, encoder.column, value)
}

type opaqueOperation struct{}

func (opaqueOperation) Evaluate(argument opaqueArgument, emitter *relbindgen.Emitter[int]) outcome.Code {
	if !emitter.PutOpaqueAt(argument.key, 7) {
		return outcome.Refused
	}
	return outcome.Opaque
}

// TestBindingCarriesOpaqueRowsThroughTheGeneratedAdapter proves that the
// generated ABI retains an authenticated opaque row and its Opaque outcome as
// one sealed proposal batch. A Produced-only gate would discard the row.
func TestBindingCarriesOpaqueRowsThroughTheGeneratedAdapter(t *testing.T) {
	place := harness.New(t, "row/opaque")
	columnID := place.Column(t, "column/output")
	typeID := place.TypeID(t, "type/output")
	column := harness.NewColumn[int](t, typeID, "store/output", 1)
	inputValue, ok := column.Encode(place.Issuer, 3)
	if !ok {
		t.Fatal("input value")
	}
	input := harness.ScalarInput(t, place.Relation, columnID, typeID, place.Denominator)
	exact, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	operation := place.Seal(t, "operation/opaque", []signature.Input{input}, []signature.Output{{
		Relation: place.Relation, Column: columnID, Type: typeID,
		Presence: signature.ProduceOpaque, Denominator: place.Denominator,
	}}, exact, outcome.Opaque, outcome.Refused)
	factory, ok := relbindgen.Bind(relbindgen.Spec[opaqueArgument, int]{
		Signature: operation,
		Decoder:   opaqueDecoder{},
		Encoder:   opaqueEncoder{column: column},
		Operation: opaqueOperation{},
		Refusal:   place.Refusal,
	})
	if !ok {
		t.Fatal("bind opaque operation")
	}
	worker := place.Worker(t, factory, operation)
	cell := place.Cell(t, columnID, place.Rows[0], typeID, inputValue)
	frame := place.Frame(t, harness.ScalarSlot(t, cell))
	buffer := place.Buffer(t, operation)
	result := worker.Evaluate(frame, buffer)
	batch, sealed := buffer.Seal(result)
	if !sealed || !batch.Available() || result.Code != outcome.Opaque || batch.Outcome().Code != outcome.Opaque || batch.Len() != 1 {
		t.Fatalf("opaque adapter result=%v sealed=%v available=%v rows=%d", result.Code, sealed, batch.Available(), batch.Len())
	}
	proposal, ok := batch.At(0)
	if !ok || !proposal.Available() || !proposal.Presence().Is(model.AuthenticatedOpaque) || !proposal.Value().Available() {
		t.Fatalf("opaque proposal available=%v presence=%v value=%v", proposal.Available(), proposal.Presence().Kind(), proposal.Value().Available())
	}
	if proposal.Destination().Row() != place.Rows[0] {
		t.Fatal("opaque adapter changed the owner-issued row")
	}
}
