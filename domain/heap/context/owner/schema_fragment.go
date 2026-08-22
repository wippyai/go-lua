package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	contextdomain "github.com/wippyai/go-lua/domain/heap/context"
)

// SchemaFragment is Context's callback-free cold Factor surface. It carries
// only the exact slot and forms; the sealed contextual authority and its
// allocation-key coordinate universe arrive at hot bind.
type SchemaFragment struct {
	slot       *engine.FactorSlot[contextdomain.Value]
	ref        engine.FactorRef[contextdomain.Value]
	exactRead  engine.SchemaReadForm[contextdomain.Value]
	exactWrite engine.SchemaWriteForm[contextdomain.Value]
}

func (fragment *SchemaFragment) Ref() engine.FactorRef[contextdomain.Value] {
	if fragment == nil {
		return engine.FactorRef[contextdomain.Value]{}
	}
	return fragment.ref
}

func (fragment *SchemaFragment) ExactRead() engine.SchemaReadForm[contextdomain.Value] {
	if fragment == nil {
		return engine.SchemaReadForm[contextdomain.Value]{}
	}
	return fragment.exactRead
}

func (fragment *SchemaFragment) ExactWrite() engine.SchemaWriteForm[contextdomain.Value] {
	if fragment == nil {
		return engine.SchemaWriteForm[contextdomain.Value]{}
	}
	return fragment.exactWrite
}

// DeclareSchema records Context's exact-read/exact-write Factor shape. The
// caller supplies only the already-resolved semantic identity.
func DeclareSchema(builder *engine.SchemaBuilder, semantic identity.SemanticKey) (*SchemaFragment, bool) {
	if builder == nil || !semantic.Available() {
		return nil, false
	}
	slot, ok := engine.NewFactorSlot[contextdomain.Value](builder, semantic)
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
