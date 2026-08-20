package activation

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
)

// SchemaFragment is Call activation's callback-free structural Rule surface.
// It retains only the activation family, structural Rule, and exact Call read.
type SchemaFragment struct {
	semantic identity.SemanticKey
	family   engine.SchemaActivationFamily
	slot     *engine.SchemaActivationRuleSlot
	input    engine.SchemaInput
	read     engine.SchemaReadSlot[calldomain.Value]
}

// DeclareSchema records the one-input trusted structural activation Rule.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, familySemantic identity.SemanticKey, owner *callowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || owner == nil || !identity.DistinctKeys(semantic, familySemantic) {
		return nil, false
	}
	family, ok := engine.DeclareSchemaActivationFamily(builder, familySemantic)
	if !ok {
		return nil, false
	}
	slot, ok := engine.DeclareSchemaActivationRule(builder, engine.SchemaStructuralRuleSpec{
		Semantic: semantic, Inputs: 1,
		Activation: family,
	})
	if !ok {
		return nil, false
	}
	input, ok := slot.Input(0)
	if !ok {
		return nil, false
	}
	read, ok := engine.SchemaActivationRead(slot, owner.ExactRead(), input)
	if !ok {
		return nil, false
	}
	return &SchemaFragment{semantic: semantic, family: family, slot: slot, input: input, read: read}, true
}

func (fragment *SchemaFragment) ActivationSlot() *engine.SchemaActivationRuleSlot {
	return fragment.slot
}
