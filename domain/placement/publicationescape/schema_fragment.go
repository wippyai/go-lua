package publicationescape

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// SchemaFragment is the callback-free mounted publication escape shape. The
// Effect-owned batch is the operand; Call gates the mounted call, Value
// supplies the receipt subjects/contexts, and Placement owns the routed write.
type SchemaFragment struct {
	slot          *engine.RuleSlot[placementdomain.Placement, effectfactor.MountedPublicationBatch]
	input         engine.SchemaInput
	callRead      engine.SchemaReadSlot[calldomain.Value]
	valueRead     engine.SchemaReadSlot[valuedomain.Value]
	placementRead engine.SchemaReadSlot[placementdomain.Placement]
	carry         engine.SchemaCarrySlot[placementdomain.Placement]
	write         engine.SchemaWriteSlot[placementdomain.Placement]
	semantic      identity.SemanticKey
}

// RuleSlot returns the exact cold Rule declaration.
func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[placementdomain.Placement, effectfactor.MountedPublicationBatch] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

// DeclareSchema records the Call exact read, selected Value source read, and
// selected Placement route-write dependency. The number of receipt rows is
// deliberately dynamic at selector time, so the cold shape stays one input
// per mounted call.
func DeclareSchema(
	builder *engine.SchemaBuilder,
	semantic, operandFamily identity.SemanticKey,
	values *valueowner.SchemaFragment,
	calls *callowner.SchemaFragment,
	owner *placementowner.SchemaFragment,
) (*SchemaFragment, bool) {
	if builder == nil || values == nil || calls == nil || owner == nil || !identity.DistinctKeys(semantic, operandFamily) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[placementdomain.Placement, effectfactor.MountedPublicationBatch](builder, engine.SchemaRuleSpec[placementdomain.Placement]{
		Semantic: semantic, OperandFamily: operandFamily, Inputs: 1,
		Output: owner.Ref(),
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
	valueRead, ok := engine.SchemaSelectedRead[valuedomain.Value](slot, values.ExactRead(), input, callRead.Ref())
	if !ok {
		return nil, false
	}
	placementRead, ok := engine.SchemaSelectedRead[placementdomain.Placement](slot, owner.ExactRead(), input, callRead.Ref(), valueRead.Ref())
	if !ok {
		return nil, false
	}
	carry, ok := engine.SchemaCarryFrom(slot, input, owner.Ref())
	if !ok {
		return nil, false
	}
	write, ok := engine.SchemaRouteWrite(slot, owner.ExactWrite(), placementRead)
	if !ok {
		return nil, false
	}
	return &SchemaFragment{
		slot: slot, input: input, callRead: callRead, valueRead: valueRead,
		placementRead: placementRead, carry: carry, write: write, semantic: semantic,
	}, true
}
