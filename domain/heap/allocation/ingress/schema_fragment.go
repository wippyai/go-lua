package ingress

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
)

// SchemaFragment is Heap-ingress's callback-free zero-input Rule surface.
type SchemaFragment struct {
	slot     *engine.RuleSlot[heapdomain.Value, source.Root]
	write    engine.SchemaWriteSlot[heapdomain.Value]
	semantic identity.SemanticKey
}

// DeclareSchema records the WorldZero ingress Rule shape.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily identity.SemanticKey, owner *heapowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || owner == nil || !identity.DistinctKeys(semantic, operandFamily) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[heapdomain.Value, source.Root](builder, engine.SchemaRuleSpec[heapdomain.Value]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 0,
		Output: owner.Ref(),
	})
	if !ok {
		return nil, false
	}
	write, ok := engine.SchemaWrite(slot, owner.ExactWrite())
	if !ok {
		return nil, false
	}
	return &SchemaFragment{slot: slot, write: write, semantic: semantic}, true
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[heapdomain.Value, source.Root] {
	return fragment.slot
}
