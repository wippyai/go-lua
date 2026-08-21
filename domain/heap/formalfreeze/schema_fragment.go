package formalfreeze

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// SchemaFragment is the callback-free formal-freeze Rule shape. The
// dependency DAG is Call exact read -> mounted actual Value selection -> Heap
// selected routed read/write. The engine owns all read ordinals and route
// target correspondence.
type SchemaFragment struct {
	slot       *engine.RuleSlot[heapdomain.Value, operand]
	callRead   engine.SchemaReadSlot[calldomain.Value]
	actualRead engine.SchemaReadSlot[valuedomain.Value]
	heapRead   engine.SchemaReadSlot[heapdomain.Value]
	carry      engine.SchemaCarrySlot[heapdomain.Value]
	write      engine.SchemaWriteSlot[heapdomain.Value]
	semantic   identity.SemanticKey
}

// RuleSlot returns the exact cold Rule declaration for composition.
func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[heapdomain.Value, operand] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

// DeclareSchema records the exact cross-axis selector DAG. It accepts only
// neutral Factor forms; Target/Call/Pack/Value/Heap meaning remains in the
// owner-fenced hot authorities.
func DeclareSchema(
	builder *engine.SchemaBuilder,
	semantic, operandFamily identity.SemanticKey,
	values *valueowner.SchemaFragment,
	calls *callowner.SchemaFragment,
	heap *heapowner.SchemaFragment,
) (*SchemaFragment, bool) {
	if builder == nil || values == nil || calls == nil || heap == nil ||
		!identity.DistinctKeys(semantic, operandFamily) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[heapdomain.Value, operand](builder, engine.SchemaRuleSpec[heapdomain.Value]{
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
	actualRead, ok := engine.SchemaSelectedRead[valuedomain.Value](slot, values.ExactRead(), input, callRead.Ref())
	if !ok {
		return nil, false
	}
	heapRead, ok := engine.SchemaSelectedRead[heapdomain.Value](slot, heap.ExactRead(), input, callRead.Ref(), actualRead.Ref())
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
		slot: slot, callRead: callRead, actualRead: actualRead, heapRead: heapRead,
		carry: carry, write: write, semantic: semantic,
	}, true
}
