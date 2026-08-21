package freshresult

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// SchemaFragment is the callback-free Link-lane shape for the Target
// fresh-result to Value transfer. It reads the exact Call factor, carries the
// existing Value factor through the recency transform, and writes only the
// already-issued fixed CallResultValue coordinate.
type SchemaFragment struct {
	slot      *engine.RuleSlot[valuedomain.Value, valuedomain.FreshResultCall]
	input     engine.SchemaInput
	callRead  engine.SchemaReadSlot[calldomain.Value]
	carry     engine.SchemaCarrySlot[valuedomain.Value]
	write     engine.SchemaWriteSlot[valuedomain.Value]
	semantic  identity.SemanticKey
	transform identity.SemanticKey
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[valuedomain.Value, valuedomain.FreshResultCall] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

// DeclareSchema records the exact Call predecessor, transformed Value carry,
// and exact Value write. Fresh-result identity and admission remain Value's
// sealed cold authority and do not enter the schema slot as a second key.
func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily, transform identity.SemanticKey, values *valueowner.SchemaFragment, calls *callowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || values == nil || calls == nil || !identity.DistinctKeys(semantic, operandFamily, transform) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[valuedomain.Value, valuedomain.FreshResultCall](builder, engine.SchemaRuleSpec[valuedomain.Value]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 1, Output: values.Ref(),
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
	carry, ok := engine.SchemaCarry(slot, input, values.Ref(), transform)
	if !ok {
		return nil, false
	}
	write, ok := engine.SchemaWrite(slot, values.ExactWrite())
	if !ok {
		return nil, false
	}
	return &SchemaFragment{slot: slot, input: input, callRead: callRead, carry: carry, write: write, semantic: semantic, transform: transform}, true
}
