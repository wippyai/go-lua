package allocation

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
)

// SchemaFragment is Value-allocation's callback-free transformed Rule
// surface. The operand and transform are retained only as typed cold slots.
type SchemaFragment struct {
	slot      *engine.RuleSlot[value.Value, operand]
	input     engine.SchemaInput
	carry     engine.SchemaCarrySlot[value.Value]
	write     engine.SchemaWriteSlot[value.Value]
	semantic  engine.SemanticKey
	transform engine.SemanticKey
	evidence  engine.SemanticKey
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[value.Value, operand] {
	return fragment.slot
}

// DeclareSchema records Value allocation's one-input transformed-carry Rule.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily, transform, evidence engine.SemanticKey, owner *valueowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || owner == nil || !engine.DistinctKeys(semantic, operandFamily, transform, evidence) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[value.Value, operand](builder, engine.SchemaRuleSpec[value.Value]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 1,
		Admission: engine.SchemaAdmission{Basis: engine.RuleAdmissionBasisDerivation, Identity: evidence},
		Output:    owner.Ref(),
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
	return &SchemaFragment{slot: slot, input: input, carry: carry, write: write, semantic: semantic, transform: transform, evidence: evidence}, true
}
