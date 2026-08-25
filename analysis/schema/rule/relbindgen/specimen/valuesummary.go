package specimen

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// ValueSummaryColumns are the owner column codecs the summary fold reads and
// publishes through.
type ValueSummaryColumns struct {
	Cell        *relbindgen.Column[valuedomain.Value]
	Observation *relbindgen.Column[valuedomain.ValueSummaryObservation]
}

// ValueSummaryArgument is the decoded frame of one grouped reduction: the
// complete delivered span of the group's coordinate cells.
type ValueSummaryArgument struct {
	Cells relbindgen.Span[valuedomain.Value]
}

// ValueSummaryOperation is the owner's coordinatewise fold. The engine chose
// the group and delivered its complete span; the operation only folds.
type ValueSummaryOperation struct {
	schema *valuedomain.Schema
}

// Evaluate folds the delivered group into one summary observation.
func (operation ValueSummaryOperation) Evaluate(argument ValueSummaryArgument, emitter *relbindgen.Emitter[valuedomain.ValueSummaryObservation]) outcome.Code {
	seed := valuedomain.BeginValueSummary(operation.schema)
	folded, ok := valuedomain.AccumulateValueSummaryRows(operation.schema, seed, argument.Cells.Len(), argument.Cells.At)
	if !ok {
		return outcome.Refused
	}
	if folded.Rows == 0 {
		return outcome.NoSelection
	}
	if !emitter.Put(folded) {
		return outcome.Refused
	}
	return outcome.Produced
}

// BindValueSummary admits the grouped-reduction specimen: one complete span of
// coordinate cells plus the scalar row that addresses the group, one folded
// observation published at that row.
func BindValueSummary(operation signature.Signature, schema *valuedomain.Schema, columns ValueSummaryColumns, refusal model.RefusalID) (binding.Factory, bool) {
	if schema == nil || !schema.Valid() || !columns.Cell.Available() || !columns.Observation.Available() {
		return nil, false
	}
	return relbindgen.Bind(relbindgen.Spec[ValueSummaryArgument, valuedomain.ValueSummaryObservation]{
		Signature: operation,
		Decoder:   valueSummaryDecoder{cell: columns.Cell},
		Encoder:   valueSummaryEncoder{observation: columns.Observation},
		Operation: ValueSummaryOperation{schema: schema},
		Address:   1,
		Refusal:   refusal,
	})
}

type valueSummaryDecoder struct {
	cell *relbindgen.Column[valuedomain.Value]
}

func (decoder valueSummaryDecoder) Decode(inputs relbindgen.Inputs) (ValueSummaryArgument, bool) {
	cells, ok := relbindgen.SpanAt(inputs, 0, decoder.cell)
	if !ok {
		return ValueSummaryArgument{}, false
	}
	return ValueSummaryArgument{Cells: cells}, true
}

type valueSummaryEncoder struct {
	observation *relbindgen.Column[valuedomain.ValueSummaryObservation]
}

func (encoder valueSummaryEncoder) Encode(outputs relbindgen.Outputs, value valuedomain.ValueSummaryObservation) bool {
	return relbindgen.PutColumn(outputs, 0, encoder.observation, value)
}
