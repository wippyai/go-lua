package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/pack"
	"github.com/wippyai/go-lua/analysis/engine"
)

// SchemaFragment is Pack's callback-free cold Factor surface. It has no Pack
// schema, Link, roots, or runtime behavior.
type SchemaFragment struct {
	slot       *engine.FactorSlot[pack.Value]
	ref        engine.FactorRef[pack.Value]
	exactRead  engine.SchemaReadForm[pack.Value]
	exactWrite engine.SchemaWriteForm[pack.Value]
}

func (fragment *SchemaFragment) Ref() engine.FactorRef[pack.Value] {
	if fragment == nil {
		return engine.FactorRef[pack.Value]{}
	}
	return fragment.ref
}
func (fragment *SchemaFragment) ExactRead() engine.SchemaReadForm[pack.Value] {
	if fragment == nil {
		return engine.SchemaReadForm[pack.Value]{}
	}
	return fragment.exactRead
}
func (fragment *SchemaFragment) ExactWrite() engine.SchemaWriteForm[pack.Value] {
	if fragment == nil {
		return engine.SchemaWriteForm[pack.Value]{}
	}
	return fragment.exactWrite
}

// DeclareSchema records Pack's exact-read/exact-write Factor shape.
func DeclareSchema(builder *engine.SchemaBuilder, semantic engine.SemanticKey) (*SchemaFragment, bool) {
	if builder == nil || !semantic.Available() {
		return nil, false
	}
	slot, ok := engine.NewFactorSlot[pack.Value](builder, semantic)
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
