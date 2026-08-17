package dispatch

import (
	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
)

// SchemaFragment is Call dispatch's callback-free one-input Rule surface.
type SchemaFragment struct {
	slot     *engine.RuleSlot[calldomain.Value, dispatchReceipt]
	input    engine.SchemaInput
	read     engine.SchemaReadSlot[valuedomain.Value]
	write    engine.SchemaWriteSlot[calldomain.Value]
	value    engine.FactorRef[valuedomain.Value]
	call     engine.FactorRef[calldomain.Value]
	semantic identity.SemanticKey
	evidence identity.SemanticKey
}

// DeclareSchema records the exact Value-read/Call-write dispatch incidence.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily, evidence identity.SemanticKey, values *valueowner.SchemaFragment, calls *callowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || values == nil || calls == nil || !identity.DistinctKeys(semantic, operandFamily, evidence) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[calldomain.Value, dispatchReceipt](builder, engine.SchemaRuleSpec[calldomain.Value]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 1,
		Admission: engine.SchemaAdmission{Basis: engine.RuleAdmissionBasisDerivation, Identity: evidence},
		Output:    calls.Ref(),
	})
	if !ok {
		return nil, false
	}
	input, ok := slot.Input(0)
	if !ok {
		return nil, false
	}
	read, ok := engine.SchemaRead[valuedomain.Value](slot, values.ExactRead(), input)
	if !ok {
		return nil, false
	}
	write, ok := engine.SchemaWrite(slot, calls.ExactWrite())
	if !ok {
		return nil, false
	}
	return &SchemaFragment{slot: slot, input: input, read: read, write: write, value: values.Ref(), call: calls.Ref(), semantic: semantic, evidence: evidence}, true
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[calldomain.Value, dispatchReceipt] {
	return fragment.slot
}
