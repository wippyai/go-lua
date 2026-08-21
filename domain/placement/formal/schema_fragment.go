package formal

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	"github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// SchemaFragment is the callback-free formal ownership Rule shape. The
// dependency DAG is Call exact read -> mounted actual Value selection ->
// Placement selected route read/write. The engine owns all read ordinals and
// route-target correspondence.
type SchemaFragment struct {
	slot          *engine.RuleSlot[placement.Placement, operand]
	callRead      engine.SchemaReadSlot[calldomain.Value]
	actualRead    engine.SchemaReadSlot[valuedomain.Value]
	placementRead engine.SchemaReadSlot[placement.Placement]
	carry         engine.SchemaCarrySlot[placement.Placement]
	write         engine.SchemaWriteSlot[placement.Placement]
	semantic      identity.SemanticKey
}

// RuleSlot returns the exact cold Rule declaration for composition.
func (fragment *SchemaFragment) RuleSlot() *engine.RuleSlot[placement.Placement, operand] {
	if fragment == nil {
		return nil
	}
	return fragment.slot
}

// DeclareSchema records the exact cross-axis selector DAG. It accepts only
// neutral Factor forms; all Target/Program/Value meaning remains in the hot
// Call-mounted receipt, Pack projection, and owner authorities.
func DeclareSchema(
	builder *engine.SchemaBuilder,
	semantic, operandFamily identity.SemanticKey,
	values *valueowner.SchemaFragment,
	calls *callowner.SchemaFragment,
	owner *placementowner.SchemaFragment,
) (*SchemaFragment, bool) {
	if builder == nil || values == nil || calls == nil || owner == nil ||
		!identity.DistinctKeys(semantic, operandFamily) {
		return nil, false
	}
	slot, ok := engine.NewRuleSlot[placement.Placement, operand](builder, engine.SchemaRuleSpec[placement.Placement]{
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
	actualRead, ok := engine.SchemaSelectedRead[valuedomain.Value](slot, values.ExactRead(), input, callRead.Ref())
	if !ok {
		return nil, false
	}
	placementRead, ok := engine.SchemaSelectedRead[placement.Placement](slot, owner.ExactRead(), input, callRead.Ref(), actualRead.Ref())
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
		slot: slot, callRead: callRead, actualRead: actualRead,
		placementRead: placementRead, carry: carry, write: write,
		semantic: semantic,
	}, true
}
