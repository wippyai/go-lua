package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/effect/factor"
)

// SchemaFragment is Effect's callback-free cold Factor surface. Effect roots,
// algebra, Link identity, and executable admission remain Binding authorities.
type SchemaFragment struct {
	slot       *engine.FactorSlot[factor.Value]
	ref        engine.FactorRef[factor.Value]
	exactRead  engine.SchemaReadForm[factor.Value]
	exactWrite engine.SchemaWriteForm[factor.Value]
}

func (fragment *SchemaFragment) Ref() engine.FactorRef[factor.Value] {
	if fragment == nil {
		return engine.FactorRef[factor.Value]{}
	}
	return fragment.ref
}
func (fragment *SchemaFragment) ExactRead() engine.SchemaReadForm[factor.Value] {
	if fragment == nil {
		return engine.SchemaReadForm[factor.Value]{}
	}
	return fragment.exactRead
}
func (fragment *SchemaFragment) ExactWrite() engine.SchemaWriteForm[factor.Value] {
	if fragment == nil {
		return engine.SchemaWriteForm[factor.Value]{}
	}
	return fragment.exactWrite
}

// DeclareSchema records Effect's exact-read/exact-write Factor shape.
func DeclareSchema(builder *engine.SchemaBuilder, semantic identity.SemanticKey) (*SchemaFragment, bool) {
	if builder == nil || !semantic.Available() {
		return nil, false
	}
	slot, ok := engine.NewFactorSlot[factor.Value](builder, semantic)
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
