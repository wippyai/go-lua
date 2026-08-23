package owner

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	staticdomain "github.com/wippyai/go-lua/domain/static"
)

// SchemaFragment is Static type flow's callback-free cold factor surface.
// The Link-local Runtime relation and Value coordinate denominator arrive only
// when the composition binds the sealed Link inputs.
type SchemaFragment struct {
	slot       *engine.FactorSlot[staticdomain.TypeFact]
	ref        engine.FactorRef[staticdomain.TypeFact]
	exactRead  engine.SchemaReadForm[staticdomain.TypeFact]
	exactWrite engine.SchemaWriteForm[staticdomain.TypeFact]
}

func (fragment *SchemaFragment) Ref() engine.FactorRef[staticdomain.TypeFact] {
	if fragment == nil {
		return engine.FactorRef[staticdomain.TypeFact]{}
	}
	return fragment.ref
}

func (fragment *SchemaFragment) ExactRead() engine.SchemaReadForm[staticdomain.TypeFact] {
	if fragment == nil {
		return engine.SchemaReadForm[staticdomain.TypeFact]{}
	}
	return fragment.exactRead
}

func (fragment *SchemaFragment) ExactWrite() engine.SchemaWriteForm[staticdomain.TypeFact] {
	if fragment == nil {
		return engine.SchemaWriteForm[staticdomain.TypeFact]{}
	}
	return fragment.exactWrite
}

// DeclareSchema records the one exact-read/exact-write static-type factor.
func DeclareSchema(builder *engine.SchemaBuilder, semantic identity.SemanticKey) (*SchemaFragment, bool) {
	if builder == nil || !semantic.Available() {
		return nil, false
	}
	slot, ok := engine.NewFactorSlot[staticdomain.TypeFact](builder, semantic)
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
