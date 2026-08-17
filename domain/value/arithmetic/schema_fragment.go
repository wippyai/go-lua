package arithmetic

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// SchemaFragment is the callback-free reusable arithmetic Rule shape. Both
// ordered operands are exact reads from the pre-expression environment;
// ordinary Value state carries across the Program-owned local computation
// cut and the result is one exact write.
type SchemaFragment struct {
	slot        *engine.RuleSlot[value.Value, value.BinaryArithmetic]
	input       engine.SchemaInput
	left, right engine.SchemaReadSlot[value.Value]
	carry       engine.SchemaCarrySlot[value.Value]
	write       engine.SchemaWriteSlot[value.Value]
	semantic    identity.SemanticKey
	evidence    identity.SemanticKey
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[value.Value, value.BinaryArithmetic] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily, evidence identity.SemanticKey, owner *valueowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || owner == nil || !identity.DistinctKeys(semantic, operandFamily, evidence) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[value.Value, value.BinaryArithmetic](builder, engine.SchemaRuleSpec[value.Value]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 1,
		Admission: engine.SchemaAdmission{Basis: engine.RuleAdmissionBasisDerivation, Identity: evidence},
		Output:    owner.Ref(),
	})
	if !ok {
		return nil, false
	}
	input, inputOK := slot.Input(0)
	left, leftOK := engine.SchemaRead[value.Value](slot, owner.ExactRead(), input)
	right, rightOK := engine.SchemaRead[value.Value](slot, owner.ExactRead(), input)
	carry, carryOK := engine.SchemaCarryFrom(slot, input, owner.Ref())
	write, writeOK := engine.SchemaWrite(slot, owner.ExactWrite())
	if !inputOK || !leftOK || !rightOK || !carryOK || !writeOK {
		return nil, false
	}
	return &SchemaFragment{slot: slot, input: input, left: left, right: right, carry: carry, write: write, semantic: semantic, evidence: evidence}, true
}
