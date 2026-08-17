package refinement

import (
	"github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
)

// SchemaFragment is the callback-free reusable branch refinement shape. It
// reads one exact Value coordinate from the arm-entry environment, carries
// every unaffected coordinate, and strongly writes the narrowed coordinate at
// the Program artifact's Local stage.
type SchemaFragment struct {
	slot     *engine.RuleSlot[value.Value, value.PresenceRefinement]
	input    engine.SchemaInput
	read     engine.SchemaReadSlot[value.Value]
	carry    engine.SchemaCarrySlot[value.Value]
	write    engine.SchemaWriteSlot[value.Value]
	semantic identity.SemanticKey
	evidence identity.SemanticKey
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[value.Value, value.PresenceRefinement] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily, evidence identity.SemanticKey, owner *valueowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || owner == nil || !identity.DistinctKeys(semantic, operandFamily, evidence) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[value.Value, value.PresenceRefinement](builder, engine.SchemaRuleSpec[value.Value]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 1,
		Admission: engine.SchemaAdmission{Basis: engine.RuleAdmissionBasisDerivation, Identity: evidence},
		Output:    owner.Ref(),
	})
	if !ok {
		return nil, false
	}
	input, inputOK := slot.Input(0)
	read, readOK := engine.SchemaRead[value.Value](slot, owner.ExactRead(), input)
	carry, carryOK := engine.SchemaCarryFrom(slot, input, owner.Ref())
	write, writeOK := engine.SchemaWrite(slot, owner.ExactWrite())
	if !inputOK || !readOK || !carryOK || !writeOK {
		return nil, false
	}
	return &SchemaFragment{slot: slot, input: input, read: read, carry: carry, write: write, semantic: semantic, evidence: evidence}, true
}
