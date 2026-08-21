// Package resultalias declares Value's selected Target ResultAlias transfer.
//
// The package owns no Target, Program, Boundary, or Pack rows.  The cold
// Value schema supplies one admitted result-slot operand per mounted call; Call's
// inverse and Pack's mounted-actual projection are consumed only by the hot
// binder.
package resultalias

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// SchemaFragment is the callback-free shape of the one-operand ResultAlias
// rule.  The dependency order is exact Call read, selected Value actual read,
// then identity carry and exact Value write.  The selected read is dynamic:
// its locator emits only the mounted actual coordinates named by the selected
// Target alias plan.
type SchemaFragment struct {
	slot       *engine.RuleSlot[valuedomain.Value, valuedomain.MountedCallResultSlot]
	input      engine.SchemaInput
	callRead   engine.SchemaReadSlot[calldomain.Value]
	actualRead engine.SchemaReadSlot[valuedomain.Value]
	carry      engine.SchemaCarrySlot[valuedomain.Value]
	write      engine.SchemaWriteSlot[valuedomain.Value]
	semantic   identity.SemanticKey
}

// RuleSlot returns the exact cold Rule declaration for composition.
func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[valuedomain.Value, valuedomain.MountedCallResultSlot] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

// DeclareSchema declares one admitted result-slot ResultAlias operand per mounted
// call.  It intentionally does not form a Call×Operation product: operation
// selection remains a bind-time Target plan consumed by the hot locator.
func DeclareSchema(
	builder *engine.SchemaBuilder,
	semantic, operandFamily identity.SemanticKey,
	values *valueowner.SchemaFragment,
	calls *callowner.SchemaFragment,
) (*SchemaFragment, bool) {
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
	actualRead, ok := engine.SchemaSelectedRead[valuedomain.Value](slot, values.ExactRead(), input, callRead.Ref())
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
	return &SchemaFragment{
		slot: slot, input: input, callRead: callRead, actualRead: actualRead,
		carry: carry, write: write, semantic: semantic,
	}, true
}
