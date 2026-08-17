package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/heap"
)

// SchemaFragment is Heap's callback-free cold Factor surface. Heap roots,
// lattices, Link identity, and runtime admission remain Binding authorities.
type SchemaFragment struct {
	slot       *engine.FactorSlot[heap.Value]
	ref        engine.FactorRef[heap.Value]
	exactRead  engine.SchemaReadForm[heap.Value]
	exactWrite engine.SchemaWriteForm[heap.Value]
}

func (fragment *SchemaFragment) Ref() engine.FactorRef[heap.Value] {
	if fragment == nil {
		return engine.FactorRef[heap.Value]{}
	}
	return fragment.ref
}
func (fragment *SchemaFragment) ExactRead() engine.SchemaReadForm[heap.Value] {
	if fragment == nil {
		return engine.SchemaReadForm[heap.Value]{}
	}
	return fragment.exactRead
}
func (fragment *SchemaFragment) ExactWrite() engine.SchemaWriteForm[heap.Value] {
	if fragment == nil {
		return engine.SchemaWriteForm[heap.Value]{}
	}
	return fragment.exactWrite
}

// DeclareSchema records Heap's exact-read/exact-write Factor shape.
func DeclareSchema(builder *engine.SchemaBuilder, semantic identity.SemanticKey) (*SchemaFragment, bool) {
	if builder == nil || !semantic.Available() {
		return nil, false
	}
	slot, ok := engine.NewFactorSlot[heap.Value](builder, semantic)
	if !ok {
		return nil, false
	}
	read, ok := slot.ExactRead()
	if !ok {
		return nil, false
	}
	write, ok := slot.ExactWrite()
	if !ok {
		return nil, false
	}
	return &SchemaFragment{slot: slot, ref: slot.Ref(), exactRead: read, exactWrite: write}, true
}
