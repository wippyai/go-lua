package index

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// RawGetSchemaFragment is the callback-free RawGet Rule shape. Selector
// callbacks and payload/topology catalogs remain Binding-owned.
type RawGetSchemaFragment struct {
	slot       *engine.RuleSlot[valuedomain.Value, Index]
	receiver   engine.SchemaReadSlot[valuedomain.Value]
	key        engine.SchemaReadSlot[valuedomain.Value]
	call       engine.SchemaReadSlot[calldomain.Value]
	heapRead   engine.SchemaReadSlot[heapdomain.Value]
	packRead   engine.SchemaReadSlot[packdomain.Value]
	sourceRead engine.SchemaReadSlot[valuedomain.Value]
	carry      engine.SchemaCarrySlot[valuedomain.Value]
	write      engine.SchemaWriteSlot[valuedomain.Value]
}

// DeclareRawGetSchema records RawGet's exact single-input selector DAG.
func DeclareRawGetSchema(builder *engine.SchemaBuilder, semantic, operandFamily identity.SemanticKey, values *valueowner.SchemaFragment, calls *callowner.SchemaFragment, heap *heapowner.SchemaFragment, packs *packowner.SchemaFragment) (*RawGetSchemaFragment, bool) {
	if builder == nil || values == nil || calls == nil || heap == nil || packs == nil || !identity.DistinctKeys(semantic, operandFamily) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[valuedomain.Value, Index](builder, engine.SchemaRuleSpec[valuedomain.Value]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 1,
		Output: values.Ref(),
	})
	if !ok {
		return nil, false
	}
	in0, ok := slot.Input(0)
	if !ok {
		return nil, false
	}
	receiver, ok := engine.SchemaRead[valuedomain.Value](slot, values.ExactRead(), in0)
	if !ok {
		return nil, false
	}
	key, ok := engine.SchemaSelectedRead[valuedomain.Value](slot, values.ExactRead(), in0, receiver.Ref())
	if !ok {
		return nil, false
	}
	call, ok := engine.SchemaSelectedRead[calldomain.Value](slot, calls.ExactRead(), in0, receiver.Ref(), key.Ref())
	if !ok {
		return nil, false
	}
	heapRead, ok := engine.SchemaSelectedRead[heapdomain.Value](slot, heap.ExactRead(), in0, receiver.Ref(), key.Ref(), call.Ref())
	if !ok {
		return nil, false
	}
	packRead, ok := engine.SchemaSelectedRead[packdomain.Value](slot, packs.ExactRead(), in0, key.Ref(), heapRead.Ref())
	if !ok {
		return nil, false
	}
	sourceRead, ok := engine.SchemaSelectedRead[valuedomain.Value](slot, values.ExactRead(), in0, key.Ref(), heapRead.Ref(), packRead.Ref())
	if !ok {
		return nil, false
	}
	carry, ok := engine.SchemaCarryFrom(slot, in0, values.Ref())
	if !ok {
		return nil, false
	}
	write, ok := engine.SchemaWrite(slot, values.ExactWrite())
	if !ok {
		return nil, false
	}
	return &RawGetSchemaFragment{slot: slot, receiver: receiver, key: key, call: call, heapRead: heapRead, packRead: packRead, sourceRead: sourceRead, carry: carry, write: write}, true
}

// RawSetSchemaFragment is the callback-free RawSet Rule shape. The routed
// write is retained as an opaque child capability; its route is the Heap
// selected read, not a reconstructed topology relation.
type RawSetSchemaFragment struct {
	slot       *engine.RuleSlot[heapdomain.Value, Index]
	valueRef   engine.FactorRef[valuedomain.Value]
	heapRef    engine.FactorRef[heapdomain.Value]
	packRef    engine.FactorRef[packdomain.Value]
	receiver   engine.SchemaReadSlot[valuedomain.Value]
	key        engine.SchemaReadSlot[valuedomain.Value]
	heapRead   engine.SchemaReadSlot[heapdomain.Value]
	packRead   engine.SchemaReadSlot[packdomain.Value]
	sourceRead engine.SchemaReadSlot[valuedomain.Value]
	carry      engine.SchemaCarrySlot[heapdomain.Value]
	write      engine.SchemaWriteSlot[heapdomain.Value]
}

func (fragment *RawGetSchemaFragment) RuleSlot() *engine.RuleSlot[valuedomain.Value, Index] {
	return fragment.slot
}
func (fragment *RawSetSchemaFragment) RuleSlot() *engine.RuleSlot[heapdomain.Value, Index] {
	return fragment.slot
}

// DeclareRawSetSchema records RawSet's exact single-input selector DAG and
// routed Heap write.
func DeclareRawSetSchema(builder *engine.SchemaBuilder, semantic, operandFamily identity.SemanticKey, values *valueowner.SchemaFragment, heap *heapowner.SchemaFragment, packs *packowner.SchemaFragment) (*RawSetSchemaFragment, bool) {
	if builder == nil || values == nil || heap == nil || packs == nil || !identity.DistinctKeys(semantic, operandFamily) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[heapdomain.Value, Index](builder, engine.SchemaRuleSpec[heapdomain.Value]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 1,
		Output: heap.Ref(),
	})
	if !ok {
		return nil, false
	}
	in0, ok := slot.Input(0)
	if !ok {
		return nil, false
	}
	receiver, ok := engine.SchemaRead[valuedomain.Value](slot, values.ExactRead(), in0)
	if !ok {
		return nil, false
	}
	key, ok := engine.SchemaSelectedRead[valuedomain.Value](slot, values.ExactRead(), in0, receiver.Ref())
	if !ok {
		return nil, false
	}
	heapRead, ok := engine.SchemaSelectedRead[heapdomain.Value](slot, heap.ExactRead(), in0, receiver.Ref(), key.Ref())
	if !ok {
		return nil, false
	}
	packRead, ok := engine.SchemaSelectedRead[packdomain.Value](slot, packs.ExactRead(), in0, receiver.Ref(), key.Ref(), heapRead.Ref())
	if !ok {
		return nil, false
	}
	sourceRead, ok := engine.SchemaSelectedRead[valuedomain.Value](slot, values.ExactRead(), in0, receiver.Ref(), key.Ref(), heapRead.Ref(), packRead.Ref())
	if !ok {
		return nil, false
	}
	carry, ok := engine.SchemaCarryFrom(slot, in0, heap.Ref())
	if !ok {
		return nil, false
	}
	write, ok := engine.SchemaRouteWrite(slot, heap.ExactWrite(), heapRead)
	if !ok {
		return nil, false
	}
	return &RawSetSchemaFragment{slot: slot, valueRef: values.Ref(), heapRef: heap.Ref(), packRef: packs.Ref(), receiver: receiver, key: key, heapRead: heapRead, packRead: packRead, sourceRead: sourceRead, carry: carry, write: write}, true
}
