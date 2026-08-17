package equality

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
)

// SchemaFragment is the callback-free reusable equality Rule shape. Both
// ordered operands are exact reads from the same pre-expression environment;
// ordinary Value state carries across the local expression cut and the
// Boolean result is an exact write.
type SchemaFragment struct {
	slot        *engine.RuleSlot[value.Value, value.BinaryEquality]
	input       engine.SchemaInput
	left, right engine.SchemaReadSlot[value.Value]
	carry       engine.SchemaCarrySlot[value.Value]
	write       engine.SchemaWriteSlot[value.Value]
	semantic    identity.SemanticKey
	evidence    identity.SemanticKey
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[value.Value, value.BinaryEquality] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily, evidence identity.SemanticKey, owner *valueowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || owner == nil || !identity.DistinctKeys(semantic, operandFamily, evidence) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[value.Value, value.BinaryEquality](builder, engine.SchemaRuleSpec[value.Value]{
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
	left, leftOK := engine.SchemaRead[value.Value](slot, owner.ExactRead(), input)
	right, rightOK := engine.SchemaRead[value.Value](slot, owner.ExactRead(), input)
	carry, carryOK := engine.SchemaCarryFrom(slot, input, owner.Ref())
	write, writeOK := engine.SchemaWrite(slot, owner.ExactWrite())
	if !leftOK || !rightOK || !carryOK || !writeOK {
		return nil, false
	}
	return &SchemaFragment{slot: slot, input: input, left: left, right: right, carry: carry, write: write, semantic: semantic, evidence: evidence}, true
}
