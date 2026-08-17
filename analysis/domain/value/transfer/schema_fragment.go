package transfer

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
)

// SchemaFragment is Value/transfer's callback-free Rule surface. It retains
// only the exact input, ordinary carry, and exact output slots.
type SchemaFragment struct {
	slot     *engine.RuleSlot[value.Value, value.StorageTransfer]
	input    engine.SchemaInput
	read     engine.SchemaReadSlot[value.Value]
	carry    engine.SchemaCarrySlot[value.Value]
	write    engine.SchemaWriteSlot[value.Value]
	semantic identity.SemanticKey
	evidence identity.SemanticKey
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[value.Value, value.StorageTransfer] {
	return fragment.slot
}

// DeclareSchema records the one-input ordinary Value transfer Rule shape.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily, evidence identity.SemanticKey, owner *valueowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || owner == nil || !identity.DistinctKeys(semantic, operandFamily, evidence) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[value.Value, value.StorageTransfer](builder, engine.SchemaRuleSpec[value.Value]{
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
	read, ok := engine.SchemaRead[value.Value](slot, owner.ExactRead(), input)
	if !ok {
		return nil, false
	}
	carry, ok := engine.SchemaCarryFrom(slot, input, owner.Ref())
	if !ok {
		return nil, false
	}
	write, ok := engine.SchemaWrite(slot, owner.ExactWrite())
	if !ok {
		return nil, false
	}
	return &SchemaFragment{slot: slot, input: input, read: read, carry: carry, write: write, semantic: semantic, evidence: evidence}, true
}
