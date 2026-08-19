package runtimekind

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// SchemaFragment is the callback-free shape of the cross-axis rule.  The
// output remains Value-owned; the two exact reads are selected from the Call
// and Value factors using the same predecessor input.
type SchemaFragment struct {
	slot           *engine.RuleSlot[valuedomain.Value, valuedomain.RuntimeKindCall]
	input          engine.SchemaInput
	callRead       engine.SchemaReadSlot[calldomain.Value]
	valueRead      engine.SchemaReadSlot[valuedomain.Value]
	comparisonRead engine.SchemaReadSlot[valuedomain.Value]
	carry          engine.SchemaCarrySlot[valuedomain.Value]
	write          engine.SchemaWriteSlot[valuedomain.Value]
	semantic       identity.SemanticKey
	evidence       identity.SemanticKey
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[valuedomain.Value, valuedomain.RuntimeKindCall] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

// DeclareSchema records one exact Call read, one exact Value read, ordinary
// Value carry, and an exact Value write.  No domain-specific engine surface
// is introduced for this relation.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily, evidence identity.SemanticKey, values *valueowner.SchemaFragment, calls *callowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || values == nil || calls == nil || !identity.DistinctKeys(semantic, operandFamily, evidence) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[valuedomain.Value, valuedomain.RuntimeKindCall](builder, engine.SchemaRuleSpec[valuedomain.Value]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 1,
		Admission: engine.SchemaAdmission{Basis: engine.RuleAdmissionBasisDerivation, Identity: evidence},
		Output:    values.Ref(),
	})
	if !ok {
		return nil, false
	}
	input, ok := slot.Input(0)
	if !ok {
		return nil, false
	}
	callRead, ok := engine.SchemaRead[calldomain.Value](slot, calls.ExactRead(), input)
	if !ok {
		return nil, false
	}
	valueRead, ok := engine.SchemaRead[valuedomain.Value](slot, values.ExactRead(), input)
	if !ok {
		return nil, false
	}
	comparisonRead, ok := engine.SchemaRead[valuedomain.Value](slot, values.ExactRead(), input)
	if !ok {
		return nil, false
	}
	carry, ok := engine.SchemaCarryFrom(slot, input, values.Ref())
	if !ok {
		return nil, false
	}
	write, ok := engine.SchemaWrite(slot, values.ExactWrite())
	if !ok {
		return nil, false
	}
	return &SchemaFragment{slot: slot, input: input, callRead: callRead, valueRead: valueRead, comparisonRead: comparisonRead, carry: carry, write: write, semantic: semantic, evidence: evidence}, true
}
