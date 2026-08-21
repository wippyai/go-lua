package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/placement"
)

// SchemaFragment is Placement's callback-free cold Factor surface. The
// Placement owner supplies Link-local admission and rank behavior later.
type SchemaFragment struct {
	slot       *engine.FactorSlot[placement.Placement]
	ref        engine.FactorRef[placement.Placement]
	exactRead  engine.SchemaReadForm[placement.Placement]
	foldRead   engine.SchemaReadForm[placement.Placement]
	exactWrite engine.SchemaWriteForm[placement.Placement]
}

func (fragment *SchemaFragment) Ref() engine.FactorRef[placement.Placement] {
	if fragment == nil {
		return engine.FactorRef[placement.Placement]{}
	}
	return fragment.ref
}

func (fragment *SchemaFragment) ExactRead() engine.SchemaReadForm[placement.Placement] {
	if fragment == nil {
		return engine.SchemaReadForm[placement.Placement]{}
	}
	return fragment.exactRead
}

// FoldSummaryRead is Placement's coordinate-wise summary form. Each Heap
// root remains an independent output coordinate while observations from
// alternate paths join at that coordinate.
func (fragment *SchemaFragment) FoldSummaryRead() engine.SchemaReadForm[placement.Placement] {
	if fragment == nil {
		return engine.SchemaReadForm[placement.Placement]{}
	}
	return fragment.foldRead
}

func (fragment *SchemaFragment) ExactWrite() engine.SchemaWriteForm[placement.Placement] {
	if fragment == nil {
		return engine.SchemaWriteForm[placement.Placement]{}
	}
	return fragment.exactWrite
}

// DeclareSchema records Placement's exact-read/exact-write Factor shape.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, foldSemantic identity.SemanticKey) (*SchemaFragment, bool) {
	if builder == nil || !semantic.Available() || !foldSemantic.Available() || semantic == foldSemantic {
		return nil, false
	}
	slot, ok := engine.NewFactorSlot[placement.Placement](builder, semantic)
	if !ok {
		return nil, false
	}
	read, ok := slot.ExactRead()
	if !ok {
		return nil, false
	}
	fold, ok := slot.DistributiveSummaryRead(foldSemantic)
	if !ok {
		return nil, false
	}
	write, ok := slot.ExactWrite()
	if !ok {
		return nil, false
	}
	return &SchemaFragment{slot: slot, ref: slot.Ref(), exactRead: read, foldRead: fold, exactWrite: write}, true
}
