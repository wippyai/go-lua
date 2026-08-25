package specimen

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// HeapAscentColumns are the owner column codecs the heap update reads and
// publishes through. Both name the same heap value TypeID: a cell update
// ascends the cell it read.
type HeapAscentColumns struct {
	Cell     *relbindgen.Column[heapdomain.Value]
	Proposed *relbindgen.Column[heapdomain.Value]
}

// HeapAscentArgument is the decoded frame of one raw-set heap update: the
// currently committed cell and the fact the write proposes for it.
type HeapAscentArgument struct {
	Current  heapdomain.Value
	Proposed heapdomain.Value
}

// HeapAscentOperation is the owner's heap ascent. It is monotone because it is
// the owner lattice's own join, so every proposal it returns is an ascent of
// the cell it read; nothing here validates that fact a second time.
type HeapAscentOperation struct{}

// Evaluate ascends the read cell by the proposed heap fact.
func (HeapAscentOperation) Evaluate(argument HeapAscentArgument, emitter *relbindgen.Emitter[heapdomain.Value]) outcome.Code {
	ascended, ok := heapdomain.Join(argument.Current, argument.Proposed)
	if !ok {
		return outcome.Refused
	}
	if !emitter.Put(ascended) {
		return outcome.Refused
	}
	return outcome.Produced
}

// BindHeapAscent admits the cell-update specimen: the read cell and the
// proposed fact, one ascended proposal at the read cell's own row.
func BindHeapAscent(operation signature.Signature, columns HeapAscentColumns, refusal model.RefusalID) (binding.Factory, bool) {
	if !columns.Cell.Available() || !columns.Proposed.Available() {
		return nil, false
	}
	return relbindgen.Bind(relbindgen.Spec[HeapAscentArgument, heapdomain.Value]{
		Signature: operation,
		Decoder:   heapAscentDecoder{columns: columns},
		Encoder:   heapAscentEncoder{cell: columns.Cell},
		Operation: HeapAscentOperation{},
		Address:   0,
		Refusal:   refusal,
	})
}

type heapAscentDecoder struct {
	columns HeapAscentColumns
}

func (decoder heapAscentDecoder) Decode(inputs relbindgen.Inputs) (HeapAscentArgument, bool) {
	current, ok := relbindgen.ScalarAt(inputs, 0, decoder.columns.Cell)
	if !ok {
		return HeapAscentArgument{}, false
	}
	proposed, ok := relbindgen.ScalarAt(inputs, 1, decoder.columns.Proposed)
	if !ok {
		return HeapAscentArgument{}, false
	}
	return HeapAscentArgument{Current: current, Proposed: proposed}, true
}

type heapAscentEncoder struct {
	cell *relbindgen.Column[heapdomain.Value]
}

func (encoder heapAscentEncoder) Encode(outputs relbindgen.Outputs, value heapdomain.Value) bool {
	return relbindgen.PutColumn(outputs, 0, encoder.cell, value)
}
