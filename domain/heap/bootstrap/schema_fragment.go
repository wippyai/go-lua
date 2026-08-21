package bootstrap

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
)

// SchemaFragment is Heap/bootstrap's callback-free Rule surface.
type SchemaFragment struct {
	slot     *engine.RuleSlot[heapdomain.Value, heapdomain.Key]
	write    engine.SchemaWriteSlot[heapdomain.Value]
	semantic identity.SemanticKey
}

// DeclareSchema records the zero-input Heap bootstrap Rule shape.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily identity.SemanticKey, owner *heapowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || owner == nil || !identity.DistinctKeys(semantic, operandFamily) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[heapdomain.Value, heapdomain.Key](builder, engine.SchemaRuleSpec[heapdomain.Value]{
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

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[heapdomain.Value, heapdomain.Key] {
	return fragment.slot
}
