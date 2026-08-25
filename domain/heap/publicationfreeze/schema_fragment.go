package publicationfreeze

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	effectpublication "github.com/wippyai/go-lua/domain/effect/publication"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// SchemaFragment is the Effect publication FreezeSeal Rule shape. The
// dependency DAG is Call exact read -> selected Value subject read -> selected
// Heap routed read/write. Effect remains the owner of the published call
// operand; it is intentionally not re-declared as a second Factor read.
type SchemaFragment struct {
	slot      *engine.RuleSlot[heapdomain.Value, effectpublication.CallRow]
	input     engine.SchemaInput
	callRead  engine.SchemaReadSlot[calldomain.Value]
	valueRead engine.SchemaReadSlot[valuedomain.Value]
	heapRead  engine.SchemaReadSlot[heapdomain.Value]
	carry     engine.SchemaCarrySlot[heapdomain.Value]
	write     engine.SchemaWriteSlot[heapdomain.Value]
	semantic  identity.SemanticKey
}

// RuleSlot returns the exact cold Rule declaration for composition.
func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[heapdomain.Value, effectpublication.CallRow] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

// DeclareSchema records one dynamic mounted-call input. Receipt rows are
// selected at solve time, so the cold shape remains one call operand per call.
func DeclareSchema(
	builder *engine.SchemaBuilder,
	semantic, operandFamily identity.SemanticKey,
	values *valueowner.SchemaFragment,
	calls *callowner.SchemaFragment,
	heap *heapowner.SchemaFragment,
) (*SchemaFragment, bool) {
	if builder == nil || values == nil || calls == nil || heap == nil || !identity.DistinctKeys(semantic, operandFamily) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[heapdomain.Value, effectpublication.CallRow](builder, engine.SchemaRuleSpec[heapdomain.Value]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 1,
		Output: heap.Ref(),
	})
	if !ok {
		return nil, false
	}
	input, ok := slot.Input(0)
	if !ok {
		return nil, false
	}
	callRead, ok := engine.SchemaRead[calldomain.Value](slot, calls.ExactRead(), input)
	if !ok {
		return nil, false
	}
	valueRead, ok := engine.SchemaSelectedRead[valuedomain.Value](slot, values.ExactRead(), input, callRead.Ref())
	if !ok {
		return nil, false
	}
	heapRead, ok := engine.SchemaSelectedRead[heapdomain.Value](slot, heap.ExactRead(), input, callRead.Ref(), valueRead.Ref())
	if !ok {
		return nil, false
	}
	carry, ok := engine.SchemaCarryFrom(slot, input, heap.Ref())
	if !ok {
		return nil, false
	}
	write, ok := engine.SchemaRouteWrite(slot, heap.ExactWrite(), heapRead)
	if !ok {
		return nil, false
	}
	return &SchemaFragment{
		slot: slot, input: input, callRead: callRead, valueRead: valueRead,
		heapRead: heapRead, carry: carry, write: write, semantic: semantic,
	}, true
}
