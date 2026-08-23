// Package bodyresult declares Value's selected executable-body result
// transfer. It maps the canonical ReturnBoundary members of selected Call
// bodies onto the caller's existing MountedCallResultSlot coordinate.
package bodyresult

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type SchemaFragment struct {
	slot       *engine.RuleSlot[valuedomain.Value, valuedomain.MountedCallResultSlot]
	input      engine.SchemaInput
	callRead   engine.SchemaReadSlot[calldomain.Value]
	returnRead engine.SchemaReadSlot[valuedomain.Value]
	carry      engine.SchemaCarrySlot[valuedomain.Value]
	write      engine.SchemaWriteSlot[valuedomain.Value]
	semantic   identity.SemanticKey
}

func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[valuedomain.Value, valuedomain.MountedCallResultSlot] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

func DeclareSchema(builder *engine.SchemaBuilder, semantic, operandFamily identity.SemanticKey, values *valueowner.SchemaFragment, calls *callowner.SchemaFragment) (*SchemaFragment, bool) {
	if builder == nil || values == nil || calls == nil || !identity.DistinctKeys(semantic, operandFamily) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[valuedomain.Value, valuedomain.MountedCallResultSlot](builder, engine.SchemaRuleSpec[valuedomain.Value]{
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
	returnRead, ok := engine.SchemaSelectedRead[valuedomain.Value](slot, values.ExactRead(), input, callRead.Ref())
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
	return &SchemaFragment{slot: slot, input: input, callRead: callRead, returnRead: returnRead, carry: carry, write: write, semantic: semantic}, true
}
