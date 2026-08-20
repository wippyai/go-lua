package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/heap"
)

// SchemaFragment is Heap's callback-free cold Factor surface. Heap roots,
// lattices, Link identity, and runtime admission remain Binding authorities.
type SchemaFragment struct {
	slot        *engine.FactorSlot[heap.Value]
	ref         engine.FactorRef[heap.Value]
	exactRead   engine.SchemaReadForm[heap.Value]
	summaryRead engine.SchemaReadForm[heap.Value]
	exactWrite  engine.SchemaWriteForm[heap.Value]
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

// SummaryRead is Heap's complete-vector summary form. Unlike a distributive
// summary, a reader of this form observes the declared Heap key vector as one
// joint summary, which preserves the identity needed by authenticated
// cross-factor query projections.
func (fragment *SchemaFragment) SummaryRead() engine.SchemaReadForm[heap.Value] {
	if fragment == nil {
		return engine.SchemaReadForm[heap.Value]{}
	}
	return fragment.summaryRead
}
func (fragment *SchemaFragment) ExactWrite() engine.SchemaWriteForm[heap.Value] {
	if fragment == nil {
		return engine.SchemaWriteForm[heap.Value]{}
	}
	return fragment.exactWrite
}

// DeclareSchema records Heap's exact-read/complete-summary/exact-write Factor
// shape. The summary semantic is a separate role because the complete-vector
// read is an independently addressed schema surface.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, summarySemantic identity.SemanticKey) (*SchemaFragment, bool) {
	if builder == nil || !semantic.Available() || !summarySemantic.Available() || semantic == summarySemantic {
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
	summary, ok := slot.SummaryRead(summarySemantic)
	if !ok {
		return nil, false
	}
	write, ok := slot.ExactWrite()
	if !ok {
		return nil, false
	}
	return &SchemaFragment{slot: slot, ref: slot.Ref(), exactRead: read, summaryRead: summary, exactWrite: write}, true
}
