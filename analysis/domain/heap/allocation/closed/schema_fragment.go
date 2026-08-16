package closed

import (
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/heap/allocation/internal/source"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
)

// SchemaFragment is Heap-closed's callback-free transformed Rule surface.
type SchemaFragment struct {
	slot         *engine.RuleSlot[heapdomain.Value, source.Closed]
	input        engine.SchemaInput
	heapRead     engine.SchemaReadSlot[heapdomain.Value]
	valueRead    engine.SchemaReadSlot[valuedomain.Value]
	valueSummary engine.SchemaReadForm[valuedomain.Value]
	carry        engine.SchemaCarrySlot[heapdomain.Value]
	write        engine.SchemaWriteSlot[heapdomain.Value]
	semantic     engine.SemanticKey
	transform    engine.SemanticKey
	evidence     engine.SemanticKey
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[heapdomain.Value, source.Closed] {
	return fragment.slot
}

// DeclareSchema records Heap closed allocation's exact incidence: one input,
// exact Heap read, Value summary read, transformed Heap carry, and write.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily, transform, evidence engine.SemanticKey, heap *heapowner.SchemaFragment, values *valueowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || heap == nil || values == nil || !distinct(semantic, operandFamily, transform, evidence) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[heapdomain.Value, source.Closed](builder, engine.SchemaRuleSpec[heapdomain.Value]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 1,
		Admission: engine.SchemaAdmission{Basis: engine.RuleAdmissionBasisDerivation, Identity: evidence},
		Output:    heap.Ref(),
	})
	if !ok {
		return nil, false
	}
	input, ok := slot.Input(0)
	if !ok {
		return nil, false
	}
	heapRead, ok := engine.SchemaRead[heapdomain.Value](slot, heap.ExactRead(), input)
	if !ok {
		return nil, false
	}
	valueRead, ok := engine.SchemaRead[valuedomain.Value](slot, values.SummaryRead(), input)
	if !ok {
		return nil, false
	}
	carry, ok := engine.SchemaCarry(slot, input, heap.Ref(), transform)
	if !ok {
		return nil, false
	}
	write, ok := engine.SchemaWrite(slot, heap.ExactWrite())
	if !ok {
		return nil, false
	}
	return &SchemaFragment{slot: slot, input: input, heapRead: heapRead, valueRead: valueRead, valueSummary: values.SummaryRead(), carry: carry, write: write, semantic: semantic, transform: transform, evidence: evidence}, true
}
