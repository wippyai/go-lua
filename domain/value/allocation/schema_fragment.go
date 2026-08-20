package allocation

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// SchemaFragment is Value-allocation's callback-free transformed Rule
// surface. The operand and transform are retained only as typed cold slots.
type SchemaFragment struct {
	slot      *engine.RuleSlot[value.Value, operand]
	input     engine.SchemaInput
	carry     engine.SchemaCarrySlot[value.Value]
	write     engine.SchemaWriteSlot[value.Value]
	semantic  identity.SemanticKey
	transform identity.SemanticKey
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[value.Value, operand] {
	return fragment.slot
}

// DeclareSchema records Value allocation's one-input transformed-carry Rule.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily, transform identity.SemanticKey, owner *valueowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || owner == nil || !identity.DistinctKeys(semantic, operandFamily, transform) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[value.Value, operand](builder, engine.SchemaRuleSpec[value.Value]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 1,
		Output: owner.Ref(),
	})
	if !ok {
		return nil, false
	}
	input, ok := slot.Input(0)
	if !ok {
		return nil, false
	}
	carry, ok := engine.SchemaCarry(slot, input, owner.Ref(), transform)
	if !ok {
		return nil, false
	}
	write, ok := engine.SchemaWrite(slot, owner.ExactWrite())
	if !ok {
		return nil, false
	}
	return &SchemaFragment{slot: slot, input: input, carry: carry, write: write, semantic: semantic, transform: transform}, true
}
